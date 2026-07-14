package store

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// NoopStore is an in-memory implementation of Store for use in stateless mode.
// It stores records in maps and provides the same interface as SQLiteStore but
// without persistence.
//
// NoopStore is safe for concurrent use.
type NoopStore struct {
	mu          sync.RWMutex
	runs        []Run
	messages    map[MessageKey]ProcessedMessage
	flags       []FlagRecord
	digests     []DigestRecord
	nextID      int
}

// NewNoopStore creates a new empty NoopStore.
func NewNoopStore() *NoopStore {
	return &NoopStore{
		messages: make(map[MessageKey]ProcessedMessage),
	}
}

// Close is a no-op for NoopStore.
func (s *NoopStore) Close() error {
	return nil
}

// RecordRun persists a new run record in memory. The returned run is assigned
// a synthetic run ID.
func (s *NoopStore) RecordRun(_ context.Context, r Run) (Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextID++
	if r.ID == "" {
		r.ID = noopRunID(s.nextID)
	}
	if r.StartedAt.IsZero() {
		r.StartedAt = time.Now()
	}
	if r.Status == "" {
		r.Status = RunStatusRunning
	}

	s.runs = append(s.runs, r)
	return r, nil
}

// FinishRun updates the most recent matching run by ID with completion details.
func (s *NoopStore) FinishRun(_ context.Context, runID string, status RunStatus, messageCount int, runErr error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	errStr := ""
	if runErr != nil {
		errStr = runErr.Error()
	}

	now := time.Now()
	for i := range s.runs {
		if s.runs[i].ID == runID {
			s.runs[i].FinishedAt = &now
			s.runs[i].Status = status
			s.runs[i].MessageCount = messageCount
			s.runs[i].Error = errStr
			return nil
		}
	}
	return ErrRunNotFound
}

// GetRun retrieves a run by ID.
func (s *NoopStore) GetRun(_ context.Context, runID string) (Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, r := range s.runs {
		if r.ID == runID {
			return r, nil
		}
	}
	return Run{}, ErrRunNotFound
}

// ListRuns returns the most recent runs, ordered by started_at descending.
func (s *NoopStore) ListRuns(_ context.Context, limit int) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Copy runs in reverse order (most recent first).
	total := len(s.runs)
	if limit > total {
		limit = total
	}

	result := make([]Run, limit)
	for i := 0; i < limit; i++ {
		result[i] = s.runs[total-1-i]
	}

	return result, nil
}

// GetLastSuccessfulRunTime returns the finished_at of the most recent
// completed run, or nil if none exists.
func (s *NoopStore) GetLastSuccessfulRunTime(_ context.Context) (*time.Time, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var latest *time.Time
	for _, r := range s.runs {
		if r.Status == RunStatusCompleted && r.FinishedAt != nil {
			if latest == nil || r.FinishedAt.After(*latest) {
				latest = r.FinishedAt
			}
		}
	}
	return latest, nil
}

// RecordMessage persists a processed message. Duplicates are silently ignored.
func (s *NoopStore) RecordMessage(_ context.Context, m ProcessedMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := m.Key()
	if _, exists := s.messages[key]; exists {
		return nil // idempotent: ignore duplicates
	}

	if m.ProcessedAt.IsZero() {
		m.ProcessedAt = time.Now()
	}

	s.messages[key] = m
	return nil
}

// AlreadyProcessed checks which keys already exist in the store.
func (s *NoopStore) AlreadyProcessed(_ context.Context, keys []MessageKey) (map[MessageKey]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make(map[MessageKey]bool, len(keys))
	for _, k := range keys {
		if _, exists := s.messages[k]; exists {
			result[k] = true
		}
	}
	return result, nil
}

// RecordFlag persists a flag application record. Duplicates are silently ignored.
func (s *NoopStore) RecordFlag(_ context.Context, r FlagRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if r.AppliedAt.IsZero() {
		r.AppliedAt = time.Now()
	}

	// Check for duplicates (same account_label, uid, flag).
	for _, existing := range s.flags {
		if existing.AccountLabel == r.AccountLabel &&
			existing.UID == r.UID &&
			existing.Flag == r.Flag {
			return nil
		}
	}

	s.flags = append(s.flags, r)
	return nil
}

// RecordDigest persists a digest delivery record.
func (s *NoopStore) RecordDigest(_ context.Context, d DigestRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.digests = append(s.digests, d)
	return nil
}

// noopRunID generates a deterministic run ID for NoopStore.
func noopRunID(n int) string {
	return fmt.Sprintf("noop-run-%d", n)
}

// Compile-time check: *NoopStore implements Store.
var _ Store = (*NoopStore)(nil)
