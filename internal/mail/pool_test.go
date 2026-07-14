package mail

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/config"
)

// ---------------------------------------------------------------------------
// Fake Ingester implementations for pool tests
// ---------------------------------------------------------------------------

// compileCheck ensures our fakes satisfy the interface at compile time.
var _ Ingester = (*fakePoolIngester)(nil)
var _ Ingester = (*slowIngester)(nil)

// fakePoolIngester is a configurable Ingester fake for pool tests.
type fakePoolIngester struct {
	Messages []Message
	FetchErr error
	mu       sync.Mutex
	fetchCalls int
}

func (f *fakePoolIngester) Fetch(_ context.Context, _ config.IMAPAccount, _ FetchOptions) ([]Message, error) {
	f.mu.Lock()
	f.fetchCalls++
	f.mu.Unlock()

	if f.FetchErr != nil {
		return nil, f.FetchErr
	}
	out := make([]Message, len(f.Messages))
	copy(out, f.Messages)
	return out, nil
}

func (f *fakePoolIngester) ApplyFlags(_ context.Context, _ config.IMAPAccount, _ []Flag) error {
	return nil
}

// slowIngester blocks until the context is cancelled, then returns the
// context error. Used to test context cancellation mid-fetch.
type slowIngester struct {
	blockUntil <-chan struct{}
}

func (s *slowIngester) Fetch(ctx context.Context, _ config.IMAPAccount, _ FetchOptions) ([]Message, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.blockUntil:
		return nil, nil
	}
}

func (s *slowIngester) ApplyFlags(_ context.Context, _ config.IMAPAccount, _ []Flag) error {
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestFetchAll_AllSucceed(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	accounts := []config.IMAPAccount{
		{Label: "work", Host: "imap.work.com", Port: 993},
		{Label: "personal", Host: "imap.personal.com", Port: 993},
	}

	ingesters := map[string]Ingester{
		"work": &fakePoolIngester{
			Messages: []Message{
				{AccountLabel: "work", UID: 1, Subject: "Meeting", Date: now},
			},
		},
		"personal": &fakePoolIngester{
			Messages: []Message{
				{AccountLabel: "personal", UID: 1, Subject: "Party", Date: now},
				{AccountLabel: "personal", UID: 2, Subject: "Reminder", Date: now.Add(-1 * time.Hour)},
			},
		},
	}

	results := FetchAll(context.Background(), accounts, ingesters, FetchOptions{Since: now.Add(-24 * time.Hour)}, 4)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	// Check work account.
	if results[0].Err != nil {
		t.Errorf("work account error: %v", results[0].Err)
	}
	if len(results[0].Messages) != 1 {
		t.Errorf("work account: got %d messages, want 1", len(results[0].Messages))
	}

	// Check personal account.
	if results[1].Err != nil {
		t.Errorf("personal account error: %v", results[1].Err)
	}
	if len(results[1].Messages) != 2 {
		t.Errorf("personal account: got %d messages, want 2", len(results[1].Messages))
	}

	// Verify FlattenMessages.
	all := FlattenMessages(results)
	if len(all) != 3 {
		t.Errorf("FlattenMessages: got %d messages, want 3", len(all))
	}
}

func TestFetchAll_OneAccountFails(t *testing.T) {
	accounts := []config.IMAPAccount{
		{Label: "good", Host: "imap.good.com", Port: 993},
		{Label: "bad", Host: "imap.bad.com", Port: 993},
		{Label: "also-good", Host: "imap.also.com", Port: 993},
	}

	ingesters := map[string]Ingester{
		"good": &fakePoolIngester{
			Messages: []Message{{AccountLabel: "good", UID: 1}},
		},
		"bad": &fakePoolIngester{
			FetchErr: errors.New("connection refused"),
		},
		"also-good": &fakePoolIngester{
			Messages: []Message{{AccountLabel: "also-good", UID: 1}},
		},
	}

	results := FetchAll(context.Background(), accounts, ingesters, FetchOptions{}, 2)

	// Check that all three results are returned.
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}

	// Good account should succeed.
	if results[0].Err != nil {
		t.Errorf("good account: unexpected error: %v", results[0].Err)
	}
	if len(results[0].Messages) != 1 {
		t.Errorf("good account: got %d messages, want 1", len(results[0].Messages))
	}

	// Bad account should have the error.
	if results[1].Err == nil {
		t.Error("bad account: expected error, got nil")
	}
	if results[1].Messages != nil {
		t.Errorf("bad account: expected nil messages, got %d", len(results[1].Messages))
	}

	// Also-good account should succeed.
	if results[2].Err != nil {
		t.Errorf("also-good account: unexpected error: %v", results[2].Err)
	}

	// FlattenMessages should skip the failed account.
	all := FlattenMessages(results)
	if len(all) != 2 {
		t.Errorf("FlattenMessages: got %d messages, want 2 (failed account skipped)", len(all))
	}

	// AccountErrors should report the bad account.
	errs := AccountErrors(results)
	if len(errs) != 1 {
		t.Errorf("AccountErrors: got %d errors, want 1", len(errs))
	}
	if _, ok := errs["bad"]; !ok {
		t.Error("AccountErrors: missing 'bad' account")
	}
}

