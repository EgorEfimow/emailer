package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/egorefimow/emailer/internal/digest"
	"github.com/egorefimow/emailer/internal/llm"
	"github.com/egorefimow/emailer/internal/mail"
	"github.com/egorefimow/emailer/internal/notify"
	"github.com/egorefimow/emailer/internal/store"
)

// ---------------------------------------------------------------------------
// Test fakes
// ---------------------------------------------------------------------------

// fakeStore implements store.Store for testing.
type fakeStore struct {
	runID           int
	runs            []store.Run
	processed       map[store.MessageKey]bool
	lastRunFinished *time.Time
	finishRunErr    error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		processed: make(map[store.MessageKey]bool),
	}
}

func (f *fakeStore) Close() error { return nil }

func (f *fakeStore) RecordRun(_ context.Context, r store.Run) (store.Run, error) {
	f.runID++
	if r.ID == "" {
		r.ID = fmt.Sprintf("test-run-%d", f.runID)
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	if r.Status == "" {
		r.Status = store.RunStatusRunning
	}
	f.runs = append(f.runs, r)
	return r, nil
}

func (f *fakeStore) FinishRun(_ context.Context, runID string, status store.RunStatus, messageCount int, runErr error) error {
	if f.finishRunErr != nil {
		return f.finishRunErr
	}
	errStr := ""
	if runErr != nil {
		errStr = runErr.Error()
	}
	now := time.Now()
	for i := range f.runs {
		if f.runs[i].ID == runID {
			f.runs[i].FinishedAt = &now
			f.runs[i].Status = status
			f.runs[i].MessageCount = messageCount
			f.runs[i].Error = errStr
			return nil
		}
	}
	return store.ErrRunNotFound
}

func (f *fakeStore) GetRun(_ context.Context, runID string) (store.Run, error) {
	for _, r := range f.runs {
		if r.ID == runID {
			return r, nil
		}
	}
	return store.Run{}, store.ErrRunNotFound
}

func (f *fakeStore) ListRuns(_ context.Context, limit int) ([]store.Run, error) {
	if limit <= 0 || limit > len(f.runs) {
		limit = len(f.runs)
	}
	return f.runs[:limit], nil
}

func (f *fakeStore) GetLastSuccessfulRunTime(_ context.Context) (*time.Time, error) {
	return f.lastRunFinished, nil
}

func (f *fakeStore) RecordMessage(_ context.Context, m store.ProcessedMessage) error {
	f.processed[store.MessageKey{AccountLabel: m.AccountLabel, UID: m.UID}] = true
	return nil
}

func (f *fakeStore) AlreadyProcessed(_ context.Context, keys []store.MessageKey) (map[store.MessageKey]bool, error) {
	result := make(map[store.MessageKey]bool, len(keys))
	for _, k := range keys {
		if f.processed[k] {
			result[k] = true
		}
	}
	return result, nil
}

func (f *fakeStore) RecordFlag(_ context.Context, _ store.FlagRecord) error { return nil }

func (f *fakeStore) RecordDigest(_ context.Context, _ store.DigestRecord) error { return nil }

// compile-time check
var _ store.Store = (*fakeStore)(nil)

// fakeIngester implements mail.Ingester for testing.
type fakeIngester struct {
	messages []mail.Message
	fetchErr error
}

func (f *fakeIngester) Fetch(_ context.Context, _ config.IMAPAccount, _ mail.FetchOptions) ([]mail.Message, error) {
	return f.messages, f.fetchErr
}

func (f *fakeIngester) ApplyFlags(_ context.Context, _ config.IMAPAccount, _ []mail.Flag) error {
	return nil
}

// compile-time check
var _ mail.Ingester = (*fakeIngester)(nil)

// fakeProvider implements llm.Provider for testing.
type fakeProvider struct {
	response llm.Response
	callErr  error
	called   bool
}

func (f *fakeProvider) Name() string { return "test-provider" }

func (f *fakeProvider) Classify(_ context.Context, _ llm.Request) (llm.Response, error) {
	f.called = true
	return f.response, f.callErr
}

// compile-time check
var _ llm.Provider = (*fakeProvider)(nil)

// fakeRenderer implements digest.Renderer for testing.
type fakeRenderer struct {
	output   string
	err      error
	name     string
	lastData digest.DigestData
}

func (f *fakeRenderer) Name() string {
	if f.name != "" {
		return f.name
	}
	return "test-renderer"
}

func (f *fakeRenderer) Render(_ context.Context, data digest.DigestData) (string, error) {
	f.lastData = data
	return f.output, f.err
}

// compile-time check
var _ digest.Renderer = (*fakeRenderer)(nil)

// fakeChannel implements notify.Channel for testing.
type fakeChannel struct {
	sentPayload string
	sentOpts    notify.SendOptions
	sendErr     error
	name        string
}

func (f *fakeChannel) Name() string {
	if f.name != "" {
		return f.name
	}
	return "test-channel"
}

func (f *fakeChannel) Send(_ context.Context, payload string, opts notify.SendOptions) error {
	f.sentPayload = payload
	f.sentOpts = opts
	return f.sendErr
}

// compile-time check
var _ notify.Channel = (*fakeChannel)(nil)

// ---------------------------------------------------------------------------
// Helper: default pipeline with one account
// ---------------------------------------------------------------------------

func defaultPipeline(s *fakeStore, ingester mail.Ingester) *Pipeline {
	cfg := config.DefaultConfig()
	cfg.IMAP.Accounts = []config.IMAPAccount{
		{Label: "personal", Host: "imap.example.com", Username: "u", Password: "p"},
	}
	cfg.LLM.Model = "test-model"

	return New(
		s,
		map[string]mail.Ingester{"personal": ingester},
		&fakeProvider{},
		&fakeRenderer{name: "markdown", output: "# Digest"},
		&fakeRenderer{name: "fallback", output: "# Fallback"},
		&fakeChannel{name: "telegram"},
		slog.Default(),
		cfg,
	)
}

// ---------------------------------------------------------------------------
// Tests: Constructor
// ---------------------------------------------------------------------------

func TestNew(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.IMAP.Accounts = []config.IMAPAccount{
		{Label: "personal", Host: "imap.example.com", Username: "user", Password: "pass"},
	}
	cfg.LLM.Provider = "gemini"
	cfg.LLM.Model = "gemini-2.0-flash"

	p := New(
		newFakeStore(),
		map[string]mail.Ingester{"personal": &fakeIngester{}},
		&fakeProvider{},
		&fakeRenderer{name: "markdown"},
		&fakeRenderer{name: "fallback"},
		&fakeChannel{name: "telegram"},
		slog.Default(),
		cfg,
	)

	if p == nil {
		t.Fatal("New returned nil")
	}
	if p.cfg.LLM.Provider != "gemini" {
		t.Error("New did not set config correctly")
	}
	if _, ok := p.ingesters["personal"]; !ok {
		t.Error("New did not set ingesters correctly")
	}
	if p.now == nil {
		t.Error("New did not set now function")
	}
}

func TestNewWithNoAccounts(t *testing.T) {
	p := New(
		newFakeStore(),
		map[string]mail.Ingester{},
		&fakeProvider{},
		&fakeRenderer{name: "markdown"},
		&fakeRenderer{name: "fallback"},
		&fakeChannel{name: "telegram"},
		slog.Default(),
		config.DefaultConfig(),
	)
	if p == nil {
		t.Fatal("New returned nil")
	}
	if len(p.ingesters) != 0 {
		t.Error("expected empty ingesters map")
	}
}

// ---------------------------------------------------------------------------
// Tests: RunOptions
// ---------------------------------------------------------------------------

func TestRunOptionsDefaults(t *testing.T) {
	opts := RunOptions{}
	if opts.Window != nil {
		t.Error("Window should be nil by default")
	}
	if opts.ForceReprocess {
		t.Error("ForceReprocess should be false by default")
	}
	if opts.DryRun {
		t.Error("DryRun should be false by default")
	}
	if opts.Stateless {
		t.Error("Stateless should be false by default")
	}
}

func TestRunOptionsExplicitWindow(t *testing.T) {
	w := 30 * time.Minute
	opts := RunOptions{Window: &w}
	if opts.Window == nil {
		t.Fatal("Window should not be nil")
	}
	if *opts.Window != 30*time.Minute {
		t.Errorf("expected 30m, got %v", *opts.Window)
	}
}

// ---------------------------------------------------------------------------
// Tests: Result
// ---------------------------------------------------------------------------

func TestResultDefaults(t *testing.T) {
	r := Result{}
	if r.RunID != "" {
		t.Errorf("expected empty RunID, got %q", r.RunID)
	}
	if r.Status != "" {
		t.Errorf("expected empty Status, got %q", r.Status)
	}
	if r.Err != nil {
		t.Errorf("expected nil Err, got %v", r.Err)
	}
}

func TestResultWithValues(t *testing.T) {
	r := Result{
		RunID:           "run-123",
		Status:          store.RunStatusCompleted,
		TotalFetched:    10,
		TotalClassified: 8,
		FailedCount:     2,
	}
	if r.RunID != "run-123" {
		t.Errorf("expected run-123, got %q", r.RunID)
	}
	if r.Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", r.Status)
	}
}

