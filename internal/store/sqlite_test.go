package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// NewSQLiteStore / migrations (existing tests preserved)
// ---------------------------------------------------------------------------

func TestNewSQLiteStore_InMemory(t *testing.T) {
	ctx := context.Background()

	s, err := NewSQLiteStore(ctx, ":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	defer s.Close()

	if s.db == nil {
		t.Fatal("expected non-nil db handle")
	}
}

func TestSQLiteStore_MigrationsCreateTables(t *testing.T) {
	s := newSQLiteStore(t)
	defer s.Close()

	expected := []string{"runs", "processed_messages", "flags_applied", "digests"}
	for _, table := range expected {
		if !tableExists(t, s, table) {
			t.Errorf("expected table %q to exist after migrations", table)
		}
	}
}

func TestSQLiteStore_MigrationsIdempotent(t *testing.T) {
	s1, err := NewSQLiteStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("first NewSQLiteStore failed: %v", err)
	}
	s1.Close()

	s2, err := NewSQLiteStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("second NewSQLiteStore failed: %v", err)
	}
	s2.Close()
}

func TestSQLiteStore_MigrationIndexes(t *testing.T) {
	s := newSQLiteStore(t)
	defer s.Close()

	if !indexExists(t, s, "sqlite_autoindex_processed_messages_1") {
		t.Error("expected composite primary key index on processed_messages")
	}
	if !indexExists(t, s, "idx_processed_messages_run_id") {
		t.Error("expected idx_processed_messages_run_id index")
	}
}

func TestSQLiteStore_ColumnIsRead(t *testing.T) {
	s := newSQLiteStore(t)
	defer s.Close()

	if !columnExists(t, s, "processed_messages", "is_read") {
		t.Error("expected is_read column in processed_messages table")
	}
}

// ---------------------------------------------------------------------------
// 4.9 + 4.10 — RecordRun & FinishRun
// ---------------------------------------------------------------------------

func TestRecordRun_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	now := time.Now().Truncate(time.Millisecond)
	r := Run{
		ID:        "test-run-1",
		StartedAt: now,
		Status:    RunStatusRunning,
	}

	got, err := s.RecordRun(ctx, r)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	if got.ID != "test-run-1" {
		t.Errorf("Run.ID = %q, want %q", got.ID, "test-run-1")
	}
	if got.Status != RunStatusRunning {
		t.Errorf("Run.Status = %q, want %q", got.Status, RunStatusRunning)
	}

	// Verify it was persisted.
	persisted, err := s.GetRun(ctx, "test-run-1")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}
	if persisted.ID != "test-run-1" {
		t.Errorf("persisted Run.ID = %q", persisted.ID)
	}
	if persisted.FinishedAt != nil {
		t.Error("expected FinishedAt to be nil for a running run")
	}
}

func TestRecordRun_EmptyID(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	r := Run{StartedAt: time.Now(), Status: RunStatusRunning}
	got, err := s.RecordRun(ctx, r)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	if got.ID == "" {
		t.Error("expected a generated ID, got empty")
	}

	// The generated ID should be retrievable.
	_, err = s.GetRun(ctx, got.ID)
	if err != nil {
		t.Errorf("GetRun(%q) failed: %v", got.ID, err)
	}
}

func TestRecordRun_DefaultStatus(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	r := Run{ID: "test-default-status"}
	got, err := s.RecordRun(ctx, r)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	if got.Status != RunStatusRunning {
		t.Errorf("expected default status %q, got %q", RunStatusRunning, got.Status)
	}
}

func TestRecordRun_DefaultStartedAt(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	r := Run{ID: "test-default-time"}
	got, err := s.RecordRun(ctx, r)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	if got.StartedAt.IsZero() {
		t.Error("expected non-zero StartedAt")
	}
}

