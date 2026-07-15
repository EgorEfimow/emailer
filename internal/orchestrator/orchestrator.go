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
	"sort"
	"strings"
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

	// AnalysisFailedCount is the number of messages whose LLM analysis
	// (summary, key_points, action_items) failed validation and fell back
	// to raw excerpt.
	AnalysisFailedCount int

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
		BatchSize:       p.cfg.Concurrency.FetchBatchSize,
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
	// Step 5: Call the LLM provider with partial fallback
	// -----------------------------------------------------------------------
	validClassifications, failedClassifications, analysisErrors, classifyErr := p.classifyWithPartialFallback(ctx, request, messages, log)
	if classifyErr != nil {
		log.ErrorContext(ctx, "LLM classification failed completely, using fallback",
			slog.Any("error", classifyErr),
		)
		result.Status = store.RunStatusDegraded
	}

	// -----------------------------------------------------------------------
	// Step 6: Render the digest
	// -----------------------------------------------------------------------
	digestData := p.buildDigestDataPartial(ctx, run.ID, messages, validClassifications, failedClassifications, analysisErrors, fetchResults, accountErrors)
	result.TotalClassified = len(digestData.Messages)
	result.FailedCount = digestData.FailedCount
	result.AnalysisFailedCount = digestData.AnalysisFailedCount

	var renderedDigest string
	if classifyErr == nil && len(validClassifications) > 0 {
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
	log.InfoContext(ctx, "digest rendered",
		slog.Int("messages", len(digestData.Messages)),
		slog.Int("analysis_failed", digestData.AnalysisFailedCount),
	)

	// Determine run status based on classification results
	// If classifyErr is set, it's a hard failure (degraded)
	// If no valid classifications remain after repair, it's degraded (all failed)
	// If some valid and some failed, it's partially_classified
	// If all valid, completed (unless other issues)
	if classifyErr != nil {
		result.Status = store.RunStatusDegraded
	} else if len(validClassifications) == 0 {
		// All items failed - no valid analyses at all
		result.Status = store.RunStatusDegraded
	} else if result.AnalysisFailedCount > 0 {
		result.Status = store.RunStatusPartiallyClassified
	}

	// -----------------------------------------------------------------------
	// Step 7: Apply IMAP keyword flags
	// -----------------------------------------------------------------------
	if !opts.DryRun && classifyErr == nil {
		p.applyClassificationFlags(ctx, validClassifications, log)
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
	if !opts.Stateless && classifyErr == nil {
		p.recordProcessedMessages(ctx, run.ID, messages, validClassifications, log)
		p.recordFlags(ctx, validClassifications, log)
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
		case store.RunStatusPartiallyClassified:
			return store.RunStatusPartiallyClassified
		default:
			if result.AnalysisFailedCount > 0 {
				return store.RunStatusPartiallyClassified
			}
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
func (p *Pipeline) buildDigestData(ctx context.Context, runID string, messages []mail.Message, classifications []mail.Classification, fetchResults []mail.FetchAllResult, accountErrors map[string]error) digest.DigestData {
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

	globalStats, accountStats, globalSenderCounts, globalDomainCounts := buildDigestStats(messages, entries, classifications, fetchResults, accountErrors)

	highlights := p.buildHighlights(ctx, runID, messages, globalStats, accountStats, globalSenderCounts, globalDomainCounts)

	return digest.DigestData{
		RunID:           runID,
		GeneratedAt:     p.now(),
		Messages:        entries,
		TotalFetched:    globalStats.FetchedCount,
		TotalClassified: globalStats.ClassifiedCount,
		FailedCount:     globalStats.FailedCount,
		GlobalStats:     globalStats,
		AccountStats:    accountStats,
		Highlights:      highlights,
	}
}

// ---------------------------------------------------------------------------
// Partial LLM Failure Fallback
// ---------------------------------------------------------------------------

// makeFailedClassification creates a minimal classification for a message
// whose analysis failed validation/repair. It carries the key and "Unknown"
// label with empty analysis fields.
func (p *Pipeline) makeFailedClassification(key mail.MessageKey) mail.Classification {
	return mail.Classification{
		Key:   key,
		Label: "Unknown",
	}
}

// repairOnce attempts to repair a failed LLM response by sending a repair
// prompt to the provider. Returns the repaired raw response or an error.
func (p *Pipeline) repairOnce(ctx context.Context, raw string, parseErr error, validLabels []string, log *slog.Logger) (string, error) {
	repairPrompt, err := llm.RepairWithPrompt(raw, parseErr, validLabels)
	if err != nil {
		return "", fmt.Errorf("repair: build prompt: %w", err)
	}
	log.InfoContext(ctx, "LLM repair attempt", slog.Int("raw_len", len(raw)))
	req := llm.Request{
		Model:                p.cfg.LLM.Model,
		SystemPrompt:         p.cfg.Prompts.SystemPrompt,
		ClassificationPrompt: p.cfg.Prompts.ClassificationPrompt,
		Labels:               validLabels,
		Messages:             nil, // repair prompt contains all context
	}
	// Override the prompt with the repair prompt by using a custom system prompt
	// that includes the repair instructions. The provider.Classify will use the
	// request's ClassificationPrompt field.
	req.ClassificationPrompt = repairPrompt
	resp, err := p.provider.Classify(ctx, req)
	if err != nil {
		return "", fmt.Errorf("repair: provider classify: %w", err)
	}
	return resp.RawResponse, nil
}

// classifyWithPartialFallback implements the partial failure fallback policy:
// 1. Call provider.Classify
// 2. Parse response with ParseResponse
// 3. If any items invalid, attempt one repair (if AnalysisRepairMaxAttempts > 0)
// 4. Accept valid analyses, mark invalid as failed with raw excerpt fallback
// 5. If zero valid items after repair, return error for whole-digest fallback
// nolint:gocyclo
func (p *Pipeline) classifyWithPartialFallback(ctx context.Context, request llm.Request, messages []mail.Message, log *slog.Logger) ([]mail.Classification, []mail.Classification, []mail.AnalysisError, error) {
	validLabels := p.buildLabelSet()
	maxAttempts := p.cfg.LLM.AnalysisRepairMaxAttempts
	if maxAttempts < 0 {
		maxAttempts = 1
	}

	// First classification attempt
	resp, err := p.provider.Classify(ctx, request)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("initial classify: %w", err)
	}

	parseResult, parseErr := llm.ParseResponse(resp.RawResponse, validLabels)

	// If fully successful, return all classifications
	if parseErr == nil {
		return parseResult.Classifications, nil, nil, nil
	}

	// parseErr is not nil, but parseResult.Classifications may contain partial valid results
	validClassifications := parseResult.Classifications
	var analysisErrors []mail.AnalysisError

	// Build a set of keys that were successfully parsed
	validKeys := make(map[mail.MessageKey]bool, len(validClassifications))
	for _, c := range validClassifications {
		validKeys[c.Key] = true
	}

	// Determine which input messages had invalid analyses
	// We need to match against the original request messages
	for _, msg := range request.Messages {
		if !validKeys[msg.Key] {
			analysisErrors = append(analysisErrors, mail.AnalysisError{
				Key:   msg.Key,
				Error: "parse/validation failed: " + parseErr.Error(),
				Stage: "parse",
			})
		}
	}

	// If we have all items valid, no need for repair
	if len(analysisErrors) == 0 {
		return validClassifications, nil, nil, nil
	}

	// If repair is disabled, mark all failed items as failed classification
	if maxAttempts == 0 {
		// Create failed classifications for each error
		var failedClassifications []mail.Classification
		for _, msg := range request.Messages {
			if !validKeys[msg.Key] {
				failedClassifications = append(failedClassifications, p.makeFailedClassification(msg.Key))
			}
		}
		return validClassifications, failedClassifications, analysisErrors, nil
	}

	// Attempt repair
	log.InfoContext(ctx, "LLM response had invalid items, attempting repair",
		slog.Int("valid", len(validClassifications)),
		slog.Int("invalid", len(analysisErrors)),
	)
	repairedRaw, repairErr := p.repairOnce(ctx, resp.RawResponse, parseErr, validLabels, log)
	if repairErr != nil {
		log.WarnContext(ctx, "LLM repair failed", slog.Any("error", repairErr))
		// Repair failed - mark remaining as failed
		var failedClassifications []mail.Classification
		for _, msg := range request.Messages {
			if !validKeys[msg.Key] {
				failedClassifications = append(failedClassifications, p.makeFailedClassification(msg.Key))
			}
		}
		// Update analysis errors to reflect repair failure
		for i := range analysisErrors {
			analysisErrors[i].Stage = "repair"
			analysisErrors[i].Error = "repair failed: " + repairErr.Error()
		}
		return validClassifications, failedClassifications, analysisErrors, nil
	}

	// Parse repaired response
	repairedResult, repairedErr := llm.ParseResponse(repairedRaw, validLabels)
	if repairedErr != nil {
		log.WarnContext(ctx, "LLM repair produced invalid response", slog.Any("error", repairedErr))
		var failedClassifications []mail.Classification
		for _, msg := range request.Messages {
			if !validKeys[msg.Key] {
				failedClassifications = append(failedClassifications, p.makeFailedClassification(msg.Key))
			}
		}
		for i := range analysisErrors {
			analysisErrors[i].Stage = "repair"
			analysisErrors[i].Error = "repair produced invalid response: " + repairedErr.Error()
		}
		return validClassifications, failedClassifications, analysisErrors, nil
	}

	// Merge repaired classifications with previously valid ones
	repairedKeys := make(map[mail.MessageKey]bool, len(repairedResult.Classifications))
	for _, c := range repairedResult.Classifications {
		repairedKeys[c.Key] = true
	}

	// Find newly valid items from repair
	for _, c := range repairedResult.Classifications {
		if !validKeys[c.Key] {
			validClassifications = append(validClassifications, c)
			validKeys[c.Key] = true
		}
	}

	// Build final failed list for items still not valid
	var failedClassifications []mail.Classification
	var finalAnalysisErrors []mail.AnalysisError
	for _, msg := range request.Messages {
		if !validKeys[msg.Key] {
			failedClassifications = append(failedClassifications, p.makeFailedClassification(msg.Key))
			finalAnalysisErrors = append(finalAnalysisErrors, mail.AnalysisError{
				Key:   msg.Key,
				Error: "repair did not produce valid analysis for this item",
				Stage: "repair",
			})
		}
	}

	return validClassifications, failedClassifications, finalAnalysisErrors, nil
}

// buildDigestDataPartial constructs DigestData from messages, valid classifications,
// failed classifications, and analysis errors. It extends buildDigestData with
// per-message analysis failure tracking.
func (p *Pipeline) buildDigestDataPartial(ctx context.Context, runID string, messages []mail.Message, validClassifications []mail.Classification, failedClassifications []mail.Classification, analysisErrors []mail.AnalysisError, fetchResults []mail.FetchAllResult, accountErrors map[string]error) digest.DigestData {
	// Build a lookup map from valid classifications
	classMap := make(map[mail.MessageKey]mail.Classification, len(validClassifications))
	for _, c := range validClassifications {
		classMap[c.Key] = c
	}
	// Add failed classifications with analysis errors attached
	// Note: analysisErrors slice doesn't have keys directly. We need to match by position.
	// The analysisErrors correspond to the failedClassifications in order.
	for i, fc := range failedClassifications {
		if i < len(analysisErrors) {
			fc.AnalysisError = &analysisErrors[i]
			classMap[fc.Key] = fc
		} else {
			// Fallback: no specific error, create generic
			fc.AnalysisError = &mail.AnalysisError{
				Key:   fc.Key,
				Error: "analysis failed",
				Stage: "unknown",
			}
			classMap[fc.Key] = fc
		}
	}

	entries := make([]digest.MessageEntry, 0, len(messages))
	analysisFailedCount := 0
	analysisFailedPerAccount := make(map[string]int)
	for _, m := range messages {
		c, ok := classMap[m.Key()]
		if !ok {
			// No classification at all (shouldn't happen with our logic)
			c = p.makeFailedClassification(m.Key())
		}
		if c.AnalysisError != nil {
			analysisFailedCount++
			analysisFailedPerAccount[m.AccountLabel]++
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

	globalStats, accountStats, globalSenderCounts, globalDomainCounts := buildDigestStats(messages, entries, validClassifications, fetchResults, accountErrors)
	// Override analysis failed counts
	globalStats.AnalysisFailedCount = analysisFailedCount
	for i := range accountStats {
		accountStats[i].AnalysisFailedCount = analysisFailedPerAccount[accountStats[i].AccountLabel]
	}

	highlights := p.buildHighlights(ctx, runID, messages, globalStats, accountStats, globalSenderCounts, globalDomainCounts)

	return digest.DigestData{
		RunID:               runID,
		GeneratedAt:         p.now(),
		Messages:            entries,
		TotalFetched:        globalStats.FetchedCount,
		TotalClassified:     globalStats.ClassifiedCount,
		FailedCount:         globalStats.FailedCount,
		AnalysisFailedCount: analysisFailedCount,
		GlobalStats:         globalStats,
		AccountStats:        accountStats,
		Highlights:          highlights,
	}
}

func buildDigestStats(messages []mail.Message, entries []digest.MessageEntry, classifications []mail.Classification, fetchResults []mail.FetchAllResult, accountErrors map[string]error) (digest.DigestStats, []digest.AccountStats, map[string]int, map[string]int) {
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

	// Track sender/domain frequencies globally and per-account.
	globalSenderCounts := make(map[string]int)
	globalDomainCounts := make(map[string]int)
	accountSenderCounts := make(map[string]map[string]int)
	accountDomainCounts := make(map[string]map[string]int)

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
		trackMessageSenders(m, globalSenderCounts, globalDomainCounts,
			accountSenderCounts, accountDomainCounts)
	}

	processClassificationEntries(entries, &global, ensureAccount, classifiedKeys)

	global.AccountsChecked = len(accountOrder)
	for _, label := range accountOrder {
		if accountByLabel[label].Status == "error" {
			global.AccountsFailed++
		}
	}
	global.AccountsSucceeded = global.AccountsChecked - global.AccountsFailed

	// Compute top 5 senders/domains.
	global.TopSenders = topN(globalSenderCounts, 5)
	global.TopDomains = topN(globalDomainCounts, 5)
	for _, label := range accountOrder {
		stats := accountByLabel[label]
		stats.TopSenders = topN(accountSenderCounts[label], 5)
		stats.TopDomains = topN(accountDomainCounts[label], 5)
	}

	accounts := make([]digest.AccountStats, 0, len(accountOrder))
	for _, label := range accountOrder {
		accounts = append(accounts, *accountByLabel[label])
	}
	return global, accounts, globalSenderCounts, globalDomainCounts
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

// ---------------------------------------------------------------------------
// Sender/domain helpers
// ---------------------------------------------------------------------------

// parseSender extracts the sender address and domain from a From header value.
// It handles "addr", "Name <addr>", and malformed inputs gracefully.
func parseSender(from string) (sender, domain string) {
	addr := extractAddress(from)
	if addr == "" {
		return "", ""
	}
	if idx := strings.LastIndex(addr, "@"); idx > 0 && idx < len(addr)-1 {
		return addr, strings.ToLower(addr[idx+1:])
	}
	return "", ""
}

// extractAddress extracts an email address from a From header value.
// Supports "addr", "Name <addr>", `"Name" <addr>`, and unclosed brackets.
func extractAddress(from string) string {
	from = strings.TrimSpace(from)
	if from == "" {
		return ""
	}
	// Look for angle-bracket pattern: anything <addr> or <addr (unclosed).
	if open := strings.LastIndex(from, "<"); open >= 0 {
		close := strings.LastIndex(from, ">")
		if close < 0 {
			close = len(from)
		}
		if close > open {
			return strings.TrimSpace(from[open+1 : close])
		}
	}
	// No angle brackets — treat the whole string as the address.
	return from
}

// processClassificationEntries iterates classification entries, populating
// label counts, classified/failed counts, and high-priority tracking.
func processClassificationEntries(entries []digest.MessageEntry, global *digest.DigestStats, ensureAccount func(string) *digest.AccountStats, classifiedKeys map[mail.MessageKey]bool) {
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
			if strings.EqualFold(entry.Classification.Priority, "high") {
				global.HighPriorityCount++
			}
		} else {
			global.FailedCount++
			stats.FailedCount++
		}
	}
}

// trackMessageSenders parses sender/domain from a single message and
// records counts into the provided frequency maps.
func trackMessageSenders(m mail.Message, globalSenderCounts, globalDomainCounts map[string]int, accountSenderCounts, accountDomainCounts map[string]map[string]int) {
	sender, domain := parseSender(m.From)
	if sender != "" {
		globalSenderCounts[sender]++
		if accountSenderCounts[m.AccountLabel] == nil {
			accountSenderCounts[m.AccountLabel] = make(map[string]int)
		}
		accountSenderCounts[m.AccountLabel][sender]++
	}
	if domain != "" {
		globalDomainCounts[domain]++
		if accountDomainCounts[m.AccountLabel] == nil {
			accountDomainCounts[m.AccountLabel] = make(map[string]int)
		}
		accountDomainCounts[m.AccountLabel][domain]++
	}
}

// topN returns the top N entries from a frequency map, sorted by count
// descending, formatted as "key (count)".
func topN(counts map[string]int, n int) []string {
	type kv struct {
		key   string
		count int
	}
	sorted := make([]kv, 0, len(counts))
	for k, v := range counts {
		sorted = append(sorted, kv{k, v})
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].count != sorted[j].count {
			return sorted[i].count > sorted[j].count
		}
		return sorted[i].key < sorted[j].key
	})
	if len(sorted) > n {
		sorted = sorted[:n]
	}
	result := make([]string, len(sorted))
	for i, kv := range sorted {
		result[i] = fmt.Sprintf("%s (%d)", kv.key, kv.count)
	}
	return result
}