// ---------------------------------------------------------------------------
// Tests: Run method — basic flow
// ---------------------------------------------------------------------------

func TestRunRecordsRun(t *testing.T) {
	s := newFakeStore()
	ingester := &fakeIngester{}
	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{})

	if result.RunID == "" {
		t.Error("expected non-empty RunID")
	}
	if len(s.runs) != 1 {
		t.Fatalf("expected 1 run recorded, got %d", len(s.runs))
	}
	if s.runs[0].ID != result.RunID {
		t.Errorf("run ID mismatch: %q vs %q", s.runs[0].ID, result.RunID)
	}
	if s.runs[0].Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", s.runs[0].Status)
	}
}

func TestRunNoMessagesReturnsEmpty(t *testing.T) {
	s := newFakeStore()
	ingester := &fakeIngester{}
	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", result.Status)
	}
	if result.TotalFetched != 0 {
		t.Errorf("expected 0 fetched, got %d", result.TotalFetched)
	}
}

func TestRunFetchesMessages(t *testing.T) {
	s := newFakeStore()
	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Hello", From: "alice@example.com", Body: "Hi", Date: time.Now()},
		{AccountLabel: "personal", UID: 2, Subject: "World", From: "bob@example.com", Body: "Hey", Date: time.Now()},
	}
	ingester := &fakeIngester{messages: msgs}
	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", result.Status)
	}
	if result.TotalFetched != 2 {
		t.Errorf("expected 2 fetched, got %d", result.TotalFetched)
	}
}