func TestFinishRun_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	// Record a run first.
	r := Run{ID: "finish-test", StartedAt: time.Now(), Status: RunStatusRunning}
	_, err := s.RecordRun(ctx, r)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	// Finish it.
	err = s.FinishRun(ctx, "finish-test", RunStatusCompleted, 42, nil)
	if err != nil {
		t.Fatalf("FinishRun failed: %v", err)
	}

	// Verify.
	got, err := s.GetRun(ctx, "finish-test")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if got.Status != RunStatusCompleted {
		t.Errorf("status = %q, want %q", got.Status, RunStatusCompleted)
	}
	if got.FinishedAt == nil {
		t.Fatal("expected non-nil FinishedAt")
	}
	if !got.FinishedAt.After(got.StartedAt) {
		t.Error("expected FinishedAt after StartedAt")
	}
	if got.MessageCount != 42 {
		t.Errorf("message_count = %d, want 42", got.MessageCount)
	}
	if got.Error != "" {
		t.Errorf("error = %q, want empty", got.Error)
	}
}

func TestFinishRun_WithError(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	r := Run{ID: "finish-error", StartedAt: time.Now(), Status: RunStatusRunning}
	_, err := s.RecordRun(ctx, r)
	if err != nil {
		t.Fatalf("RecordRun failed: %v", err)
	}

	simulatedErr := fmt.Errorf("something went wrong: %w", context.Canceled)
	err = s.FinishRun(ctx, "finish-error", RunStatusDegraded, 5, simulatedErr)
	if err != nil {
		t.Fatalf("FinishRun failed: %v", err)
	}

	got, err := s.GetRun(ctx, "finish-error")
	if err != nil {
		t.Fatalf("GetRun failed: %v", err)
	}

	if got.Status != RunStatusDegraded {
		t.Errorf("status = %q, want %q", got.Status, RunStatusDegraded)
	}
	if got.Error != "something went wrong: context canceled" {
		t.Errorf("error = %q, want %q", got.Error, "something went wrong: context canceled")
	}
}

