package mail

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"

	"github.com/egorefimow/emailer/internal/config"
)

// newTestClient returns an IMAPClient wired with a discard logger for use in
// unit tests that exercise logic before any network call.
func newTestClient(t *testing.T) *IMAPClient {
	t.Helper()
	c, err := NewIMAPClient(slog.New(slog.NewTextHandler(io.Discard, nil)), 0)
	if err != nil {
		t.Fatalf("NewIMAPClient: %v", err)
	}
	return c
}

func TestNewIMAPClient(t *testing.T) {
	c := newTestClient(t)
	if c == nil {
		t.Fatal("NewIMAPClient() returned nil")
	}
	if c.cli != nil {
		t.Fatal("NewIMAPClient() should not have an active connection yet")
	}
}

func TestNewIMAPClient_NilLogger(t *testing.T) {
	if _, err := NewIMAPClient(nil, 0); err == nil {
		t.Fatal("expected error when constructing with a nil logger")
	}
}

func TestIsConnectionError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "io.EOF", err: io.EOF, want: true},
		{name: "net op error", err: &net.OpError{Op: "read", Err: errors.New("timeout")}, want: true},
		{name: "use of closed connection", err: errors.New("read tcp: use of closed network connection"), want: true},
		{name: "connection reset", err: errors.New("connection reset by peer"), want: true},
		{name: "broken pipe", err: errors.New("write: broken pipe"), want: true},
		{name: "imap no response", err: errors.New("imap: expected OK response"), want: false},
		{name: "semantic error", err: fmt.Errorf("imap.uid_store: NO cannot"), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isConnectionError(tt.err); got != tt.want {
				t.Errorf("isConnectionError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestKeywordAllowed(t *testing.T) {
	tests := []struct {
		name          string
		permanent     []string
		keyword       string
		want          bool
	}{
		{name: "system flag always allowed", permanent: nil, keyword: "\\Seen", want: true},
		{name: "wildcard allows custom", permanent: []string{"\\*"}, keyword: "Useful", want: true},
		{name: "explicit keyword listed", permanent: []string{"\\Seen", "Useful"}, keyword: "Useful", want: true},
		{name: "case-insensitive match", permanent: []string{"useful"}, keyword: "Useful", want: true},
		{name: "keyword not listed", permanent: []string{"\\Seen", "ToDelete"}, keyword: "Useful", want: false},
		{name: "empty permanentflags attempts", permanent: nil, keyword: "Useful", want: true},
		{name: "empty permanentflags system flag", permanent: []string{}, keyword: "\\Flagged", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keywordAllowed(tt.permanent, tt.keyword); got != tt.want {
				t.Errorf("keywordAllowed(%v, %q) = %v, want %v", tt.permanent, tt.keyword, got, tt.want)
			}
		})
	}
}

func TestIMAPClient_Login_WithoutDial(t *testing.T) {
	c := newTestClient(t)

	err := c.Login(context.Background(), dummyAccount())
	if err == nil {
		t.Fatal("expected error when logging in without dialing first")
	}
}

func TestIMAPClient_Close_WithoutDial(t *testing.T) {
	c := newTestClient(t)

	// Should not panic or error when no connection exists.
	err := c.Close()
	if err != nil {
		t.Fatalf("unexpected error from Close on unconnected client: %v", err)
	}
}

func TestIMAPClient_Fetch_NotConnected(t *testing.T) {
	c := newTestClient(t)

	_, err := c.Fetch(context.Background(), dummyAccount(), FetchOptions{})
	if err == nil {
		t.Fatal("expected error from Fetch when not connected")
	}
}

func TestIMAPClient_ApplyFlags_NotConnected(t *testing.T) {
	c := newTestClient(t)

	err := c.ApplyFlags(context.Background(), dummyAccount(), []Flag{
		{Key: MessageKey{AccountLabel: "test", UID: 1}, Keyword: "Useful"},
	})
	if err == nil {
		t.Fatal("expected error from ApplyFlags when not connected")
	}
}

func TestIMAPClient_ApplyFlags_EmptyFlags(t *testing.T) {
	c := newTestClient(t)

	// Empty flags should not error, even without a connection.
	err := c.ApplyFlags(context.Background(), dummyAccount(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// dummyAccount returns a minimal IMAPAccount for use in unit tests that
// exercise logic before any network call.
func dummyAccount() config.IMAPAccount {
	return config.IMAPAccount{
		Label:    "test",
		Host:     "imap.example.com",
		Port:     993,
		Username: "user",
		Password: "pass",
		Folders:  []string{"INBOX"},
		UseTLS:   true,
	}
}