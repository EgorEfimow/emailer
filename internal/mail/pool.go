package mail

import (
	"context"
	"fmt"
	"sync"

	"github.com/egorefimow/emailer/internal/config"
	"golang.org/x/sync/errgroup"
)

// ---------------------------------------------------------------------------
// FetchAllResult
// ---------------------------------------------------------------------------

// FetchAllResult holds the fetch outcome for a single account.
type FetchAllResult struct {
	Account  config.IMAPAccount
	Messages []Message
	Err      error
}

// ---------------------------------------------------------------------------
// FetchAll
// ---------------------------------------------------------------------------

// FetchAll fetches messages from all accounts concurrently using the provided
// ingesters map (account label → Ingester). Concurrency is limited by
// maxConcurrent (bounded semaphore via errgroup).
//
// Partial failures are collected per-account. The caller decides whether to
// abort based on the results. Results are returned in the same order as the
// input accounts slice.
func FetchAll(ctx context.Context, accounts []config.IMAPAccount, ingesters map[string]Ingester, opts FetchOptions, maxConcurrent int) []FetchAllResult {
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	results := make([]FetchAllResult, len(accounts))

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(maxConcurrent)

	for i, acct := range accounts {
		i, acct := i, acct // capture for closure

		g.Go(func() error {
			// Check context early — respect cancellation.
			select {
			case <-ctx.Done():
				results[i] = FetchAllResult{
					Account: acct,
					Err:     fmt.Errorf("fetch_all: %s: %w", acct.Label, ctx.Err()),
				}
				// Return nil so errgroup doesn't cancel other goroutines.
				// We collect the error in the result struct instead.
				return nil
			default:
			}

			in, ok := ingesters[acct.Label]
			if !ok {
				results[i] = FetchAllResult{
					Account: acct,
					Err:     fmt.Errorf("fetch_all: %s: no ingester registered", acct.Label),
				}
				return nil
			}

			msgs, err := in.Fetch(ctx, acct, opts)
			results[i] = FetchAllResult{
				Account:  acct,
				Messages: msgs,
				Err:      err,
			}
			return nil
		})
	}

	// Wait for all goroutines to finish.
	_ = g.Wait() //nolint:errcheck // errors are collected in results, not propagated

	return results
}

// ---------------------------------------------------------------------------
// FlattenMessages
// ---------------------------------------------------------------------------

// FlattenMessages extracts all non-error messages from FetchAll results into
// a single slice. Accounts with errors are skipped so the caller can still
// process partial results.
func FlattenMessages(results []FetchAllResult) []Message {
	var total int
	for _, r := range results {
		if r.Err == nil {
			total += len(r.Messages)
		}
	}
	if total == 0 {
		return nil
	}

	out := make([]Message, 0, total)
	for _, r := range results {
		if r.Err == nil {
			out = append(out, r.Messages...)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// FilterAlreadyProcessed
// ---------------------------------------------------------------------------

// FilterAlreadyProcessed removes messages whose composite keys appear in the
// processed set. Returns only messages that have not been processed before.
func FilterAlreadyProcessed(messages []Message, processed map[MessageKey]bool) []Message {
	if len(processed) == 0 {
		return messages
	}

	out := make([]Message, 0, len(messages))
	for _, m := range messages {
		if !processed[m.Key()] {
			out = append(out, m)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// GroupByAccount
// ---------------------------------------------------------------------------

// GroupByAccount groups messages by their account label. Returns a map from
// label to the slice of messages belonging to that account.
func GroupByAccount(messages []Message) map[string][]Message {
	groups := make(map[string][]Message)
	for _, m := range messages {
		groups[m.AccountLabel] = append(groups[m.AccountLabel], m)
	}
	return groups
}

// ---------------------------------------------------------------------------
// AccountErrors
// ---------------------------------------------------------------------------

// AccountErrors returns the set of accounts that had fetch errors, for use
// in alerting and logging.
func AccountErrors(results []FetchAllResult) map[string]error {
	errs := make(map[string]error)
	for _, r := range results {
		if r.Err != nil {
			errs[r.Account.Label] = r.Err
		}
	}
	return errs
}

// ---------------------------------------------------------------------------
// BoundedSemaphore (simple helper)
// ---------------------------------------------------------------------------

// BoundedSemaphore is a simple channel-based semaphore for limiting
// concurrency. Used internally by FetchAll via errgroup.SetLimit, but
// exported for use in other parts of the pipeline.
type BoundedSemaphore struct {
	ch chan struct{}
}

// NewBoundedSemaphore creates a semaphore with the given capacity.
func NewBoundedSemaphore(n int) *BoundedSemaphore {
	if n <= 0 {
		n = 1
	}
	return &BoundedSemaphore{ch: make(chan struct{}, n)}
}

// Acquire blocks until a slot is available or the context is cancelled.
// The context is checked first so a cancelled context is honoured even
// when the semaphore has available capacity.
func (s *BoundedSemaphore) Acquire(ctx context.Context) error {
	// Fast path: check context before attempting to acquire.
	if err := ctx.Err(); err != nil {
		return err
	}

	select {
	case s.ch <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns a slot to the pool.
func (s *BoundedSemaphore) Release() {
	<-s.ch
}

// WaitGroupWithTimeout is a convenience wrapper around sync.WaitGroup with
// a context-based timeout.
type WaitGroupWithTimeout struct {
	wg sync.WaitGroup
}

// Add adds delta to the WaitGroup counter.
func (w *WaitGroupWithTimeout) Add(delta int) {
	w.wg.Add(delta)
}

// Done decrements the WaitGroup counter.
func (w *WaitGroupWithTimeout) Done() {
	w.wg.Done()
}

// Wait blocks until the WaitGroup counter is zero or the context is cancelled.
// Returns nil on completion, or ctx.Err() if the context was cancelled.
func (w *WaitGroupWithTimeout) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}