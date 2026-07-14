// Package store provides the state store interface and domain types for
// persisting run metadata, processed messages, flag applications, and
// digest delivery records.
//
// The composite key (account_label, uid) is used throughout for
// idempotent deduplication of processed messages.
package store

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// Composite key
// ---------------------------------------------------------------------------

// MessageKey is the composite dedup key for a processed message.
type MessageKey struct {
	AccountLabel string
	UID          uint32
}

// Key returns the composite key string for dedup lookups.
func (k MessageKey) Key() string {
	return k.AccountLabel + "/" + itoa(k.UID)
}

// itoa is a small helper to avoid importing strconv for the Key method.
// It handles the common case of UIDs being well within uint32 range.
func itoa(n uint32) string {
	if n == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// ---------------------------------------------------------------------------
// Run
// ---------------------------------------------------------------------------

// RunStatus represents the lifecycle state of a pipeline run.
type RunStatus string

const (
	RunStatusRunning     RunStatus = "running"
	RunStatusCompleted   RunStatus = "completed"
	RunStatusDegraded    RunStatus = "degraded"
	RunStatusIngestFailed RunStatus = "ingest_failed"
	RunStatusPartial     RunStatus = "partial"
	RunStatusCancelled   RunStatus = "cancelled"
)

// Run represents a single pipeline execution.
type Run struct {
	ID           string
	StartedAt    time.Time
	FinishedAt   *time.Time // nil while the run is in progress
	Status       RunStatus
	MessageCount int
	Error        string // last non-nil error, empty if none
}

// ---------------------------------------------------------------------------
// ProcessedMessage
// ---------------------------------------------------------------------------

// ProcessedMessage represents a single email that was classified during a run.
type ProcessedMessage struct {
	RunID          string
	AccountLabel   string
	UID            uint32
	IsRead         bool   // whether the \Seen flag was set on the server
	Classification string // LLM-assigned classification label
	DigestExcerpt  string // short excerpt for the digest
	ProcessedAt    time.Time
}

// Key returns the composite dedup key for this message.
func (m ProcessedMessage) Key() MessageKey {
	return MessageKey{AccountLabel: m.AccountLabel, UID: m.UID}
}

// ---------------------------------------------------------------------------
// FlagRecord
// ---------------------------------------------------------------------------

// FlagRecord records that an IMAP keyword flag was applied to a message.
type FlagRecord struct {
	AccountLabel string
	UID          uint32
	Flag         string // e.g. "Useful", "ToDelete", "Ads"
	AppliedAt    time.Time
}

// Key returns the composite dedup key for this flag record.
func (r FlagRecord) Key() MessageKey {
	return MessageKey{AccountLabel: r.AccountLabel, UID: r.UID}
}

// ---------------------------------------------------------------------------
// DigestRecord
// ---------------------------------------------------------------------------

// DigestStatus represents the delivery state of a digest.
type DigestStatus string

const (
	DigestStatusSent    DigestStatus = "sent"
	DigestStatusFailed  DigestStatus = "failed"
	DigestStatusSkipped DigestStatus = "skipped"
)

// DigestRecord represents a digest delivery attempt.
type DigestRecord struct {
	RunID       string
	Channel     string // e.g. "telegram"
	Status      DigestStatus
	PayloadHash string // sha256 of the rendered payload for dedup
}

// ---------------------------------------------------------------------------
// Store interface
// ---------------------------------------------------------------------------

// Store is the persistence contract for the emailer state store.
//
// Implementations must be safe for concurrent use when the orchestrator
// runs multiple accounts in parallel.
type Store interface {
	// Close releases any resources held by the store.
	Close() error

	// -----------------------------------------------------------------------
	// Run lifecycle
	// -----------------------------------------------------------------------

	// RecordRun persists a new run record and returns it with the assigned ID.
	RecordRun(ctx context.Context, r Run) (Run, error)

	// FinishRun updates a run record with completion status, finished_at, and
	// error details.
	FinishRun(ctx context.Context, runID string, status RunStatus, messageCount int, runErr error) error

	// GetRun retrieves a single run by ID.
	GetRun(ctx context.Context, runID string) (Run, error)

	// ListRuns returns the most recent runs, ordered by started_at descending.
	// Limit bounds the result set; use 0 for a sensible default (e.g. 10).
	ListRuns(ctx context.Context, limit int) ([]Run, error)

	// GetLastSuccessfulRunTime returns the finished_at timestamp of the most
	// recent run with status "completed". Returns nil if no successful run
	// exists (or if all runs failed).
	GetLastSuccessfulRunTime(ctx context.Context) (*time.Time, error)

	// -----------------------------------------------------------------------
	// Processed messages
	// -----------------------------------------------------------------------

	// RecordMessage persists a processed message record.
	RecordMessage(ctx context.Context, m ProcessedMessage) error

	// AlreadyProcessed checks whether any of the given message keys have been
	// processed in a previous run. Returns the set of keys that already exist.
	AlreadyProcessed(ctx context.Context, keys []MessageKey) (map[MessageKey]bool, error)

	// -----------------------------------------------------------------------
	// Flag records
	// -----------------------------------------------------------------------

	// RecordFlag persists a flag application record.
	RecordFlag(ctx context.Context, r FlagRecord) error

	// -----------------------------------------------------------------------
	// Digest records
	// -----------------------------------------------------------------------

	// RecordDigest persists a digest delivery record.
	RecordDigest(ctx context.Context, d DigestRecord) error
}