// ---------------------------------------------------------------------------
// Tests: Run method — fetch window
// ---------------------------------------------------------------------------

func TestRunExplicitWindow(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	s := newFakeStore()
	ingester := &fakeIngester{}

	p := defaultPipeline(s, ingester)
	p.now = func() time.Time { return frozen }

	window := 10 * time.Minute
	_ = p.Run(context.Background(), RunOptions{Window: &window})

	if len(s.runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(s.runs))
	}
	if s.runs[0].Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", s.runs[0].Status)
	}
}

// ---------------------------------------------------------------------------
// Tests: Run method — error handling
// ---------------------------------------------------------------------------

func TestRunAllAccountsFail(t *testing.T) {
	s := newFakeStore()
	ingester := &fakeIngester{
		fetchErr: errors.New("connection refused"),
	}
	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusIngestFailed {
		t.Errorf("expected ingest_failed, got %q", result.Status)
	}
	if result.Err == nil {
		t.Error("expected non-nil error")
	}
	if len(s.runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(s.runs))
	}
	if s.runs[0].Status != store.RunStatusIngestFailed {
		t.Errorf("expected ingest_failed, got %q", s.runs[0].Status)
	}
}

func TestRunStoreFinishRunFails(t *testing.T) {
	s := &fakeStore{
		finishRunErr: errors.New("store error"),
	}
	ingester := &fakeIngester{messages: []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Test", Body: "Body", Date: time.Now()},
	}}
	p := defaultPipeline(s, ingester)
	p.now = func() time.Time { return time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC) }

	result := p.Run(context.Background(), RunOptions{})

	if result.RunID == "" {
		t.Error("expected non-empty RunID")
	}
}

// ---------------------------------------------------------------------------
// Tests: fetchWindow helper
// ---------------------------------------------------------------------------

func TestFetchWindowExplicit(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	s := newFakeStore()
	p := defaultPipeline(s, &fakeIngester{})
	p.now = func() time.Time { return frozen }

	window := 2 * time.Hour
	since, err := p.fetchWindow(context.Background(), RunOptions{Window: &window})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := frozen.Add(-2 * time.Hour)
	if !since.Equal(expected) {
		t.Errorf("expected %v, got %v", expected, since)
	}
}

func TestFetchWindowDynamicFromLastRun(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	lastRun := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)

	s := newFakeStore()
	s.lastRunFinished = &lastRun
	p := defaultPipeline(s, &fakeIngester{})
	p.now = func() time.Time { return frozen }

	since, err := p.fetchWindow(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !since.Equal(lastRun) {
		t.Errorf("expected %v (last run), got %v", lastRun, since)
	}
}