func TestFetchAll_AllAccountsFail(t *testing.T) {
	accounts := []config.IMAPAccount{
		{Label: "a", Host: "imap.a.com", Port: 993},
		{Label: "b", Host: "imap.b.com", Port: 993},
	}

	ingesters := map[string]Ingester{
		"a": &fakePoolIngester{FetchErr: errors.New("timeout")},
		"b": &fakePoolIngester{FetchErr: errors.New("auth failed")},
	}

	results := FetchAll(context.Background(), accounts, ingesters, FetchOptions{}, 2)

	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}

	for _, r := range results {
		if r.Err == nil {
			t.Errorf("account %s: expected error, got nil", r.Account.Label)
		}
	}

	all := FlattenMessages(results)
	if all != nil {
		t.Errorf("FlattenMessages: expected nil, got %d messages", len(all))
	}

	errs := AccountErrors(results)
	if len(errs) != 2 {
		t.Errorf("AccountErrors: got %d errors, want 2", len(errs))
	}
}

func TestFetchAll_NoIngesterRegistered(t *testing.T) {
	accounts := []config.IMAPAccount{
		{Label: "orphan", Host: "imap.orphan.com", Port: 993},
	}

	ingesters := map[string]Ingester{} // empty

	results := FetchAll(context.Background(), accounts, ingesters, FetchOptions{}, 1)

	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}

	if results[0].Err == nil {
		t.Fatal("expected error for missing ingester, got nil")
	}
}

