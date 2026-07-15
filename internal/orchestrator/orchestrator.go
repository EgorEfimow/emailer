// Package orchestrator composes the email AI agent pipeline: ingest → reason →
// act → notify. It is the top-level coordinator that wires together the mail
// ingester, LLM provider, digest renderer, and notification channel into a
// single Run method.
//
// The orchestrator is designed for one-shot execution by an OS scheduler
// (cron, systemd timer). It is not a long-running server.
package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/egorefimow/emailer/internal/digest"
	"github.com/egorefimow/emailer/internal/llm"
	"github.com/egorefimow/emailer/internal/mail"
	"github.com/egorefimow/emailer/internal/notify"
	"github.com/egorefimow/emailer/internal/store"
)

// ---------------------------------------------------------------------------
// RunOptions
// ---------------------------------------------------------------------------

// RunOptions controls the behaviour of a single pipeline execution.
//
// Zero values are sensible defaults: no explicit window (dynamic from last
// successful run), no forced reprocessing, full read-write mode.
type RunOptions struct {
	// Window overrides the dynamic fetch window derived from the last
	// successful run. When nil, the orchestrator uses GetLastSuccessfulRunTime
	// and caps the lookback to config.MaxWindow. When set, the dynamic logic
	// is bypassed entirely.
	Window *time.Duration

	// ForceReprocess skips the dedup check against already-processed messages.
	// All fetched messages are classified regardless of prior runs.
	ForceReprocess bool

	// DryRun skips side effects: IMAP flag writes and notification delivery.
	// The pipeline still fetches, classifies, and renders the digest, but
	// nothing is persisted or sent externally.
	DryRun bool

	// Stateless disables persistence to the store. When true, the store is
	// still opened (for the run ledger) but no messages or flags are recorded.
	// Requires FetchUnreadOnly=true in the config to avoid data loss.
	Stateless bool
}

// ---------------------------------------------------------------------------
// Result
// ---------------------------------------------------------------------------

// Result holds the outcome of a single pipeline run.
type Result struct {
	// RunID is the unique identifier assigned to this run by the store.
	RunID string

	// Status indicates the final state of the run.
	Status store.RunStatus

	// TotalFetched is the number of messages fetched from all accounts.
	TotalFetched int

	// TotalClassified is the number of messages successfully classified by
	// the LLM (or marked as failures). This may differ from TotalFetched
	// when messages are filtered out as already processed.
	TotalClassified int

	// FailedCount is the number of messages that failed classification.
	FailedCount int

	// Err is the overall run error, if any. Per-account and per-message
	// errors are logged individually; this field is set only when the run
	// as a whole cannot proceed (e.g. all accounts failed, context cancelled).
	Err error
}

// ---------------------------------------------------------------------------
// Pipeline
// ---------------------------------------------------------------------------

// Pipeline composes the email AI agent pipeline stages into a single run.
//
// All dependencies are injected at construction time. The Run method executes
// the pipeline: fetch → filter → classify → render → flag → notify.
//
// Pipeline is safe for sequential use (one Run at a time). Concurrent calls
// are not supported.
type Pipeline struct {
	store            store.Store
	ingesters        map[string]mail.Ingester
	provider         llm.Provider
	renderer         digest.Renderer
	fallbackRenderer digest.Renderer
	channel          notify.Channel
	logger           *slog.Logger
	cfg              config.Config
	now              func() time.Time
}

// New creates a new Pipeline with the given dependencies.
//
// All parameters are required. The ingesters map must have an entry for every
// account in config.IMAP.Accounts (by label). The logger is used for
// structured logging throughout the run.
func New(
	store store.Store,
	ingesters map[string]mail.Ingester,
	provider llm.Provider,
	renderer digest.Renderer,
	fallbackRenderer digest.Renderer,
	channel notify.Channel,
	logger *slog.Logger,
	cfg config.Config,
) *Pipeline {
	return &Pipeline{
		store:            store,
		ingesters:        ingesters,
		provider:         provider,
		renderer:         renderer,
		fallbackRenderer: fallbackRenderer,
		channel:          channel,
		logger:           logger,
		cfg:              cfg,
		now:              time.Now,
	}
}

// ---------------------------------------------------------------------------
// Run (steps 1-4)
// ---------------------------------------------------------------------------

