package mail

import (
	"context"
	"testing"
	"time"
)

func TestNewSearchWindowCriteria_Since(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	criteria := newSearchWindowCriteria(since, false)

	if criteria.Since != since {
		t.Errorf("Since = %v, want %v", criteria.Since, since)
	}
}

func TestNewSearchWindowCriteria_UnreadOnly(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	criteria := newSearchWindowCriteria(since, true)

	if len(criteria.WithoutFlags) != 1 {
		t.Fatalf("expected 1 WithoutFlag, got %v", criteria.WithoutFlags)
	}
	if criteria.WithoutFlags[0] != "\\Seen" {
		t.Errorf("WithoutFlags[0] = %q, want %q", criteria.WithoutFlags[0], "\\Seen")
	}
}

func TestNewSearchWindowCriteria_UnreadOnlyFalse(t *testing.T) {
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	criteria := newSearchWindowCriteria(since, false)

	if len(criteria.WithoutFlags) != 0 {
		t.Errorf("expected no WithoutFlags when unreadOnly=false, got %v", criteria.WithoutFlags)
	}
}

func TestNewSearchWindowCriteria_ZeroTime(t *testing.T) {
	criteria := newSearchWindowCriteria(time.Time{}, false)

	if !criteria.Since.IsZero() {
		t.Errorf("Since should be zero, got %v", criteria.Since)
	}
}

func TestSearchByWindow_NotConnected(t *testing.T) {
	c := NewIMAPClient()
	since := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

	_, err := c.searchByWindow(context.Background(), since, false)
	if err == nil {
		t.Fatal("expected error from searchByWindow when not connected")
	}
}

func TestSelectFolder_NotConnected(t *testing.T) {
	c := NewIMAPClient()

	_, err := c.selectFolder(context.Background(), "INBOX", true)
	if err == nil {
		t.Fatal("expected error from selectFolder when not connected")
	}
}