func TestFetchAll_ContextCancelledMidFetch(t *testing.T) {
	blocked := make(chan struct{})
	accounts := []config.IMAPAccount{
		{Label: "slow", Host: "imap.slow.com", Port: 993},
		{Label: "fast", Host: "imap.fast.com", Port: 993},
	}

	ingesters := map[string]Ingester{
		"slow": &slowIngester{blockUntil: blocked},
		"fast": &fakePoolIngester{
			Messages: []Message{{AccountLabel: "fast", UID: 1}},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Run FetchAll in a goroutine and cancel the context after a brief delay.
	type fetchResult struct {
		results []FetchAllResult
	}
	done := make(chan fetchResult, 1)

	go func() {
		results := FetchAll(ctx, accounts, ingesters, FetchOptions{}, 2)
		done <- fetchResult{results: results}
	}()

	// Give the goroutine time to start, then cancel.
	time.Sleep(10 * time.Millisecond)
	cancel()

	// Wait for results.
	select {
	case fr := <-done:
		results := fr.results
		// We expect 2 results.
		if len(results) != 2 {
			t.Fatalf("got %d results, want 2", len(results))
		}

		// The fast account should have succeeded (or may have been cancelled).
		// Either is acceptable — the key assertion is that the framework
		// handles cancellation gracefully without panicking.
		fastResult := results[1] // "fast" is second in the list
		_ = fastResult

		// The slow account should have a context error.
		slowResult := results[0] // "slow" is first
		if slowResult.Err == nil {
			t.Error("slow account: expected context error, got nil")
		}
		_ = slowResult

	case <-time.After(5 * time.Second):
		t.Fatal("FetchAll did not return after context cancellation within 5s")
	}

	// Close the blocked channel so the slow ingester doesn't leak.
	close(blocked)
}

func TestFetchAll_MaxConcurrentZero(t *testing.T) {
	// When maxConcurrent is 0 or negative, it should default to 1.
	accounts := []config.IMAPAccount{
		{Label: "a", Host: "imap.a.com", Port: 993},
	}

	ingesters := map[string]Ingester{
		"a": &fakePoolIngester{
			Messages: []Message{{AccountLabel: "a", UID: 1}},
		},
	}

	results := FetchAll(context.Background(), accounts, ingesters, FetchOptions{}, 0)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
}

func TestFetchAll_MaxConcurrentNegative(t *testing.T) {
	accounts := []config.IMAPAccount{
		{Label: "a", Host: "imap.a.com", Port: 993},
	}

	ingesters := map[string]Ingester{
		"a": &fakePoolIngester{
			Messages: []Message{{AccountLabel: "a", UID: 1}},
		},
	}

	results := FetchAll(context.Background(), accounts, ingesters, FetchOptions{}, -1)
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("unexpected error: %v", results[0].Err)
	}
}

func TestFilterAlreadyProcessed(t *testing.T) {
	processed := map[MessageKey]bool{
		{AccountLabel: "work", UID: 1}: true,
		{AccountLabel: "work", UID: 2}: true,
	}

	messages := []Message{
		{AccountLabel: "work", UID: 1}, // filtered
		{AccountLabel: "work", UID: 2}, // filtered
		{AccountLabel: "work", UID: 3}, // kept
		{AccountLabel: "personal", UID: 1}, // kept
	}

	filtered := FilterAlreadyProcessed(messages, processed)
	if len(filtered) != 2 {
		t.Errorf("got %d messages, want 2", len(filtered))
	}

	for _, m := range filtered {
		if processed[m.Key()] {
			t.Errorf("message %v should have been filtered", m.Key())
		}
	}
}

func TestFilterAlreadyProcessed_EmptyProcessed(t *testing.T) {
	messages := []Message{
		{AccountLabel: "work", UID: 1},
		{AccountLabel: "work", UID: 2},
	}

	filtered := FilterAlreadyProcessed(messages, nil)
	if len(filtered) != 2 {
		t.Errorf("got %d messages, want 2 (no filter)", len(filtered))
	}
}

func TestGroupByAccount(t *testing.T) {
	messages := []Message{
		{AccountLabel: "work", UID: 1},
		{AccountLabel: "personal", UID: 1},
		{AccountLabel: "work", UID: 2},
		{AccountLabel: "personal", UID: 2},
		{AccountLabel: "work", UID: 3},
	}

	groups := GroupByAccount(messages)
	if len(groups) != 2 {
		t.Fatalf("got %d groups, want 2", len(groups))
	}
	if len(groups["work"]) != 3 {
		t.Errorf("work account: got %d messages, want 3", len(groups["work"]))
	}
	if len(groups["personal"]) != 2 {
		t.Errorf("personal account: got %d messages, want 2", len(groups["personal"]))
	}
}

func TestGroupByAccount_Empty(t *testing.T) {
	groups := GroupByAccount(nil)
	if groups == nil {
		t.Error("expected non-nil map for empty input")
	}
	if len(groups) != 0 {
		t.Errorf("expected empty map, got %d entries", len(groups))
	}
}

func TestBoundedSemaphore_AcquireRelease(t *testing.T) {
	sem := NewBoundedSemaphore(2)

	ctx := context.Background()
	if err := sem.Acquire(ctx); err != nil {
		t.Fatalf("acquire 1: %v", err)
	}
	if err := sem.Acquire(ctx); err != nil {
		t.Fatalf("acquire 2: %v", err)
	}

	// The third acquire should block (but we're not testing that here).
	// Just release what we acquired.
	sem.Release()
	sem.Release()
}

func TestBoundedSemaphore_CapacityZero(t *testing.T) {
	sem := NewBoundedSemaphore(0)
	if cap(sem.ch) != 1 {
		t.Errorf("expected capacity 1, got %d", cap(sem.ch))
	}
}

func TestBoundedSemaphore_ContextCancelled(t *testing.T) {
	sem := NewBoundedSemaphore(1)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	if err := sem.Acquire(ctx); err == nil {
		t.Error("expected error on cancelled context, got nil")
	}
}

func TestWaitGroupWithTimeout_WaitCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var wg WaitGroupWithTimeout

	wg.Add(1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		wg.Done()
	}()

	// Cancel before the WaitGroup finishes.
	cancel()

	err := wg.Wait(ctx)
	if err == nil {
		t.Error("expected context error, got nil")
	}
}

func TestWaitGroupWithTimeout_WaitCompletes(t *testing.T) {
	ctx := context.Background()
	var wg WaitGroupWithTimeout

	wg.Add(1)
	go func() {
		time.Sleep(10 * time.Millisecond)
		wg.Done()
	}()

	err := wg.Wait(ctx)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}