package store

import (
	"context"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// MessageKey
// ---------------------------------------------------------------------------

func TestMessageKey_Key(t *testing.T) {
	tests := []struct {
		name     string
		key      MessageKey
		expected string
	}{
		{name: "personal account", key: MessageKey{AccountLabel: "personal", UID: 1}, expected: "personal/1"},
		{name: "work account", key: MessageKey{AccountLabel: "work", UID: 42}, expected: "work/42"},
		{name: "zero uid", key: MessageKey{AccountLabel: "test", UID: 0}, expected: "test/0"},
		{name: "max uint32", key: MessageKey{AccountLabel: "a", UID: 4294967295}, expected: "a/4294967295"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.key.Key(); got != tt.expected {
				t.Errorf("MessageKey.Key() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestMessageKey_Equality(t *testing.T) {
	a := MessageKey{AccountLabel: "personal", UID: 1}
	b := MessageKey{AccountLabel: "personal", UID: 1}
	c := MessageKey{AccountLabel: "work", UID: 1}
	d := MessageKey{AccountLabel: "personal", UID: 2}

	if a != b {
		t.Error("identical MessageKeys should be equal")
	}
	if a == c {
		t.Error("different account labels should not be equal")
	}
	if a == d {
		t.Error("different UIDs should not be equal")
	}
}

func TestMessageKey_MapKey(t *testing.T) {
	// Verify that MessageKey works as a map key (struct value equality).
	m := make(map[MessageKey]bool)
	m[MessageKey{AccountLabel: "personal", UID: 1}] = true
	m[MessageKey{AccountLabel: "personal", UID: 2}] = true

	if !m[MessageKey{AccountLabel: "personal", UID: 1}] {
		t.Error("expected map to contain key personal/1")
	}
	if m[MessageKey{AccountLabel: "work", UID: 1}] {
		t.Error("expected map not to contain key work/1")
	}
}

// ---------------------------------------------------------------------------
// ProcessedMessage
// ---------------------------------------------------------------------------

func TestProcessedMessage_Key(t *testing.T) {
	msg := ProcessedMessage{
		AccountLabel: "personal",
		UID:          42,
	}

	expected := MessageKey{AccountLabel: "personal", UID: 42}
	if got := msg.Key(); got != expected {
		t.Errorf("ProcessedMessage.Key() = %v, want %v", got, expected)
	}
}

func TestProcessedMessage_IsRead(t *testing.T) {
	read := ProcessedMessage{IsRead: true}
	unread := ProcessedMessage{IsRead: false}

	if !read.IsRead {
		t.Error("expected IsRead to be true")
	}
	if unread.IsRead {
		t.Error("expected IsRead to be false")
	}
}

// ---------------------------------------------------------------------------
// FlagRecord
// ---------------------------------------------------------------------------

func TestFlagRecord_Key(t *testing.T) {
	r := FlagRecord{
		AccountLabel: "personal",
		UID:          7,
		Flag:         "Useful",
	}

	expected := MessageKey{AccountLabel: "personal", UID: 7}
	if got := r.Key(); got != expected {
		t.Errorf("FlagRecord.Key() = %v, want %v", got, expected)
	}
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

func TestRun_Defaults(t *testing.T) {
	now := time.Now()
	r := Run{
		ID:        "run-1",
		StartedAt: now,
		Status:    RunStatusRunning,
	}

	if r.ID != "run-1" {
		t.Errorf("Run.ID = %q, want %q", r.ID, "run-1")
	}
	if r.Status != RunStatusRunning {
		t.Errorf("Run.Status = %q, want %q", r.Status, RunStatusRunning)
	}
	if r.FinishedAt != nil {
		t.Error("expected FinishedAt to be nil for a running run")
	}
}

func TestRun_Finished(t *testing.T) {
	now := time.Now()
	finish := now.Add(5 * time.Second)
	r := Run{
		ID:         "run-2",
		StartedAt:  now,
		FinishedAt: &finish,
		Status:     RunStatusCompleted,
	}

	if r.Status != RunStatusCompleted {
		t.Errorf("Run.Status = %q, want %q", r.Status, RunStatusCompleted)
	}
	if r.FinishedAt == nil {
		t.Fatal("expected FinishedAt to be non-nil")
	}
	if !r.FinishedAt.After(r.StartedAt) {
		t.Error("expected FinishedAt to be after StartedAt")
	}
}

// ---------------------------------------------------------------------------
// DigestRecord
// ---------------------------------------------------------------------------

func TestDigestRecord_Defaults(t *testing.T) {
	d := DigestRecord{
		RunID:       "run-1",
		Channel:     "telegram",
		Status:      DigestStatusSent,
		PayloadHash: "abc123",
	}

	if d.Status != DigestStatusSent {
		t.Errorf("DigestRecord.Status = %q, want %q", d.Status, DigestStatusSent)
	}
}

// ---------------------------------------------------------------------------
// RunStatus constants
// ---------------------------------------------------------------------------

func TestRunStatus_Values(t *testing.T) {
	statuses := []RunStatus{
		RunStatusRunning,
		RunStatusCompleted,
		RunStatusDegraded,
		RunStatusIngestFailed,
		RunStatusPartial,
		RunStatusCancelled,
	}
	expected := []string{
		"running", "completed", "degraded",
		"ingest_failed", "partial", "cancelled",
	}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("RunStatus = %q, want %q", s, expected[i])
		}
	}
}

func TestDigestStatus_Values(t *testing.T) {
	statuses := []DigestStatus{
		DigestStatusSent,
		DigestStatusFailed,
		DigestStatusSkipped,
	}
	expected := []string{"sent", "failed", "skipped"}

	for i, s := range statuses {
		if string(s) != expected[i] {
			t.Errorf("DigestStatus = %q, want %q", s, expected[i])
		}
	}
}

// ---------------------------------------------------------------------------
// Store interface compilation check
// ---------------------------------------------------------------------------

// compileCheck verifies that a concrete type can implement Store.
// This is a compile-time check, not a runtime test.
func TestStoreInterface_compiles(t *testing.T) {
	// A minimal fake that implements Store, proving the interface is sound.
	var _ Store = (*fakeStore)(nil)
	_ = t // silence unused-import warning
}

// fakeStore is a minimal in-memory implementation used only to verify the
// Store interface compiles. Full fakes live in internal/testutil.
type fakeStore struct{}

func (f *fakeStore) Close() error                                 { return nil }
func (f *fakeStore) RecordRun(_ context.Context, _ Run) (Run, error) { return Run{}, nil }
func (f *fakeStore) FinishRun(_ context.Context, _ string, _ RunStatus, _ int, _ error) error {
	return nil
}
func (f *fakeStore) GetRun(_ context.Context, _ string) (Run, error) { return Run{}, nil }
func (f *fakeStore) ListRuns(_ context.Context, _ int) ([]Run, error) { return nil, nil }
func (f *fakeStore) GetLastSuccessfulRunTime(_ context.Context) (*time.Time, error) { return nil, nil }
func (f *fakeStore) RecordMessage(_ context.Context, _ ProcessedMessage) error { return nil }
func (f *fakeStore) AlreadyProcessed(_ context.Context, _ []MessageKey) (map[MessageKey]bool, error) {
	return nil, nil
}
func (f *fakeStore) RecordFlag(_ context.Context, _ FlagRecord) error { return nil }
func (f *fakeStore) RecordDigest(_ context.Context, _ DigestRecord) error { return nil }