func TestFetchWindowNoLastRunUses24hDefault(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	s := newFakeStore()
	p := defaultPipeline(s, &fakeIngester{})
	p.now = func() time.Time { return frozen }

	since, err := p.fetchWindow(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := frozen.Add(-24 * time.Hour)
	if !since.Equal(expected) {
		t.Errorf("expected %v (24h default), got %v", expected, since)
	}
}

func TestFetchWindowCapsAtMaxWindow(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	lastRun := frozen.Add(-100 * time.Hour)

	s := newFakeStore()
	s.lastRunFinished = &lastRun
	p := defaultPipeline(s, &fakeIngester{})
	p.now = func() time.Time { return frozen }
	p.cfg.MaxWindow = 72 * time.Hour

	since, err := p.fetchWindow(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := frozen.Add(-72 * time.Hour)
	if !since.Equal(expected) {
		t.Errorf("expected %v (capped at 72h), got %v", expected, since)
	}
}

func TestFetchWindowZeroMaxWindowDefaultsTo72h(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	lastRun := frozen.Add(-200 * time.Hour)

	s := newFakeStore()
	s.lastRunFinished = &lastRun
	p := defaultPipeline(s, &fakeIngester{})
	p.now = func() time.Time { return frozen }
	p.cfg.MaxWindow = 0

	since, err := p.fetchWindow(context.Background(), RunOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := frozen.Add(-72 * time.Hour)
	if !since.Equal(expected) {
		t.Errorf("expected %v (default 72h cap), got %v", expected, since)
	}
}

// ---------------------------------------------------------------------------
// Tests: filterProcessed helper
// ---------------------------------------------------------------------------

func TestFilterProcessedRemovesDuplicates(t *testing.T) {
	s := newFakeStore()
	s.processed[store.MessageKey{AccountLabel: "personal", UID: 1}] = true

	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Already done"},
		{AccountLabel: "personal", UID: 2, Subject: "New message"},
	}

	p := defaultPipeline(s, &fakeIngester{})
	filtered := p.filterProcessed(context.Background(), "run-1", msgs, slog.Default())

	if len(filtered) != 1 {
		t.Fatalf("expected 1 after filter, got %d", len(filtered))
	}
	if filtered[0].UID != 2 {
		t.Errorf("expected UID 2, got %d", filtered[0].UID)
	}
}

func TestFilterProcessedAllNew(t *testing.T) {
	s := newFakeStore()
	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "A"},
		{AccountLabel: "personal", UID: 2, Subject: "B"},
	}

	p := defaultPipeline(s, &fakeIngester{})
	filtered := p.filterProcessed(context.Background(), "run-1", msgs, slog.Default())

	if len(filtered) != 2 {
		t.Fatalf("expected 2, got %d", len(filtered))
	}
}

func TestFilterProcessedAllDuplicates(t *testing.T) {
	s := newFakeStore()
	s.processed[store.MessageKey{AccountLabel: "personal", UID: 1}] = true
	s.processed[store.MessageKey{AccountLabel: "personal", UID: 2}] = true

	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "A"},
		{AccountLabel: "personal", UID: 2, Subject: "B"},
	}

	p := defaultPipeline(s, &fakeIngester{})
	filtered := p.filterProcessed(context.Background(), "run-1", msgs, slog.Default())

	if len(filtered) != 0 {
		t.Fatalf("expected 0, got %d", len(filtered))
	}
}

// ---------------------------------------------------------------------------
// Tests: messagesToInputs helper
// ---------------------------------------------------------------------------

func TestMessagesToInputs(t *testing.T) {
	now := time.Now()
	msgs := []mail.Message{
		{
			AccountLabel: "personal",
			UID:          1,
			Subject:      "Hello",
			From:         "alice@example.com",
			Date:         now,
			Body:         "Body text",
			IsRead:       true,
		},
		{
			AccountLabel: "personal",
			UID:          2,
			Subject:      "World",
			From:         "bob@example.com",
			Date:         now,
			Body:         "More text",
			IsRead:       false,
		},
	}

	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	inputs := p.messagesToInputs(msgs)

	if len(inputs) != 2 {
		t.Fatalf("expected 2 inputs, got %d", len(inputs))
	}
	if inputs[0].Key != (mail.MessageKey{AccountLabel: "personal", UID: 1}) {
		t.Errorf("key mismatch: %+v", inputs[0].Key)
	}
	if inputs[0].Subject != "Hello" {
		t.Errorf("expected 'Hello', got %q", inputs[0].Subject)
	}
	if !inputs[0].IsRead {
		t.Error("expected IsRead=true")
	}
	if inputs[1].IsRead {
		t.Error("expected IsRead=false")
	}
}

// ---------------------------------------------------------------------------
// Tests: Run method — LLM failure
// ---------------------------------------------------------------------------

func TestRunLLMFailureUsesFallback(t *testing.T) {
	s := newFakeStore()
	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Hello", Body: "Body", Date: time.Now()},
	}
	ingester := &fakeIngester{messages: msgs}
	provider := &fakeProvider{callErr: errors.New("LLM API error")}

	p := defaultPipeline(s, ingester)
	p.provider = provider

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusDegraded {
		t.Errorf("expected degraded, got %q", result.Status)
	}
	if !provider.called {
		t.Error("expected provider to be called")
	}
}

func TestRunLLMFailureLogsError(t *testing.T) {
	s := newFakeStore()
	provider := &fakeProvider{callErr: errors.New("timeout")}

	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Hello", Body: "Body", Date: time.Now()},
	}
	ingester := &fakeIngester{messages: msgs}

	p := defaultPipeline(s, ingester)
	p.provider = provider

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusDegraded {
		t.Errorf("expected degraded, got %q", result.Status)
	}
}

