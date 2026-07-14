package mail

import (
	"context"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/config"
)

// compileCheck ensures our fake Ingester satisfies the interface at compile time.
var _ Ingester = (*fakeIngester)(nil)

// fakeIngester is a minimal in-memory implementation of Ingester for use in
// unit tests that need an Ingester, but do not need real IMAP connectivity.
type fakeIngester struct {
	Messages []Message
	Flags    []Flag
	FetchErr error
	FlagErr  error
}

func (f *fakeIngester) Fetch(_ context.Context, account config.IMAPAccount, opts FetchOptions) ([]Message, error) {
	if f.FetchErr != nil {
		return nil, f.FetchErr
	}
	// Return a copy so the caller can't mutate the fixture.
	out := make([]Message, len(f.Messages))
	copy(out, f.Messages)
	return out, nil
}

func (f *fakeIngester) ApplyFlags(_ context.Context, _ config.IMAPAccount, flags []Flag) error {
	if f.FlagErr != nil {
		return f.FlagErr
	}
	f.Flags = append(f.Flags, flags...)
	return nil
}

func TestIngester_Fetch_HappyPath(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	f := &fakeIngester{
		Messages: []Message{
			{AccountLabel: "work", UID: 1, Subject: "Hello", Date: now},
			{AccountLabel: "work", UID: 2, Subject: "World", Date: now.Add(-1 * time.Hour)},
		},
	}

	msgs, err := f.Fetch(context.Background(), config.IMAPAccount{Label: "work"}, FetchOptions{Since: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("got %d messages, want 2", len(msgs))
	}
	if msgs[0].Subject != "Hello" {
		t.Errorf("first message subject = %q, want %q", msgs[0].Subject, "Hello")
	}
}

func TestIngester_Fetch_Error(t *testing.T) {
	f := &fakeIngester{FetchErr: assertAnError("connection refused")}

	_, err := f.Fetch(context.Background(), config.IMAPAccount{}, FetchOptions{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestIngester_ApplyFlags_HappyPath(t *testing.T) {
	f := &fakeIngester{}

	err := f.ApplyFlags(context.Background(), config.IMAPAccount{Label: "work"}, []Flag{
		{Key: MessageKey{AccountLabel: "work", UID: 1}, Keyword: "Useful"},
		{Key: MessageKey{AccountLabel: "work", UID: 2}, Keyword: "ToDelete"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(f.Flags) != 2 {
		t.Fatalf("got %d flags, want 2", len(f.Flags))
	}
	if f.Flags[0].Keyword != "Useful" {
		t.Errorf("first flag keyword = %q, want %q", f.Flags[0].Keyword, "Useful")
	}
}

func TestIngester_ApplyFlags_Error(t *testing.T) {
	f := &fakeIngester{FlagErr: assertAnError("store failed")}

	err := f.ApplyFlags(context.Background(), config.IMAPAccount{}, []Flag{
		{Key: MessageKey{AccountLabel: "work", UID: 1}, Keyword: "Useful"},
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestFetchOptions_Defaults(t *testing.T) {
	var opts FetchOptions

	if !opts.Since.IsZero() {
		t.Errorf("Since should be zero value by default, got %v", opts.Since)
	}
	if opts.FetchUnreadOnly {
		t.Error("FetchUnreadOnly should be false by default")
	}
	if opts.Folders != nil {
		t.Errorf("Folders should be nil by default, got %v", opts.Folders)
	}
}

// assertAnError returns a simple error for testing error paths.
type assertAnError string

func (e assertAnError) Error() string { return string(e) }