// ---------------------------------------------------------------------------
// buildHighlights
// ---------------------------------------------------------------------------

// buildHighlights generates a deterministic list of notable observations for
// the current run by comparing current stats against the previous completed
// run's snapshot from the store.
//
// The highlights are ordered by priority: high-priority messages, failed
// accounts, label deltas, sender bursts. The returned slice is always
// non-nil; an empty slice means "nothing notable this run" and the
// renderer will show a neutral placeholder.
func (p *Pipeline) buildHighlights(ctx context.Context, runID string, messages []mail.Message, globalStats digest.DigestStats, accountStats []digest.AccountStats, globalSenderCounts, globalDomainCounts map[string]int) []string {
	highlights := make([]string, 0)

	// 1. High-priority emails present
	if globalStats.HighPriorityCount > 0 {
		highlights = append(highlights, fmt.Sprintf("%d high-priority email%s require attention", globalStats.HighPriorityCount, plural(globalStats.HighPriorityCount)))
	}

	// 2. Accounts with fetch errors
	for _, acct := range accountStats {
		if acct.Status == "error" {
			errMsg := acct.Error
			if len(errMsg) > 80 {
				errMsg = errMsg[:77] + "..."
			}
			highlights = append(highlights, fmt.Sprintf("Account %q failed: %s", acct.AccountLabel, errMsg))
		}
	}

	// 3. Delta vs previous run (if we have a prior snapshot)
	prev, err := p.store.GetPreviousRunDigestSummary(ctx, runID)
	if err != nil {
		p.logger.WarnContext(ctx, "failed to fetch previous run summary for highlights", slog.Any("error", err))
	} else if prev != nil {
		// 3a. Ads increase
		prevAds := prev.CountsByLabel["Ads"]
		currAds := globalStats.CountsByLabel["Ads"]
		if currAds > prevAds && prevAds > 0 {
			highlights = append(highlights, fmt.Sprintf("Advertisements up by %d (was %d, now %d)", currAds-prevAds, prevAds, currAds))
		}

		// 3b. Same-sender burst (compare sender counts from current messages vs previous run)
		for sender, currCount := range globalSenderCounts {
			prevCount := prev.SenderCounts[sender]
			if currCount >= 3 && currCount > prevCount {
				highlights = append(highlights, fmt.Sprintf("Burst from %s: %d messages (was %d)", sender, currCount, prevCount))
			}
		}

		// 3c. Failed account increase
		if globalStats.AccountsFailed > prev.AccountsFailed && prev.AccountsFailed >= 0 {
			highlights = append(highlights, fmt.Sprintf("Account failures increased: %d (was %d)", globalStats.AccountsFailed, prev.AccountsFailed))
		}
	}

	return highlights
}

// plural returns "s" if n != 1, else empty string.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