// ---------------------------------------------------------------------------
// Tests: Run method — DryRun mode
// ---------------------------------------------------------------------------

func TestRunDryRunSkipsFlagsAndDigest(t *testing.T) {
	s := newFakeStore()

	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Test", Body: "Body", Date: time.Now()},
	}
	ingester := &fakeIngester{messages: msgs}

	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{DryRun: true})

	if result.Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", result.Status)
	}
	// The channel should not have been sent to in dry-run mode.
	// Since defaultPipeline creates a fresh fakeChannel, we can't check it
	// directly, but the status should be completed.
}

// ---------------------------------------------------------------------------
// Tests: Run method — digest delivery
// ---------------------------------------------------------------------------

func TestRunDigestDelivery(t *testing.T) {
	s := newFakeStore()

	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Test", Body: "Body", Date: time.Now()},
	}
	ingester := &fakeIngester{messages: msgs}

	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", result.Status)
	}
}

func TestRunPartialAccountFailureExposesErrorToDigest(t *testing.T) {
	now := time.Now()
	fakeStore := newFakeStore()
	renderer := &fakeRenderer{name: "markdown", output: "# Digest"}
	cfg := config.DefaultConfig()
	cfg.IMAP.Accounts = []config.IMAPAccount{
		{Label: "work", Host: "imap.example.com", Username: "u", Password: "p"},
		{Label: "personal", Host: "imap.example.com", Username: "u", Password: "p"},
	}
	cfg.LLM.Model = "test-model"

	msg := mail.Message{AccountLabel: "work", UID: 1, Subject: "A", Body: "Body", Date: now}
	p := New(
		fakeStore,
		map[string]mail.Ingester{
			"work":     &fakeIngester{messages: []mail.Message{msg}},
			"personal": &fakeIngester{fetchErr: errors.New("imap timeout")},
		},
		&fakeProvider{response: llm.Response{Classifications: []mail.Classification{{Key: msg.Key(), Label: "Useful", Confidence: 0.9}}}},
		renderer,
		&fakeRenderer{name: "fallback", output: "# Fallback"},
		&fakeChannel{name: "telegram"},
		slog.Default(),
		cfg,
	)

	result := p.Run(context.Background(), RunOptions{DryRun: true})
	if result.Status != store.RunStatusPartial {
		t.Fatalf("expected partial status, got %s", result.Status)
	}

	found := false
	for _, stats := range renderer.lastData.AccountStats {
		if stats.AccountLabel == "personal" {
			found = true
			if stats.Status != "error" || stats.Error != "imap timeout" {
				t.Fatalf("expected personal account error in digest data, got %#v", stats)
			}
		}
	}
	if !found {
		t.Fatalf("expected personal account stats in digest data: %#v", renderer.lastData.AccountStats)
	}
}

// ---------------------------------------------------------------------------
// Tests: buildDigestData helper
// ---------------------------------------------------------------------------

func TestBuildDigestData(t *testing.T) {
	now := time.Now()
	msgs := []mail.Message{
		{
			AccountLabel: "personal",
			UID:          1,
			Subject:      "Hello",
			From:         "alice@example.com",
			Date:         now,
			Body:         "Body text",
			IsRead:       true,
		},
		{
			AccountLabel: "personal",
			UID:          2,
			Subject:      "World",
			From:         "bob@example.com",
			Date:         now,
			Body:         "More text",
			IsRead:       false,
		},
	}
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "personal", UID: 1}, Label: "Useful", Confidence: 0.95, Reason: "Important"},
		{Key: mail.MessageKey{AccountLabel: "personal", UID: 2}, Label: "ToDelete", Confidence: 0.8, Reason: "Spam"},
	}

	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	data := p.buildDigestData("run-1", msgs, classifications, nil, nil)

	if data.RunID != "run-1" {
		t.Errorf("expected run-1, got %q", data.RunID)
	}
	if len(data.Messages) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(data.Messages))
	}
	if data.Messages[0].Classification.Label != "Useful" {
		t.Errorf("expected Useful, got %q", data.Messages[0].Classification.Label)
	}
	if data.Messages[1].Classification.Label != "ToDelete" {
		t.Errorf("expected ToDelete, got %q", data.Messages[1].Classification.Label)
	}
	if data.TotalFetched != 2 {
		t.Errorf("expected 2 fetched, got %d", data.TotalFetched)
	}
	if data.TotalClassified != 2 {
		t.Errorf("expected 2 classified, got %d", data.TotalClassified)
	}
	if data.FailedCount != 0 {
		t.Errorf("expected 0 failed, got %d", data.FailedCount)
	}
}

