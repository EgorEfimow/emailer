package mail

import (
	"context"
	"testing"

	"github.com/egorefimow/emailer/internal/config"
)

func TestNewIMAPClient(t *testing.T) {
	c := NewIMAPClient()
	if c == nil {
		t.Fatal("NewIMAPClient() returned nil")
	}
	if c.cli != nil {
		t.Fatal("NewIMAPClient() should not have an active connection yet")
	}
}

func TestIMAPClient_Login_WithoutDial(t *testing.T) {
	c := NewIMAPClient()

	err := c.Login(context.Background(), dummyAccount())
	if err == nil {
		t.Fatal("expected error when logging in without dialing first")
	}
}

func TestIMAPClient_Close_WithoutDial(t *testing.T) {
	c := NewIMAPClient()

	// Should not panic or error when no connection exists.
	err := c.Close()
	if err != nil {
		t.Fatalf("unexpected error from Close on unconnected client: %v", err)
	}
}

func TestIMAPClient_Fetch_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic from unimplemented Fetch")
		}
	}()

	c := NewIMAPClient()
	_, _ = c.Fetch(context.Background(), dummyAccount(), FetchOptions{})
}

func TestIMAPClient_ApplyFlags_NotConnected(t *testing.T) {
	c := NewIMAPClient()

	err := c.ApplyFlags(context.Background(), dummyAccount(), []Flag{
		{Key: MessageKey{AccountLabel: "test", UID: 1}, Keyword: "Useful"},
	})
	if err == nil {
		t.Fatal("expected error from ApplyFlags when not connected")
	}
}

func TestIMAPClient_ApplyFlags_EmptyFlags(t *testing.T) {
	c := NewIMAPClient()

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