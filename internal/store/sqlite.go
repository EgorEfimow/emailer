package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite" // uses database/sql with any sqlite driver
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "modernc.org/sqlite" // registers the "sqlite" database/sql driver
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// SQLiteStore implements the Store interface backed by a SQLite database.
// It is safe for concurrent use.
type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteStore opens or creates a SQLite database at the given path and
// runs all pending migrations. The path can be ":memory:" for testing.
func NewSQLiteStore(ctx context.Context, path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("store.NewSQLiteStore: open db: %w", err)
	}

	// Configure modernc.org/sqlite for our workload.
	db.SetMaxOpenConns(1) // SQLite is single-writer; serialise access.
	db.SetMaxIdleConns(1)

	// Verify the connection is alive.
	if err := db.PingContext(ctx); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("store.NewSQLiteStore: ping db: %w", err)
	}

	// Run migrations.
	if err := runMigrations(db); err != nil {
		db.Close() //nolint:errcheck
		return nil, fmt.Errorf("store.NewSQLiteStore: migrations: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close releases the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// runMigrations applies all pending SQL migration files to the database.
func runMigrations(db *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("migration source: %w", err)
	}
	defer src.Close() //nolint:errcheck

	inst, err := sqlite.WithInstance(db, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("migration instance: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "sqlite", inst)
	if err != nil {
		return fmt.Errorf("migrate init: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrate up: %w", err)
	}

	// NOTE: we do NOT call m.Close() here because it would close the
	// *sql.DB that the caller needs to keep using. The migrate instance
	// will be garbage-collected.
	return nil
}

// ---------------------------------------------------------------------------
// Run lifecycle
// ---------------------------------------------------------------------------

// RecordRun persists a new run record. If the run ID is empty, a unique ID is
// generated. Returns the run with its assigned ID.
func (s *SQLiteStore) RecordRun(ctx context.Context, r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.ID == "" {
		id, err := generateID()
		if err != nil {
			return Run{}, fmt.Errorf("store.RecordRun: generate id: %w", err)
		}
		r.ID = id
	}

	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	if r.Status == "" {
		r.Status = RunStatusRunning
	}

	const query = "INSERT INTO runs (id, started_at, finished_at, status, message_count, error) VALUES (?, ?, ?, ?, ?, ?)"
	_, err := s.db.ExecContext(ctx, query,
		r.ID,
		formatTime(r.StartedAt),
		nil, // finished_at is NULL when starting
		r.Status,
		r.MessageCount,
		r.Error,
	)
	if err != nil {
		return Run{}, fmt.Errorf("store.RecordRun: exec: %w", err)
	}

	return r, nil
}

// FinishRun updates a run record with completion status, finished_at, and
// error details.
func (s *SQLiteStore) FinishRun(ctx context.Context, runID string, status RunStatus, messageCount int, runErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	errStr := ""
	if runErr != nil {
		errStr = runErr.Error()
	}

	const query = "UPDATE runs SET finished_at = ?, status = ?, message_count = ?, error = ? WHERE id = ?"
	result, err := s.db.ExecContext(ctx, query,
		formatTime(time.Now()),
		status,
		messageCount,
		errStr,
		runID,
	)
	if err != nil {
		return fmt.Errorf("store.FinishRun: exec: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("store.FinishRun: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("store.FinishRun: %w", ErrRunNotFound)
	}

	return nil
}

// GetRun retrieves a single run by ID.
func (s *SQLiteStore) GetRun(ctx context.Context, runID string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const query = "SELECT id, started_at, finished_at, status, message_count, error FROM runs WHERE id = ?"
	row := s.db.QueryRowContext(ctx, query, runID)

	var (
		id            string
		startedAtStr  string
		finishedAtStr sql.NullString
		status        string
		messageCount  int
		errorStr      string
	)
	if err := row.Scan(&id, &startedAtStr, &finishedAtStr, &status, &messageCount, &errorStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Run{}, fmt.Errorf("store.GetRun: %w", ErrRunNotFound)
		}
		return Run{}, fmt.Errorf("store.GetRun: scan: %w", err)
	}

	startedAt, err := parseTime(startedAtStr)
	if err != nil {
		return Run{}, fmt.Errorf("store.GetRun: parse started_at: %w", err)
	}

	run := Run{
		ID:           id,
		StartedAt:    startedAt,
		Status:       RunStatus(status),
		MessageCount: messageCount,
		Error:        errorStr,
	}
	if finishedAtStr.Valid {
		t, err := parseTime(finishedAtStr.String)
		if err != nil {
			return Run{}, fmt.Errorf("store.GetRun: parse finished_at: %w", err)
		}
		run.FinishedAt = &t
	}

	return run, nil
}

// ListRuns returns the most recent runs, ordered by started_at descending.
// Defaults to a limit of 10 if limit is 0 or negative.
func (s *SQLiteStore) ListRuns(ctx context.Context, limit int) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	const query = "SELECT id, started_at, finished_at, status, message_count, error FROM runs ORDER BY started_at DESC LIMIT ?"
	rows, err := s.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("store.ListRuns: query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	var runs []Run
	for rows.Next() {
		var (
			id            string
			startedAtStr  string
			finishedAtStr sql.NullString
			status        string
			messageCount  int
			errorStr      string
		)
		if err := rows.Scan(&id, &startedAtStr, &finishedAtStr, &status, &messageCount, &errorStr); err != nil {
			return nil, fmt.Errorf("store.ListRuns: scan: %w", err)
		}

		startedAt, err := parseTime(startedAtStr)
		if err != nil {
			return nil, fmt.Errorf("store.ListRuns: parse started_at: %w", err)
		}

		run := Run{
			ID:           id,
			StartedAt:    startedAt,
			Status:       RunStatus(status),
			MessageCount: messageCount,
			Error:        errorStr,
		}
		if finishedAtStr.Valid {
			t, err := parseTime(finishedAtStr.String)
			if err != nil {
				return nil, fmt.Errorf("store.ListRuns: parse finished_at: %w", err)
			}
			run.FinishedAt = &t
		}

		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.ListRuns: rows: %w", err)
	}

	// Return empty slice instead of nil for JSON marshalling.
	if runs == nil {
		runs = []Run{}
	}

	return runs, nil
}

// GetLastSuccessfulRunTime returns the finished_at timestamp of the most
// recent run with status "completed". Returns nil if no successful run exists.
func (s *SQLiteStore) GetLastSuccessfulRunTime(ctx context.Context) (*time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	const query = "SELECT finished_at FROM runs WHERE status = 'completed' AND finished_at IS NOT NULL ORDER BY finished_at DESC LIMIT 1"
	row := s.db.QueryRowContext(ctx, query)

	var finishedAtStr sql.NullString
	if err := row.Scan(&finishedAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store.GetLastSuccessfulRunTime: scan: %w", err)
	}

	if !finishedAtStr.Valid {
		return nil, nil
	}

	t, err := parseTime(finishedAtStr.String)
	if err != nil {
		return nil, fmt.Errorf("store.GetLastSuccessfulRunTime: parse: %w", err)
	}

	return &t, nil
}

// ---------------------------------------------------------------------------
// Processed messages
// ---------------------------------------------------------------------------

// RecordMessage persists a processed message record. If the message was
// already processed (same account_label + uid), the insert is silently
// ignored, preserving idempotency.
func (s *SQLiteStore) RecordMessage(ctx context.Context, m ProcessedMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if m.ProcessedAt.IsZero() {
		m.ProcessedAt = time.Now()
	}

	const query = `INSERT OR IGNORE INTO processed_messages
		(run_id, account_label, uid, is_read, classification, digest_excerpt, processed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`

	isRead := 0
	if m.IsRead {
		isRead = 1
	}

	_, err := s.db.ExecContext(ctx, query,
		m.RunID,
		m.AccountLabel,
		m.UID,
		isRead,
		m.Classification,
		m.DigestExcerpt,
		formatTime(m.ProcessedAt),
	)
	if err != nil {
		return fmt.Errorf("store.RecordMessage: exec: %w", err)
	}

	return nil
}

// AlreadyProcessed checks which of the given message keys have already been
// processed in a previous run. Returns the set of keys that exist in the
// processed_messages table.
func (s *SQLiteStore) AlreadyProcessed(ctx context.Context, keys []MessageKey) (map[MessageKey]bool, error) {
	if len(keys) == 0 {
		return map[MessageKey]bool{}, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build a parameterised query: SELECT account_label, uid FROM
	// processed_messages WHERE (account_label = ? AND uid = ?) OR ...
	var builder strings.Builder
	builder.WriteString("SELECT account_label, uid FROM processed_messages WHERE ")
	args := make([]any, 0, len(keys)*2)
	for i, k := range keys {
		if i > 0 {
			builder.WriteString(" OR ")
		}
		builder.WriteString("(account_label = ? AND uid = ?)")
		args = append(args, k.AccountLabel, k.UID)
	}

	rows, err := s.db.QueryContext(ctx, builder.String(), args...)
	if err != nil {
		return nil, fmt.Errorf("store.AlreadyProcessed: query: %w", err)
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[MessageKey]bool, len(keys))
	for rows.Next() {
		var label string
		var uid uint32
		if err := rows.Scan(&label, &uid); err != nil {
			return nil, fmt.Errorf("store.AlreadyProcessed: scan: %w", err)
		}
		result[MessageKey{AccountLabel: label, UID: uid}] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("store.AlreadyProcessed: rows: %w", err)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Flag records
// ---------------------------------------------------------------------------

// RecordFlag persists a flag application record. Duplicate flag entries for
// the same message are silently ignored.
func (s *SQLiteStore) RecordFlag(ctx context.Context, r FlagRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.AppliedAt.IsZero() {
		r.AppliedAt = time.Now()
	}

	const query = "INSERT OR IGNORE INTO flags_applied (account_label, uid, flag, applied_at) VALUES (?, ?, ?, ?)"
	_, err := s.db.ExecContext(ctx, query,
		r.AccountLabel,
		r.UID,
		r.Flag,
		formatTime(r.AppliedAt),
	)
	if err != nil {
		return fmt.Errorf("store.RecordFlag: exec: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Digest records
// ---------------------------------------------------------------------------

// RecordDigest persists a digest delivery record.
func (s *SQLiteStore) RecordDigest(ctx context.Context, d DigestRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	const query = "INSERT INTO digests (run_id, channel, status, payload_hash) VALUES (?, ?, ?, ?)"
	_, err := s.db.ExecContext(ctx, query,
		d.RunID,
		d.Channel,
		d.Status,
		d.PayloadHash,
	)
	if err != nil {
		return fmt.Errorf("store.RecordDigest: exec: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Run digest summary
// ---------------------------------------------------------------------------

// SaveRunDigestSummary persists (or replaces) the digest snapshot for a run
// after it has been rendered. It is idempotent for a given RunID.
func (s *SQLiteStore) SaveRunDigestSummary(ctx context.Context, summary RunDigestSummary) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("store.SaveRunDigestSummary: begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if err := s.upsertDigestSummaryTx(ctx, tx, summary); err != nil {
		return err
	}
	if err := s.replaceCountTableTx(ctx, tx, summary.RunID, summary.CountsByLabel,
		"DELETE FROM run_digest_label_counts WHERE run_id = ?",
		"INSERT INTO run_digest_label_counts (run_id, label, count) VALUES (?, ?, ?)",
		"labels"); err != nil {
		return err
	}
	if err := s.replaceCountTableTx(ctx, tx, summary.RunID, summary.SenderCounts,
		"DELETE FROM run_digest_sender_counts WHERE run_id = ?",
		"INSERT INTO run_digest_sender_counts (run_id, sender, count) VALUES (?, ?, ?)",
		"senders"); err != nil {
		return err
	}
	if err := s.replaceCountTableTx(ctx, tx, summary.RunID, summary.DomainCounts,
		"DELETE FROM run_digest_domain_counts WHERE run_id = ?",
		"INSERT INTO run_digest_domain_counts (run_id, domain, count) VALUES (?, ?, ?)",
		"domains"); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("store.SaveRunDigestSummary: commit: %w", err)
	}
	return nil
}

// upsertDigestSummaryTx inserts or updates the header row for a run snapshot
// inside a transaction. It does not commit.
func (s *SQLiteStore) upsertDigestSummaryTx(ctx context.Context, tx *sql.Tx, summary RunDigestSummary) error {
	const upsertSummary = `INSERT INTO run_digest_summaries
		(run_id, finished_at, accounts_failed, high_priority_count, payload_json)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(run_id) DO UPDATE SET
			finished_at = excluded.finished_at,
			accounts_failed = excluded.accounts_failed,
			high_priority_count = excluded.high_priority_count,
			payload_json = excluded.payload_json`
	_, err := tx.ExecContext(ctx, upsertSummary,
		summary.RunID,
		formatTime(summary.FinishedAt),
		summary.AccountsFailed,
		summary.HighPriorityCount,
		"{}",
	)
	if err != nil {
		return fmt.Errorf("store.SaveRunDigestSummary: upsert summary: %w", err)
	}
	return nil
}

// replaceCountTableTx deletes all rows for runID and re-inserts positive counts
// from the map. delQuery and insQuery must use ? placeholders with runID as first arg.
func (s *SQLiteStore) replaceCountTableTx(ctx context.Context, tx *sql.Tx, runID string, counts map[string]int, delQuery, insQuery, tableName string) error {
	if _, err := tx.ExecContext(ctx, delQuery, runID); err != nil {
		return fmt.Errorf("store.SaveRunDigestSummary: del %s: %w", tableName, err)
	}
	for k, count := range counts {
		if count <= 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx, insQuery, runID, k, count); err != nil {
			return fmt.Errorf("store.SaveRunDigestSummary: ins %s %q: %w", tableName, k, err)
		}
	}
	return nil
}

// loadCountTable loads key/count rows for a run from a child table.
func (s *SQLiteStore) loadCountTable(ctx context.Context, runID, query string) (map[string]int, error) {
	rows, err := s.db.QueryContext(ctx, query, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close() //nolint:errcheck

	result := make(map[string]int)
	for rows.Next() {
		var key string
		var count int
		if err := rows.Scan(&key, &count); err != nil {
			return nil, err
		}
		result[key] = count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetPreviousRunDigestSummary returns the most recent digest snapshot
// attached to a *completed* run whose finished_at is strictly before the
// given run's finished_at. Pass the current run's ID so we can fetch its
// finished_at and use that as the cutoff. Returns (nil, nil) if no prior
// snapshot exists.
func (s *SQLiteStore) GetPreviousRunDigestSummary(ctx context.Context, beforeRunID string) (*RunDigestSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// 1) Get the finished_at of the beforeRunID (current run in progress or just finished).
	var currentFinishedAtStr sql.NullString
	const getCurrent = "SELECT finished_at FROM runs WHERE id = ?"
	row := s.db.QueryRowContext(ctx, getCurrent, beforeRunID)
	if err := row.Scan(&currentFinishedAtStr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: get current run: %w", err)
	}

	var currentFinishedAt time.Time
	if currentFinishedAtStr.Valid {
		t, err := parseTime(currentFinishedAtStr.String)
		if err != nil {
			return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: parse current finished_at: %w", err)
		}
		currentFinishedAt = t
	} else {
		currentFinishedAt = time.Now()
	}

	// 2) Find the most recent completed run with finished_at < currentFinishedAt.
	const query = `
		SELECT s.run_id, s.finished_at, s.accounts_failed, s.high_priority_count
		FROM run_digest_summaries s
		JOIN runs r ON r.id = s.run_id
		WHERE r.status = 'completed' AND r.finished_at IS NOT NULL AND r.finished_at < ?
		ORDER BY r.finished_at DESC
		LIMIT 1
	`
	row = s.db.QueryRowContext(ctx, query, formatTime(currentFinishedAt))

	var (
		summaryRunID       string
		summaryFinishedAtStr string
		accountsFailed     int
		highPriorityCount  int
	)
	if err := row.Scan(&summaryRunID, &summaryFinishedAtStr, &accountsFailed, &highPriorityCount); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: scan summary: %w", err)
	}

	finishedAt, err := parseTime(summaryFinishedAtStr)
	if err != nil {
		return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: parse finished_at: %w", err)
	}

	summary := &RunDigestSummary{
		RunID:             summaryRunID,
		FinishedAt:        finishedAt,
		AccountsFailed:    accountsFailed,
		HighPriorityCount: highPriorityCount,
		CountsByLabel:     make(map[string]int),
		SenderCounts:      make(map[string]int),
		DomainCounts:      make(map[string]int),
	}

	// 3) Load label counts
	labels, err := s.loadCountTable(ctx, summaryRunID, "SELECT label, count FROM run_digest_label_counts WHERE run_id = ?")
	if err != nil {
		return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: load labels: %w", err)
	}
	summary.CountsByLabel = labels

	// 4) Load sender counts
	senders, err := s.loadCountTable(ctx, summaryRunID, "SELECT sender, count FROM run_digest_sender_counts WHERE run_id = ?")
	if err != nil {
		return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: load senders: %w", err)
	}
	summary.SenderCounts = senders

	// 5) Load domain counts
	domains, err := s.loadCountTable(ctx, summaryRunID, "SELECT domain, count FROM run_digest_domain_counts WHERE run_id = ?")
	if err != nil {
		return nil, fmt.Errorf("store.GetPreviousRunDigestSummary: load domains: %w", err)
	}
	summary.DomainCounts = domains

	return summary, nil
}

// ---------------------------------------------------------------------------
// Sentinels
// ---------------------------------------------------------------------------

// ErrRunNotFound is returned when a run ID is not found in the store.
var ErrRunNotFound = errors.New("run not found")

// timeLayout is the single canonical layout used for every timestamp we
// write to and read from SQLite. Using one explicit, fixed layout on both
// sides avoids depending on the sqlite driver's implicit stringification of
// time.Time, whose output shape varies with the process's local timezone
// database (numeric offset like "+0200" vs. named zone like "UTC") and with
// how many fractional-second digits happen to be non-zero.
const timeLayout = time.RFC3339Nano

// formatTime renders t as a UTC string in timeLayout. All timestamps are
// normalised to UTC before storage so reads never have to worry about zone
// interpretation.
func formatTime(t time.Time) string {
	return t.UTC().Format(timeLayout)
}

// parseTime parses a timestamp string previously written by formatTime.
// It also tolerates legacy rows written before this fix, which may have
// been stored using the sqlite driver's default time.Time stringification
// (e.g. "2006-01-02 15:04:05.999999999 -0700 -07" or "... -0700 MST",
// optionally followed by a monotonic clock suffix like " m=+0.013361351").
func parseTime(s string) (time.Time, error) {
	// Strip a monotonic clock suffix if present (legacy rows only).
	if idx := strings.Index(s, " m="); idx >= 0 {
		s = s[:idx]
	}

	if t, err := time.Parse(timeLayout, s); err == nil {
		return t, nil
	}

	// Legacy fallback formats, kept only for backward compatibility with
	// rows written before this fix. New writes always use timeLayout.
	legacyFormats := []string{
		"2006-01-02 15:04:05.999999999 -0700 -07",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05.999999999 -0700",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02 15:04:05",
	}
	for _, f := range legacyFormats {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("parse time %q: no matching format", s)
}

// generateID produces a random hex ID suitable for use as a run identifier.
func generateID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