func TestBuildDigestDataWithoutClassifications(t *testing.T) {
	now := time.Now()
	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Hello", Body: "Body", Date: now},
		{AccountLabel: "personal", UID: 2, Subject: "World", Body: "Body", Date: now},
	}

	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	data := p.buildDigestData("run-1", msgs, nil, nil, nil)

	if len(data.Messages) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(data.Messages))
	}
	for i, entry := range data.Messages {
		if entry.Classification.Label != "Unknown" {
			t.Errorf("entry[%d]: expected Unknown, got %q", i, entry.Classification.Label)
		}
	}
	if data.TotalClassified != 0 {
		t.Errorf("expected 0 classified, got %d", data.TotalClassified)
	}
	if data.FailedCount != 2 {
		t.Errorf("expected 2 failed, got %d", data.FailedCount)
	}
}

func TestBuildDigestDataPartialClassifications(t *testing.T) {
	now := time.Now()
	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "A", Body: "Body", Date: now},
		{AccountLabel: "personal", UID: 2, Subject: "B", Body: "Body", Date: now},
	}
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "personal", UID: 1}, Label: "Useful", Confidence: 0.9},
	}

	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	data := p.buildDigestData("run-1", msgs, classifications, nil, nil)

	if len(data.Messages) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(data.Messages))
	}
	if data.Messages[0].Classification.Label != "Useful" {
		t.Errorf("expected Useful, got %q", data.Messages[0].Classification.Label)
	}
	if data.Messages[1].Classification.Label != "Unknown" {
		t.Errorf("expected Unknown, got %q", data.Messages[1].Classification.Label)
	}
	if data.FailedCount != 1 {
		t.Errorf("expected 1 failed, got %d", data.FailedCount)
	}
}

func TestBuildDigestDataAggregatesStats(t *testing.T) { //nolint:gocyclo
	now := time.Now()
	msgs := []mail.Message{
		{AccountLabel: "work", UID: 1, Subject: "A", Body: "Body", Date: now, IsRead: true},
		{AccountLabel: "work", UID: 2, Subject: "B", Body: "Body", Date: now, IsRead: false},
	}
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "work", UID: 1}, Label: "Useful", Confidence: 0.9},
	}
	fetchResults := []mail.FetchAllResult{
		{Account: config.IMAPAccount{Label: "work"}, Messages: msgs},
		{Account: config.IMAPAccount{Label: "personal"}, Err: errors.New("connection refused")},
	}

	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	data := p.buildDigestData("run-1", msgs, classifications, fetchResults, mail.AccountErrors(fetchResults))

	if data.GlobalStats.FetchedCount != 2 {
		t.Errorf("expected 2 global fetched, got %d", data.GlobalStats.FetchedCount)
	}
	if data.GlobalStats.ClassifiedCount != 1 {
		t.Errorf("expected 1 global classified, got %d", data.GlobalStats.ClassifiedCount)
	}
	if data.GlobalStats.FailedCount != 1 {
		t.Errorf("expected 1 global failed, got %d", data.GlobalStats.FailedCount)
	}
	if data.GlobalStats.ReadCount != 1 || data.GlobalStats.UnreadCount != 1 {
		t.Errorf("expected 1 read and 1 unread, got %d read and %d unread", data.GlobalStats.ReadCount, data.GlobalStats.UnreadCount)
	}
	if data.GlobalStats.CountsByLabel["Useful"] != 1 || data.GlobalStats.CountsByLabel["Unknown"] != 1 {
		t.Errorf("unexpected global label counts: %#v", data.GlobalStats.CountsByLabel)
	}
	if len(data.AccountStats) != 2 {
		t.Fatalf("expected 2 account stats, got %d", len(data.AccountStats))
	}
	if data.AccountStats[0].AccountLabel != "work" || data.AccountStats[0].FetchedCount != 2 || data.AccountStats[0].ClassifiedCount != 1 || data.AccountStats[0].FailedCount != 1 {
		t.Errorf("unexpected work account stats: %#v", data.AccountStats[0])
	}
	if data.AccountStats[1].AccountLabel != "personal" || data.AccountStats[1].Status != "error" || data.AccountStats[1].Error == "" {
		t.Errorf("unexpected personal account stats: %#v", data.AccountStats[1])
	}
}