// Run executes the full pipeline: ingest → reason → act → notify.
//
// Steps 1-4 (this increment):
//  1. Record the run in the store.
//  2. Determine the fetch window (explicit, dynamic, or default 24h).
//  3. Fetch messages from all accounts concurrently.
//  4. Filter out already-processed messages (unless ForceReprocess).
//  5. Build the LLM classification request.
//
// Steps 5-10 (next increment):
//  6. Call the LLM provider and parse the response.
//  7. Render the digest (Markdown or fallback).
//  8. Apply IMAP keyword flags.
//  9. Send the digest via the notification channel.
//  10. Record the run finish.
func (p *Pipeline) Run(ctx context.Context, opts RunOptions) Result { //nolint:gocyclo
	// -----------------------------------------------------------------------
	// Step 1: Record run start
	// -----------------------------------------------------------------------
	run := store.Run{
		StartedAt: p.now(),
		Status:    store.RunStatusRunning,
	}
	run, err := p.store.RecordRun(ctx, run)
	if err != nil {
		return Result{
			Status: store.RunStatusIngestFailed,
			Err:    fmt.Errorf("orchestrator.record_run: %w", err),
		}
	}

	result := Result{RunID: run.ID}
	log := p.logger.With(slog.String("run_id", run.ID))
	log.InfoContext(ctx, "run started")

	// -----------------------------------------------------------------------
	// Step 2: Determine fetch window and fetch from all accounts
	// -----------------------------------------------------------------------
	since, err := p.fetchWindow(ctx, opts)
	if err != nil {
		log.ErrorContext(ctx, "failed to determine fetch window", slog.Any("error", err))
		if ferr := p.store.FinishRun(ctx, run.ID, store.RunStatusIngestFailed, 0, err); ferr != nil {
			log.ErrorContext(ctx, "failed to finish run", slog.Any("error", ferr))
		}
		result.Status = store.RunStatusIngestFailed
		result.Err = err
		return result
	}
	log.InfoContext(ctx, "fetch window determined",
		slog.Time("since", since),
		slog.Bool("fetch_unread_only", p.cfg.FetchUnreadOnly),
	)

	fetchOpts := mail.FetchOptions{
		Since:           since,
		FetchUnreadOnly: p.cfg.FetchUnreadOnly,
	}
	fetchResults := mail.FetchAll(
		ctx, p.cfg.IMAP.Accounts, p.ingesters, fetchOpts,
		p.cfg.Concurrency.MaxAccounts,
	)

	accountErrors := mail.AccountErrors(fetchResults)
	messages := mail.FlattenMessages(fetchResults)
	result.TotalFetched = len(messages)

	if len(accountErrors) > 0 {
		for label, aerr := range accountErrors {
			log.ErrorContext(ctx, "account fetch failed",
				slog.String("account", label),
				slog.Any("error", aerr),
			)
		}
		if len(messages) == 0 {
			// All accounts failed — cannot proceed.
			allFailed := fmt.Errorf("all %d account(s) failed to fetch messages", len(accountErrors))
			if ferr := p.store.FinishRun(ctx, run.ID, store.RunStatusIngestFailed, 0, allFailed); ferr != nil {
				log.ErrorContext(ctx, "failed to finish run", slog.Any("error", ferr))
			}
			result.Status = store.RunStatusIngestFailed
			result.Err = allFailed
			return result
		}
		// Some accounts succeeded — continue but mark as degraded.
		result.Status = store.RunStatusPartial
	}
	log.InfoContext(ctx, "messages fetched", slog.Int("total", len(messages)))

	// -----------------------------------------------------------------------
	// Step 3: Filter already-processed messages
	// -----------------------------------------------------------------------
	if !opts.ForceReprocess && len(messages) > 0 {
		messages = p.filterProcessed(ctx, run.ID, messages, log)
		result.TotalFetched = len(messages)
	}

	if len(messages) == 0 {
		log.InfoContext(ctx, "no new messages to process")
		if ferr := p.store.FinishRun(ctx, run.ID, store.RunStatusCompleted, 0, nil); ferr != nil {
			log.ErrorContext(ctx, "failed to finish run", slog.Any("error", ferr))
		}
		result.Status = store.RunStatusCompleted
		return result
	}

	// -----------------------------------------------------------------------
	// Step 4: Build LLM classification request
	// -----------------------------------------------------------------------
	labels := p.buildLabelSet()
	inputMessages := p.messagesToInputs(messages)

	request := llm.Request{
		Model:                p.cfg.LLM.Model,
		SystemPrompt:         p.cfg.Prompts.SystemPrompt,
		ClassificationPrompt: p.cfg.Prompts.ClassificationPrompt,
		Labels:               labels,
		Messages:             inputMessages,
	}

	log.InfoContext(ctx, "LLM request built",
		slog.Int("messages", len(inputMessages)),
	)

	// -----------------------------------------------------------------------
	// Step 5: Call the LLM provider
	// -----------------------------------------------------------------------
	llmResponse, llmErr := p.provider.Classify(ctx, request)
	if llmErr != nil {
		log.ErrorContext(ctx, "LLM classification failed, using fallback",
			slog.Any("error", llmErr),
		)
		result.Status = store.RunStatusDegraded
	}

	// -----------------------------------------------------------------------
	// Step 6: Render the digest
	// -----------------------------------------------------------------------
	var classifications []mail.Classification
	if llmErr == nil {
		classifications = llmResponse.Classifications
	}

	digestData := p.buildDigestData(run.ID, messages, classifications, fetchResults, accountErrors)
	result.TotalClassified = len(digestData.Messages)
	result.FailedCount = digestData.FailedCount

	var renderedDigest string
	if llmErr == nil {
		renderedDigest, err = p.renderer.Render(ctx, digestData)
	} else {
		renderedDigest, err = p.fallbackRenderer.Render(ctx, digestData)
	}
	if err != nil {
		log.ErrorContext(ctx, "digest rendering failed", slog.Any("error", err))
		if ferr := p.store.FinishRun(ctx, run.ID, store.RunStatusDegraded, len(messages), err); ferr != nil {
			log.ErrorContext(ctx, "failed to finish run", slog.Any("error", ferr))
		}
		result.Status = store.RunStatusDegraded
		result.Err = err
		return result
	}
	log.InfoContext(ctx, "digest rendered", slog.Int("messages", len(digestData.Messages)))

	// -----------------------------------------------------------------------
	// Step 7: Apply IMAP keyword flags
	// -----------------------------------------------------------------------
	if !opts.DryRun && llmErr == nil {
		p.applyClassificationFlags(ctx, classifications, log)
	}

	// -----------------------------------------------------------------------
	// Step 8: Send the digest via the notification channel
	// -----------------------------------------------------------------------
	if !opts.DryRun {
		digestHash := payloadHash(renderedDigest)
		digestErr := p.channel.Send(ctx, renderedDigest, notify.SendOptions{})

		if digestErr != nil {
			log.ErrorContext(ctx, "digest delivery failed", slog.Any("error", digestErr))
			if derr := p.store.RecordDigest(ctx, store.DigestRecord{
				RunID:       run.ID,
				Channel:     p.channel.Name(),
				Status:      store.DigestStatusFailed,
				PayloadHash: digestHash,
			}); derr != nil {
				log.ErrorContext(ctx, "failed to record digest", slog.Any("error", derr))
			}
			if ferr := p.store.FinishRun(ctx, run.ID, store.RunStatusDegraded, len(messages), digestErr); ferr != nil {
				log.ErrorContext(ctx, "failed to finish run", slog.Any("error", ferr))
			}
			result.Status = store.RunStatusDegraded
			result.Err = digestErr
			return result
		}

		if derr := p.store.RecordDigest(ctx, store.DigestRecord{
			RunID:       run.ID,
			Channel:     p.channel.Name(),
			Status:      store.DigestStatusSent,
			PayloadHash: digestHash,
		}); derr != nil {
			log.ErrorContext(ctx, "failed to record digest", slog.Any("error", derr))
		}
		log.InfoContext(ctx, "digest sent")
	}

	// -----------------------------------------------------------------------
	// Step 9: Record processed messages and flags
	// -----------------------------------------------------------------------
	if !opts.Stateless && llmErr == nil {
		p.recordProcessedMessages(ctx, run.ID, messages, classifications, log)
		p.recordFlags(ctx, classifications, log)
	}

	// -----------------------------------------------------------------------
	// Step 10: Record run finish
	// -----------------------------------------------------------------------
	runStatus := func() store.RunStatus {
		switch result.Status {
		case store.RunStatusPartial:
			return store.RunStatusPartial
		case store.RunStatusDegraded:
			return store.RunStatusDegraded
		default:
			return store.RunStatusCompleted
		}
	}()
	if ferr := p.store.FinishRun(ctx, run.ID, runStatus, len(messages), nil); ferr != nil {
		log.ErrorContext(ctx, "failed to finish run", slog.Any("error", ferr))
	}
	result.Status = runStatus
	log.InfoContext(ctx, "run finished", slog.String("status", string(runStatus)))
	return result
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// fetchWindow computes the fetch window start time.
//
// Priority order:
//  1. Explicit Window option (bypasses dynamic logic entirely).
//  2. Last successful run time (dynamic watermark), capped at MaxWindow.
//  3. 24-hour fallback when no previous run exists.
func (p *Pipeline) fetchWindow(ctx context.Context, opts RunOptions) (time.Time, error) {
	// Explicit window overrides dynamic logic.
	if opts.Window != nil {
		return p.now().Add(-*opts.Window), nil
	}

	// Dynamic window: derive from the last successful run.
	lastRun, err := p.store.GetLastSuccessfulRunTime(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("orchestrator.window: %w", err)
	}

	since := p.now().Add(-24 * time.Hour) // default: 24h fallback
	if lastRun != nil {
		since = *lastRun
	}

	// Cap the lookback to prevent overwhelming the LLM after prolonged
	// host downtime or a very long gap between runs.
	maxWin := p.cfg.MaxWindow
	if maxWin <= 0 {
		maxWin = 72 * time.Hour
	}
	minSince := p.now().Add(-maxWin)
	if since.Before(minSince) {
		since = minSince
	}

	return since, nil
}

// filterProcessed removes messages that have already been processed in a
// previous run. Returns the filtered slice. On store error, it logs a warning
// and returns the original slice (fail-soft).
func (p *Pipeline) filterProcessed(ctx context.Context, runID string, messages []mail.Message, log *slog.Logger) []mail.Message {
	keys := make([]store.MessageKey, len(messages))
	for i, m := range messages {
		keys[i] = store.MessageKey{AccountLabel: m.AccountLabel, UID: m.UID}
	}

	processed, err := p.store.AlreadyProcessed(ctx, keys)
	if err != nil {
		log.WarnContext(ctx, "failed to check processed messages, continuing without dedup",
			slog.Any("error", err),
		)
		return messages
	}

	if len(processed) == 0 {
		return messages
	}

	log.InfoContext(ctx, "filtering already processed messages",
		slog.Int("duplicates", len(processed)),
	)

	// Convert store.MessageKey to mail.MessageKey for the filter function.
	processedMail := make(map[mail.MessageKey]bool, len(processed))
	for pk := range processed {
		processedMail[mail.MessageKey{AccountLabel: pk.AccountLabel, UID: pk.UID}] = true
	}

	return mail.FilterAlreadyProcessed(messages, processedMail)
}

// buildLabelSet returns the full set of classification labels (defaults +
// custom).
func (p *Pipeline) buildLabelSet() []string {
	labels := make([]string, 0, len(mail.DefaultLabels)+len(p.cfg.Labels.Custom))
	labels = append(labels, mail.DefaultLabels...)
	labels = append(labels, p.cfg.Labels.Custom...)
	return labels
}

// messagesToInputs converts mail.Messages to llm.InputMessages for the LLM
// request.
func (p *Pipeline) messagesToInputs(messages []mail.Message) []llm.InputMessage {
	inputs := make([]llm.InputMessage, len(messages))
	for i, m := range messages {
		inputs[i] = llm.InputMessage{
			Key:     mail.MessageKey{AccountLabel: m.AccountLabel, UID: m.UID},
			Subject: m.Subject,
			From:    m.From,
			Date:    m.Date,
			Body:    m.Body,
			IsRead:  m.IsRead,
		}
	}
	return inputs
}

// ---------------------------------------------------------------------------
// buildDigestData
// ---------------------------------------------------------------------------

// buildDigestData constructs a digest.DigestData from the fetched messages
// and LLM classifications. When classifications are nil (LLM failure), the
// digest data is built without classification data for the fallback renderer.
func (p *Pipeline) buildDigestData(runID string, messages []mail.Message, classifications []mail.Classification, fetchResults []mail.FetchAllResult, accountErrors map[string]error) digest.DigestData {
	// Build a lookup map from the classification key.
	classMap := make(map[mail.MessageKey]mail.Classification, len(classifications))
	for _, c := range classifications {
		classMap[c.Key] = c
	}

	entries := make([]digest.MessageEntry, 0, len(messages))
	failedCount := 0
	for _, m := range messages {
		c, ok := classMap[m.Key()]
		if !ok {
			c = mail.Classification{
				Key:   m.Key(),
				Label: "Unknown",
			}
			failedCount++
		}
		entries = append(entries, digest.MessageEntry{
			Subject:        m.Subject,
			From:           m.From,
			Date:           m.Date,
			IsRead:         m.IsRead,
			Classification: c,
			Excerpt:        m.Body,
		})
	}

	globalStats, accountStats := buildDigestStats(messages, entries, classifications, fetchResults, accountErrors)

	return digest.DigestData{
		RunID:           runID,
		GeneratedAt:     p.now(),
		Messages:        entries,
		TotalFetched:    globalStats.FetchedCount,
		TotalClassified: globalStats.ClassifiedCount,
		FailedCount:     globalStats.FailedCount,
		GlobalStats:     globalStats,
		AccountStats:    accountStats,
	}
}

func buildDigestStats(messages []mail.Message, entries []digest.MessageEntry, classifications []mail.Classification, fetchResults []mail.FetchAllResult, accountErrors map[string]error) (digest.DigestStats, []digest.AccountStats) {
	global := digest.DigestStats{
		FetchedCount:    len(messages),
		ClassifiedCount: len(classifications),
		CountsByLabel:   make(map[string]int),
	}
	classifiedKeys := make(map[mail.MessageKey]bool, len(classifications))
	for _, classification := range classifications {
		classifiedKeys[classification.Key] = true
	}

	accountByLabel := make(map[string]*digest.AccountStats)
	accountOrder := make([]string, 0)
	ensureAccount := func(label string) *digest.AccountStats {
		if stats, ok := accountByLabel[label]; ok {
			return stats
		}
		accountByLabel[label] = &digest.AccountStats{
			AccountLabel:  label,
			Status:        "ok",
			CountsByLabel: make(map[string]int),
		}
		accountOrder = append(accountOrder, label)
		return accountByLabel[label]
	}

	for _, r := range fetchResults {
		ensureAccount(r.Account.Label)
	}
	for label, err := range accountErrors {
		stats := ensureAccount(label)
		stats.Status = "error"
		if err != nil {
			stats.Error = err.Error()
		}
	}

	for _, m := range messages {
		stats := ensureAccount(m.AccountLabel)
		stats.FetchedCount++
		if m.IsRead {
			global.ReadCount++
			stats.ReadCount++
		} else {
			global.UnreadCount++
			stats.UnreadCount++
		}
	}

	for _, entry := range entries {
		label := entry.Classification.Label
		if label == "" {
			label = "Unknown"
		}
		global.CountsByLabel[label]++
		stats := ensureAccount(entry.Classification.Key.AccountLabel)
		stats.CountsByLabel[label]++
		if classifiedKeys[entry.Classification.Key] {
			stats.ClassifiedCount++
		} else {
			global.FailedCount++
			stats.FailedCount++
		}
	}

	accounts := make([]digest.AccountStats, 0, len(accountOrder))
	for _, label := range accountOrder {
		accounts = append(accounts, *accountByLabel[label])
	}
	return global, accounts
}

// ---------------------------------------------------------------------------
// applyClassificationFlags
// ---------------------------------------------------------------------------

// applyClassificationFlags converts classifications to IMAP keyword flags and
// applies them per account. Errors are logged per-account but do not abort the
// pipeline.
func (p *Pipeline) applyClassificationFlags(ctx context.Context, classifications []mail.Classification, log *slog.Logger) {
	// Convert classifications to flags.
	flags := make([]mail.Flag, 0, len(classifications))
	for _, c := range classifications {
		flag := mail.ClassificationToFlag(c, p.cfg.Labels)
		if flag.Keyword != "" {
			flags = append(flags, flag)
		}
	}

	if len(flags) == 0 {
		return
	}

	// Group flags by account for batched application.
	byAccount := make(map[string][]mail.Flag)
	for _, f := range flags {
		byAccount[f.Key.AccountLabel] = append(byAccount[f.Key.AccountLabel], f)
	}

	// Build a fast lookup of account configs.
	acctMap := make(map[string]config.IMAPAccount, len(p.cfg.IMAP.Accounts))
	for _, a := range p.cfg.IMAP.Accounts {
		acctMap[a.Label] = a
	}

	for label, accountFlags := range byAccount {
		acct, ok := acctMap[label]
		if !ok {
			log.WarnContext(ctx, "account config not found for flag application",
				slog.String("account", label),
			)
			continue
		}
		ingester, ok := p.ingesters[label]
		if !ok {
			log.WarnContext(ctx, "ingester not found for flag application",
				slog.String("account", label),
			)
			continue
		}
		if err := ingester.ApplyFlags(ctx, acct, accountFlags); err != nil {
			log.ErrorContext(ctx, "flag application failed",
				slog.String("account", label),
				slog.Any("error", err),
			)
		} else {
			log.InfoContext(ctx, "flags applied",
				slog.String("account", label),
				slog.Int("count", len(accountFlags)),
			)
		}
	}
}

// ---------------------------------------------------------------------------
// recordProcessedMessages
// ---------------------------------------------------------------------------

// recordProcessedMessages persists processed message records to the store.
// Errors are logged per-message but do not abort the pipeline.
func (p *Pipeline) recordProcessedMessages(ctx context.Context, runID string, messages []mail.Message, classifications []mail.Classification, log *slog.Logger) {
	classMap := make(map[mail.MessageKey]mail.Classification, len(classifications))
	for _, c := range classifications {
		classMap[c.Key] = c
	}

	excerptLimit := p.cfg.Digest.MaxMessageExcerpt
	if excerptLimit <= 0 {
		excerptLimit = 500
	}

	for _, m := range messages {
		c, ok := classMap[m.Key()]
		if !ok {
			continue
		}
		pm := store.ProcessedMessage{
			RunID:          runID,
			AccountLabel:   m.AccountLabel,
			UID:            m.UID,
			IsRead:         m.IsRead,
			Classification: c.Label,
			DigestExcerpt:  truncateExcerpt(m.Body, excerptLimit),
			ProcessedAt:    p.now(),
		}
		if err := p.store.RecordMessage(ctx, pm); err != nil {
			log.ErrorContext(ctx, "failed to record processed message",
				slog.String("key", m.Key().Key()),
				slog.Any("error", err),
			)
		}
	}
}

// ---------------------------------------------------------------------------
// recordFlags
// ---------------------------------------------------------------------------

// recordFlags persists flag application records to the store. Errors are
// logged per-flag but do not abort the pipeline.
func (p *Pipeline) recordFlags(ctx context.Context, classifications []mail.Classification, log *slog.Logger) {
	for _, c := range classifications {
		flag := mail.ClassificationToFlag(c, p.cfg.Labels)
		if flag.Keyword == "" {
			continue
		}
		rec := store.FlagRecord{
			AccountLabel: c.Key.AccountLabel,
			UID:          c.Key.UID,
			Flag:         flag.Keyword,
			AppliedAt:    p.now(),
		}
		if err := p.store.RecordFlag(ctx, rec); err != nil {
			log.ErrorContext(ctx, "failed to record flag",
				slog.String("key", c.Key.Key()),
				slog.String("flag", flag.Keyword),
				slog.Any("error", err),
			)
		}
	}
}

// ---------------------------------------------------------------------------
// payloadHash
// ---------------------------------------------------------------------------

// payloadHash computes a SHA-256 hex digest of the rendered payload for
// dedup and integrity tracking.
func payloadHash(payload string) string {
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// ---------------------------------------------------------------------------
// truncateExcerpt
// ---------------------------------------------------------------------------

// truncateExcerpt truncates a body string to the given limit, appending
// an ellipsis if truncated.
func truncateExcerpt(body string, limit int) string {
	if limit <= 0 || len(body) <= limit {
		return body
	}
	return body[:limit] + "…"
}