func TestFinishRun_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	err := s.FinishRun(ctx, "nonexistent", RunStatusCompleted, 0, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("expected ErrRunNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 4.11 + 4.12 — RecordMessage & AlreadyProcessed
// ---------------------------------------------------------------------------

func TestRecordMessage_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	setupRun(t, s, "run-msg-1")

	m := ProcessedMessage{
		RunID:          "run-msg-1",
		AccountLabel:   "personal",
		UID:            100,
		IsRead:         true,
		Classification: "Useful",
		DigestExcerpt:  "Meeting notes",
		ProcessedAt:    time.Now(),
	}

	err := s.RecordMessage(ctx, m)
	if err != nil {
		t.Fatalf("RecordMessage failed: %v", err)
	}
}

func TestRecordMessage_DefaultProcessedAt(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	setupRun(t, s, "run-msg-default")

	m := ProcessedMessage{
		RunID:          "run-msg-default",
		AccountLabel:   "personal",
		UID:            101,
		Classification: "ToDelete",
		DigestExcerpt:  "Spam",
	}

	err := s.RecordMessage(ctx, m)
	if err != nil {
		t.Fatalf("RecordMessage failed: %v", err)
	}
}

func TestRecordMessage_Dedup(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	setupRun(t, s, "run-dedup-1")

	m := ProcessedMessage{
		RunID:          "run-dedup-1",
		AccountLabel:   "personal",
		UID:            200,
		IsRead:         false,
		Classification: "Useful",
		DigestExcerpt:  "Original",
		ProcessedAt:    time.Now(),
	}

	// First insert should succeed.
	if err := s.RecordMessage(ctx, m); err != nil {
		t.Fatalf("first RecordMessage failed: %v", err)
	}

	// Second insert with same key should be silently ignored.
	m.RunID = "run-dedup-2" // different run
	m.DigestExcerpt = "Updated"
	m.IsRead = true
	if err := s.RecordMessage(ctx, m); err != nil {
		t.Fatalf("second RecordMessage failed: %v", err)
	}

	// Verify only the first record is persisted (INSERT OR IGNORE).
	processed, err := s.AlreadyProcessed(ctx, []MessageKey{{AccountLabel: "personal", UID: 200}})
	if err != nil {
		t.Fatalf("AlreadyProcessed failed: %v", err)
	}
	if !processed[MessageKey{AccountLabel: "personal", UID: 200}] {
		t.Error("expected key to be marked as processed")
	}
}

func TestAlreadyProcessed_EmptyBatch(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	result, err := s.AlreadyProcessed(ctx, nil)
	if err != nil {
		t.Fatalf("AlreadyProcessed with nil keys failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}

	result, err = s.AlreadyProcessed(ctx, []MessageKey{})
	if err != nil {
		t.Fatalf("AlreadyProcessed with empty keys failed: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d entries", len(result))
	}
}

func TestAlreadyProcessed_MultipleKeys(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	setupRun(t, s, "run-ap-multi")

	// Record messages.
	expected := []ProcessedMessage{
		{RunID: "run-ap-multi", AccountLabel: "a", UID: 1, Classification: "Useful", DigestExcerpt: "x", ProcessedAt: time.Now()},
		{RunID: "run-ap-multi", AccountLabel: "a", UID: 2, Classification: "Ads", DigestExcerpt: "y", ProcessedAt: time.Now()},
		{RunID: "run-ap-multi", AccountLabel: "b", UID: 1, Classification: "Useful", DigestExcerpt: "z", ProcessedAt: time.Now()},
	}
	for _, m := range expected {
		if err := s.RecordMessage(ctx, m); err != nil {
			t.Fatalf("RecordMessage failed: %v", err)
		}
	}

	// Check a mix of existing and non-existing keys.
	keys := []MessageKey{
		{AccountLabel: "a", UID: 1}, // exists
		{AccountLabel: "a", UID: 3}, // does not exist
		{AccountLabel: "b", UID: 1}, // exists
		{AccountLabel: "c", UID: 1}, // does not exist
	}

	result, err := s.AlreadyProcessed(ctx, keys)
	if err != nil {
		t.Fatalf("AlreadyProcessed failed: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("expected 2 existing keys, got %d", len(result))
	}
	if !result[MessageKey{AccountLabel: "a", UID: 1}] {
		t.Error("expected a/1 to be processed")
	}
	if !result[MessageKey{AccountLabel: "b", UID: 1}] {
		t.Error("expected b/1 to be processed")
	}
	if result[MessageKey{AccountLabel: "a", UID: 3}] {
		t.Error("expected a/3 to NOT be processed")
	}
	if result[MessageKey{AccountLabel: "c", UID: 1}] {
		t.Error("expected c/1 to NOT be processed")
	}
}

// ---------------------------------------------------------------------------
// 4.13 + 4.14 — RecordFlag & RecordDigest
// ---------------------------------------------------------------------------

func TestRecordFlag_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	flag := FlagRecord{
		AccountLabel: "personal",
		UID:          42,
		Flag:         "Useful",
		AppliedAt:    time.Now(),
	}

	if err := s.RecordFlag(ctx, flag); err != nil {
		t.Fatalf("RecordFlag failed: %v", err)
	}
}

func TestRecordFlag_DefaultAppliedAt(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	flag := FlagRecord{
		AccountLabel: "personal",
		UID:          99,
		Flag:         "ToDelete",
	}

	if err := s.RecordFlag(ctx, flag); err != nil {
		t.Fatalf("RecordFlag failed: %v", err)
	}
}

func TestRecordFlag_Dedup(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	flag := FlagRecord{
		AccountLabel: "personal",
		UID:          7,
		Flag:         "Ads",
		AppliedAt:    time.Now(),
	}

	// First insert should succeed.
	if err := s.RecordFlag(ctx, flag); err != nil {
		t.Fatalf("first RecordFlag failed: %v", err)
	}

	// Second insert with same composite key should be silently ignored.
	if err := s.RecordFlag(ctx, flag); err != nil {
		t.Fatalf("second RecordFlag failed: %v", err)
	}
}

func TestRecordDigest_HappyPath(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	setupRun(t, s, "run-digest-1")

	digest := DigestRecord{
		RunID:       "run-digest-1",
		Channel:     "telegram",
		Status:      DigestStatusSent,
		PayloadHash: "abc123def456",
	}

	if err := s.RecordDigest(ctx, digest); err != nil {
		t.Fatalf("RecordDigest failed: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 4.15 + 4.16 — GetRun, ListRuns, GetLastSuccessfulRunTime
// ---------------------------------------------------------------------------

func TestGetRun_NotFound(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	_, err := s.GetRun(ctx, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for nonexistent run")
	}
	if !errors.Is(err, ErrRunNotFound) {
		t.Errorf("expected ErrRunNotFound, got %v", err)
	}
}

func TestListRuns_Empty(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	runs, err := s.ListRuns(ctx, 10)
	if err != nil {
		t.Fatalf("ListRuns on empty store failed: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

func TestListRuns_Ordering(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	// Insert runs with staggered start times.
	for i := 0; i < 5; i++ {
		r := Run{
			ID:        fmt.Sprintf("run-%d", i),
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    RunStatusCompleted,
		}
		if _, err := s.RecordRun(ctx, r); err != nil {
			t.Fatalf("RecordRun failed: %v", err)
		}
	}

	runs, err := s.ListRuns(ctx, 10)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}

	if len(runs) != 5 {
		t.Fatalf("expected 5 runs, got %d", len(runs))
	}

	// Verify descending order (most recent first).
	for i := 1; i < len(runs); i++ {
		if runs[i-1].StartedAt.Before(runs[i].StartedAt) {
			t.Errorf("run[%d].StartedAt %v is before run[%d].StartedAt %v (expected DESC)",
				i-1, runs[i-1].StartedAt, i, runs[i].StartedAt)
		}
	}
}

func TestListRuns_Limit(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	for i := 0; i < 20; i++ {
		r := Run{
			ID:        fmt.Sprintf("run-%d", i),
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    RunStatusCompleted,
		}
		if _, err := s.RecordRun(ctx, r); err != nil {
			t.Fatalf("RecordRun failed: %v", err)
		}
	}

	runs, err := s.ListRuns(ctx, 5)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 5 {
		t.Errorf("expected 5 runs, got %d", len(runs))
	}
}

func TestListRuns_DefaultLimit(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	for i := 0; i < 15; i++ {
		r := Run{
			ID:        fmt.Sprintf("run-%d", i),
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    RunStatusCompleted,
		}
		if _, err := s.RecordRun(ctx, r); err != nil {
			t.Fatalf("RecordRun failed: %v", err)
		}
	}

	// Default limit is 10.
	runs, err := s.ListRuns(ctx, 0)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 10 {
		t.Errorf("expected 10 runs (default limit), got %d", len(runs))
	}
}

func TestListRuns_NegativeLimit(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	for i := 0; i < 5; i++ {
		r := Run{
			ID:        fmt.Sprintf("run-%d", i),
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    RunStatusCompleted,
		}
		if _, err := s.RecordRun(ctx, r); err != nil {
			t.Fatalf("RecordRun failed: %v", err)
		}
	}

	// Negative limit should default to 10.
	runs, err := s.ListRuns(ctx, -1)
	if err != nil {
		t.Fatalf("ListRuns failed: %v", err)
	}
	if len(runs) != 5 {
		t.Errorf("expected 5 runs (only 5 exist), got %d", len(runs))
	}
}

func TestGetLastSuccessfulRunTime_NoRuns(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	got, err := s.GetLastSuccessfulRunTime(ctx)
	if err != nil {
		t.Fatalf("GetLastSuccessfulRunTime failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetLastSuccessfulRunTime_OnlyFailedRuns(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	// Insert only failed runs.
	failedStatuses := []RunStatus{RunStatusIngestFailed, RunStatusDegraded, RunStatusCancelled}
	for i, status := range failedStatuses {
		r := Run{
			ID:        fmt.Sprintf("run-%d", i),
			StartedAt: time.Now().Add(time.Duration(i) * time.Second),
			Status:    status,
		}
		rec, err := s.RecordRun(ctx, r)
		if err != nil {
			t.Fatalf("RecordRun failed: %v", err)
		}
		// Also finish runs so they have a finished_at.
		_ = s.FinishRun(ctx, rec.ID, status, 0, nil)
	}

	got, err := s.GetLastSuccessfulRunTime(ctx)
	if err != nil {
		t.Fatalf("GetLastSuccessfulRunTime failed: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil (no completed runs), got %v", got)
	}
}

func TestGetLastSuccessfulRunTime_ReturnsLatest(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	// Create a few runs with staggered times — only one is "completed".
	t1 := setupAndFinishRun(t, s, "run-early", RunStatusDegraded, time.Now())
	t2 := setupAndFinishRun(t, s, "run-mid", RunStatusCompleted, time.Now().Add(1*time.Second))
	t3 := setupAndFinishRun(t, s, "run-late", RunStatusPartial, time.Now().Add(2*time.Second))

	_ = t1
	_ = t3

	got, err := s.GetLastSuccessfulRunTime(ctx)
	if err != nil {
		t.Fatalf("GetLastSuccessfulRunTime failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}

	// The latest completed run is "run-mid" with the time we passed.
	if !got.Equal(t2) {
		t.Errorf("expected time %v, got %v", t2, *got)
	}
}

func TestGetLastSuccessfulRunTime_MultipleCompleted(t *testing.T) {
	ctx := context.Background()
	s := newSQLiteStore(t)
	defer s.Close()

	t1 := setupAndFinishRun(t, s, "run-1", RunStatusCompleted, time.Now())
	t2 := setupAndFinishRun(t, s, "run-2", RunStatusCompleted, time.Now().Add(5*time.Second))

	got, err := s.GetLastSuccessfulRunTime(ctx)
	if err != nil {
		t.Fatalf("GetLastSuccessfulRunTime failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil time")
	}

	// Should return the latest (run-2).
	if !got.Equal(t2) {
		t.Errorf("expected latest time %v, got %v", t2, *got)
	}
	_ = t1
}

// ---------------------------------------------------------------------------
// Context cancellation
// ---------------------------------------------------------------------------

func TestSQLiteStore_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled

	s := newSQLiteStore(t)
	defer s.Close()

	_, err := s.RecordRun(ctx, Run{ID: "cancelled"})
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// newSQLiteStore opens a fresh in-memory store for testing.
func newSQLiteStore(t testing.TB) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore(:memory:) failed: %v", err)
	}
	return s
}

// setupRun inserts a run record for tests that depend on a run existing via
// foreign key (digests table).
func setupRun(t testing.TB, s *SQLiteStore, runID string) {
	t.Helper()
	_, err := s.RecordRun(context.Background(), Run{
		ID:        runID,
		StartedAt: time.Now(),
		Status:    RunStatusRunning,
	})
	if err != nil {
		t.Fatalf("setupRun: RecordRun(%q) failed: %v", runID, err)
	}
}

// setupAndFinishRun creates and finishes a run, returning the finished_at time
// that was set (approximately).
func setupAndFinishRun(t testing.TB, s *SQLiteStore, runID string, status RunStatus, finishedAt time.Time) time.Time {
	t.Helper()
	ctx := context.Background()
	_, err := s.RecordRun(ctx, Run{
		ID:        runID,
		StartedAt: finishedAt.Add(-1 * time.Minute), // started before finished
		Status:    RunStatusRunning,
	})
	if err != nil {
		t.Fatalf("setupAndFinishRun: RecordRun(%q) failed: %v", runID, err)
	}

	// Override the finished_at by manually updating.
	const updateQuery = "UPDATE runs SET finished_at = ?, status = ? WHERE id = ?"
	_, err = s.db.ExecContext(ctx, updateQuery, finishedAt, status, runID)
	if err != nil {
		t.Fatalf("setupAndFinishRun: update(%q) failed: %v", runID, err)
	}

	return finishedAt
}

// tableExists checks if a table exists in the database.
func tableExists(t *testing.T, s *SQLiteStore, name string) bool {
	t.Helper()
	row := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", name,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query table %q: %v", name, err)
	}
	return count > 0
}

// indexExists checks if an index exists in the database.
func indexExists(t *testing.T, s *SQLiteStore, name string) bool {
	t.Helper()
	row := s.db.QueryRow(
		"SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?", name,
	)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("query index %q: %v", name, err)
	}
	return count > 0
}

// columnExists checks if a column exists in a table.
func columnExists(t *testing.T, s *SQLiteStore, table, column string) bool {
	t.Helper()
	rows, err := s.db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		t.Fatalf("pragma table_info(%q): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan column: %v", err)
		}
		if name == column {
			return true
		}
	}
	return false
}