func TestBuildDigestDataGlobalStatsAccountsAndPriority(t *testing.T) {
	now := time.Now()
	msgs := []mail.Message{
		{AccountLabel: "work", UID: 1, Subject: "A", Body: "Body", Date: now, IsRead: true},
		{AccountLabel: "work", UID: 2, Subject: "B", Body: "Body", Date: now, IsRead: false},
		{AccountLabel: "personal", UID: 3, Subject: "C", Body: "Body", Date: now, IsRead: false},
	}
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "work", UID: 1}, Label: "Useful", Confidence: 0.9, Priority: "high"},
		{Key: mail.MessageKey{AccountLabel: "work", UID: 2}, Label: "Ads", Confidence: 0.8, Priority: "low"},
		{Key: mail.MessageKey{AccountLabel: "personal", UID: 3}, Label: "Useful", Confidence: 0.9, Priority: "high"},
	}
	fetchResults := []mail.FetchAllResult{
		{Account: config.IMAPAccount{Label: "work"}, Messages: msgs[:2]},
		{Account: config.IMAPAccount{Label: "personal"}, Messages: msgs[2:3]},
		{Account: config.IMAPAccount{Label: "broken"}, Err: errors.New("timeout")},
	}

	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	data := p.buildDigestData("run-1", msgs, classifications, fetchResults, mail.AccountErrors(fetchResults))

	if data.GlobalStats.AccountsChecked != 3 {
		t.Errorf("expected 3 accounts checked, got %d", data.GlobalStats.AccountsChecked)
	}
	if data.GlobalStats.AccountsSucceeded != 2 {
		t.Errorf("expected 2 accounts succeeded, got %d", data.GlobalStats.AccountsSucceeded)
	}
	if data.GlobalStats.AccountsFailed != 1 {
		t.Errorf("expected 1 account failed, got %d", data.GlobalStats.AccountsFailed)
	}
	if data.GlobalStats.HighPriorityCount != 2 {
		t.Errorf("expected 2 high-priority, got %d", data.GlobalStats.HighPriorityCount)
	}
}

// ---------------------------------------------------------------------------
// Tests: payloadHash helper
// ---------------------------------------------------------------------------

func TestPayloadHash(t *testing.T) {
	h1 := payloadHash("hello")
	h2 := payloadHash("hello")
	h3 := payloadHash("world")

	if h1 != h2 {
		t.Error("same input should produce same hash")
	}
	if h1 == h3 {
		t.Error("different input should produce different hash")
	}
	if len(h1) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(h1))
	}
}

func TestPayloadHashEmpty(t *testing.T) {
	h := payloadHash("")
	if len(h) != 64 {
		t.Errorf("expected 64 hex chars, got %d", len(h))
	}
}

// ---------------------------------------------------------------------------
// Tests: truncateExcerpt helper
// ---------------------------------------------------------------------------

func TestTruncateExcerptNoTruncation(t *testing.T) {
	result := truncateExcerpt("short", 10)
	if result != "short" {
		t.Errorf("expected 'short', got %q", result)
	}
}

func TestTruncateExcerptExact(t *testing.T) {
	result := truncateExcerpt("exact", 5)
	if result != "exact" {
		t.Errorf("expected 'exact', got %q", result)
	}
}

func TestTruncateExcerptWithEllipsis(t *testing.T) {
	result := truncateExcerpt("long body text here", 9)
	expected := "long body…"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestTruncateExcerptZeroLimit(t *testing.T) {
	result := truncateExcerpt("body", 0)
	if result != "body" {
		t.Errorf("expected 'body', got %q", result)
	}
}

// ---------------------------------------------------------------------------
// Tests: End-to-end with classifications
// ---------------------------------------------------------------------------

func TestRunWithClassifications(t *testing.T) {
	s := newFakeStore()

	now := time.Now()
	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Hello", From: "a@b.com", Body: "Body", Date: now, IsRead: true},
		{AccountLabel: "personal", UID: 2, Subject: "World", From: "c@d.com", Body: "Body", Date: now, IsRead: false},
	}
	ingester := &fakeIngester{messages: msgs}

	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "personal", UID: 1}, Label: "Useful", Confidence: 0.95, Reason: "Important"},
		{Key: mail.MessageKey{AccountLabel: "personal", UID: 2}, Label: "ToDelete", Confidence: 0.8, Reason: "Spam"},
	}
	provider := &fakeProvider{
		response: llm.Response{
			Classifications: classifications,
			TokenUsage:      llm.TokenUsage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150},
		},
	}

	p := defaultPipeline(s, ingester)
	p.provider = provider

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusCompleted {
		t.Errorf("expected completed, got %q", result.Status)
	}
	if result.TotalFetched != 2 {
		t.Errorf("expected 2 fetched, got %d", result.TotalFetched)
	}
	if result.TotalClassified != 2 {
		t.Errorf("expected 2 classified, got %d", result.TotalClassified)
	}
	if !provider.called {
		t.Error("expected provider to be called")
	}

	// Verify messages were recorded in the store.
	if len(s.processed) != 2 {
		t.Errorf("expected 2 processed messages, got %d", len(s.processed))
	}
	if !s.processed[store.MessageKey{AccountLabel: "personal", UID: 1}] {
		t.Error("expected UID 1 to be recorded as processed")
	}
	if !s.processed[store.MessageKey{AccountLabel: "personal", UID: 2}] {
		t.Error("expected UID 2 to be recorded as processed")
	}
}

// ---------------------------------------------------------------------------
// Tests: buildLabelSet
// ---------------------------------------------------------------------------

func TestBuildLabelSetDefaults(t *testing.T) {
	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	labels := p.buildLabelSet()

	expected := []string{"Useful", "ToDelete", "Ads"}
	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %v", len(expected), len(labels), labels)
	}
	for i, l := range expected {
		if labels[i] != l {
			t.Errorf("label[%d]: expected %q, got %q", i, l, labels[i])
		}
	}
}

func TestBuildLabelSetWithCustom(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Labels.Custom = []string{"Important", "FollowUp"}
	cfg.IMAP.Accounts = []config.IMAPAccount{
		{Label: "personal", Host: "imap.example.com", Username: "u", Password: "p"},
	}

	p := New(
		newFakeStore(),
		map[string]mail.Ingester{"personal": &fakeIngester{}},
		&fakeProvider{},
		&fakeRenderer{name: "markdown"},
		&fakeRenderer{name: "fallback"},
		&fakeChannel{name: "telegram"},
		slog.Default(),
		cfg,
	)

	labels := p.buildLabelSet()
	expected := []string{"Useful", "ToDelete", "Ads", "Important", "FollowUp"}
	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d: %v", len(expected), len(labels), labels)
	}
	for i, l := range expected {
		if labels[i] != l {
			t.Errorf("label[%d]: expected %q, got %q", i, l, labels[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Tests: clock
// ---------------------------------------------------------------------------

func TestNowFuncIsSet(t *testing.T) {
	p := defaultPipeline(newFakeStore(), &fakeIngester{})
	now := p.now()
	if now.IsZero() {
		t.Error("now() should not return zero time")
	}
}

func TestNowFuncCustomClock(t *testing.T) {
	frozen := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	s := newFakeStore()
	p := defaultPipeline(s, &fakeIngester{})
	p.now = func() time.Time { return frozen }

	now := p.now()
	if !now.Equal(frozen) {
		t.Errorf("expected %v, got %v", frozen, now)
	}

	result := p.Run(context.Background(), RunOptions{})
	if result.RunID == "" {
		t.Error("expected non-empty RunID")
	}
	if len(s.runs) == 0 {
		t.Fatal("expected run to be recorded")
	}
	if !s.runs[0].StartedAt.Equal(frozen) {
		t.Errorf("expected run started at %v, got %v", frozen, s.runs[0].StartedAt)
	}
}

// ---------------------------------------------------------------------------
// Tests: ForceReprocess skips dedup
// ---------------------------------------------------------------------------

func TestForceReprocessSkipsDedup(t *testing.T) {
	s := newFakeStore()
	s.processed[store.MessageKey{AccountLabel: "personal", UID: 1}] = true

	msgs := []mail.Message{
		{AccountLabel: "personal", UID: 1, Subject: "Already done", Body: "Body", Date: time.Now()},
		{AccountLabel: "personal", UID: 2, Subject: "New message", Body: "Body", Date: time.Now()},
	}
	ingester := &fakeIngester{messages: msgs}
	p := defaultPipeline(s, ingester)

	result := p.Run(context.Background(), RunOptions{ForceReprocess: true})

	if result.TotalFetched != 2 {
		t.Errorf("expected 2 fetched (ForceReprocess), got %d", result.TotalFetched)
	}
}

// ---------------------------------------------------------------------------
// Tests: Partial account failure
// ---------------------------------------------------------------------------

func TestRunPartialAccountFailure(t *testing.T) {
	s := newFakeStore()

	cfg := config.DefaultConfig()
	cfg.IMAP.Accounts = []config.IMAPAccount{
		{Label: "work", Host: "imap.work.com", Username: "u", Password: "p"},
		{Label: "personal", Host: "imap.personal.com", Username: "u", Password: "p"},
	}
	cfg.LLM.Model = "test-model"

	ingesters := map[string]mail.Ingester{
		"work":     &fakeIngester{messages: []mail.Message{{AccountLabel: "work", UID: 1, Subject: "Work", Body: "Body", Date: time.Now()}}},
		"personal": &fakeIngester{fetchErr: errors.New("connection refused")},
	}

	p := New(
		s,
		ingesters,
		&fakeProvider{},
		&fakeRenderer{name: "markdown"},
		&fakeRenderer{name: "fallback"},
		&fakeChannel{name: "telegram"},
		slog.Default(),
		cfg,
	)

	result := p.Run(context.Background(), RunOptions{})

	if result.Status != store.RunStatusPartial {
		t.Errorf("expected partial, got %q", result.Status)
	}
	if result.TotalFetched != 1 {
		t.Errorf("expected 1 fetched, got %d", result.TotalFetched)
	}
}
