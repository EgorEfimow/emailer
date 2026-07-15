package digest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

func TestMarkdownRenderer_HappyPath(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 30, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:           "run-abc-123",
		GeneratedAt:     now,
		AccountLabel:    "work",
		TotalFetched:    3,
		TotalClassified: 2,
		FailedCount:     0,
		Messages: []MessageEntry{
			{
				Subject: "Project Update",
				From:    "alice@example.com",
				Date:    time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC),
				IsRead:  true,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "work", UID: 1},
					Label:      "Useful",
					Confidence: 0.95,
					Reason:     "Important project update",
				},
				Excerpt: "The project is on track for the July release.",
			},
			{
				Subject: "Special Offer",
				From:    "marketing@spam.com",
				Date:    time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC),
				IsRead:  false,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "work", UID: 2},
					Label:      "Ads",
					Confidence: 0.88,
					Reason:     "Marketing newsletter",
				},
				Excerpt: "Buy now and save 50% on your purchase!",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Check header.
	if !strings.Contains(result, "Email Digest") {
		t.Error("result missing 'Email Digest' header")
	}
	if !strings.Contains(result, "run-abc-123") {
		t.Error("result missing run ID")
	}

	// Check account label.
	if !strings.Contains(result, "work") {
		t.Error("result missing account label")
	}

	// Check classification sections.
	if !strings.Contains(result, "## Useful") {
		t.Error("result missing 'Useful' section header")
	}
	if !strings.Contains(result, "## Ads") {
		t.Error("result missing 'Ads' section header")
	}

	// Check message details.
	if !strings.Contains(result, "Project Update") {
		t.Error("result missing message subject")
	}
	if !strings.Contains(result, "alice@example.com") {
		t.Error("result missing from address")
	}

	// Check read/unread status.
	if !strings.Contains(result, "✅ Read") {
		t.Error("result missing read status badge")
	}
	if !strings.Contains(result, "🆕 Unread") {
		t.Error("result missing unread status badge")
	}

	// Check dates are rendered.
	if !strings.Contains(result, "2026-07-14") {
		t.Error("result missing date rendering")
	}

	// Check confidence.
	if !strings.Contains(result, "95%") {
		t.Error("result missing confidence percentage")
	}
	if !strings.Contains(result, "88%") {
		t.Error("result missing confidence percentage for Ads")
	}

	// Check reasons.
	if !strings.Contains(result, "Important project update") {
		t.Error("result missing classification reason")
	}

	// Check excerpts.
	if !strings.Contains(result, "The project is on track") {
		t.Error("result missing message excerpt")
	}

	// Check total message count.
	if !strings.Contains(result, "2 classified") {
		t.Error("result missing classified count")
	}
}

func TestMarkdownRenderer_EmptyMessages(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:        "run-empty",
		GeneratedAt:  now,
		TotalFetched: 0,
		FailedCount:  0,
		Messages:     nil,
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "0 classified") {
		t.Error("result should show 0 messages")
	}
}

func TestMarkdownRenderer_NoReadStatus(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(false, 200)

	data := DigestData{
		RunID:       "run-no-status",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Test",
				From:    "test@example.com",
				Date:    time.Now(),
				IsRead:  true,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 1},
					Label:      "Useful",
					Confidence: 0.9,
					Reason:     "Test",
				},
				Excerpt: "Test body",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if strings.Contains(result, "✅ Read") || strings.Contains(result, "🆕 Unread") {
		t.Error("result should not contain read/unread status when IncludeReadStatus is false")
	}
}

func TestMarkdownRenderer_ExcerptTruncation(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(true, 20) // Very short limit.

	longBody := "This is a very long body that should be truncated to twenty characters."
	data := DigestData{
		RunID:       "run-truncate",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Test",
				From:    "test@example.com",
				Date:    time.Now(),
				IsRead:  false,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 1},
					Label:      "Useful",
					Confidence: 0.9,
					Reason:     "Test",
				},
				Excerpt: longBody,
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// The excerpt should be truncated with "…".
	if !strings.Contains(result, "…") {
		t.Error("truncated excerpt should end with …")
	}
}

func TestMarkdownRenderer_UnknownLabel(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-unknown",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Mystery",
				From:    "unknown@example.com",
				Date:    time.Now(),
				IsRead:  true,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 1},
					Label:      "", // Empty label maps to "Unknown".
					Confidence: 0.5,
					Reason:     "Could not determine",
				},
				Excerpt: "Mysterious content",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "## Unknown") {
		t.Error("result should have 'Unknown' section for empty label messages")
	}
}

func TestMarkdownRenderer_Name(t *testing.T) {
	r := NewMarkdownRenderer(true, 200)
	if r.Name() != "markdown" {
		t.Errorf("Name() = %q, want %q", r.Name(), "markdown")
	}
}

func TestMarkdownRenderer_MultipleLabels(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-multi",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Work item",
				From:    "a@work.com",
				Date:    time.Now(),
				IsRead:  true,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 1},
					Label:      "Useful",
					Confidence: 0.9,
					Reason:     "Work related",
				},
				Excerpt: "Work content",
			},
			{
				Subject: "Spam",
				From:    "spam@spam.com",
				Date:    time.Now(),
				IsRead:  false,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 2},
					Label:      "ToDelete",
					Confidence: 0.95,
					Reason:     "Spam",
				},
				Excerpt: "Spam content",
			},
			{
				Subject: "Promo",
				From:    "ads@marketing.com",
				Date:    time.Now(),
				IsRead:  false,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 3},
					Label:      "Ads",
					Confidence: 0.85,
					Reason:     "Promotional",
				},
				Excerpt: "Promo content",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// All three labels should appear in alphabetical order.
	usefulPos := strings.Index(result, "## Useful")
	toDeletePos := strings.Index(result, "## ToDelete")
	adsPos := strings.Index(result, "## Ads")

	if usefulPos < 0 {
		t.Error("missing 'Useful' section")
	}
	if toDeletePos < 0 {
		t.Error("missing 'ToDelete' section")
	}
	if adsPos < 0 {
		t.Error("missing 'Ads' section")
	}

	// Verify alphabetical order: Ads, ToDelete, Useful.
	if !(adsPos < toDeletePos && toDeletePos < usefulPos) {
		t.Error("sections not in alphabetical order")
	}
}

func TestMarkdownRenderer_ContextCancelled(t *testing.T) {
	r := NewMarkdownRenderer(true, 200)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	data := DigestData{
		RunID:       "run-cancel",
		GeneratedAt: time.Now(),
	}

	// Template execution does not respect context cancellation directly,
	// but we should not panic or deadlock.
	_, err := r.Render(ctx, data)
	if err != nil {
		// Context cancellation is not expected to cause errors in template
		// rendering, but if it does, it should be a non-fatal error.
		t.Logf("Render() with cancelled context returned error: %v", err)
	}
}
func TestMarkdownRenderer_RendersStatsBlocksBeforeMessages(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:           "run-stats",
		GeneratedAt:     now,
		TotalFetched:    3,
		TotalClassified: 2,
		FailedCount:     1,
		GlobalStats: DigestStats{
			FetchedCount:    3,
			ClassifiedCount: 2,
			FailedCount:     1,
			ReadCount:       1,
			UnreadCount:     2,
			CountsByLabel: map[string]int{
				"Ads":     1,
				"Unknown": 1,
				"Useful":  1,
			},
		},
		AccountStats: []AccountStats{
			{
				AccountLabel:    "work",
				FetchedCount:    2,
				ClassifiedCount: 1,
				FailedCount:     1,
				ReadCount:       1,
				UnreadCount:     1,
				CountsByLabel: map[string]int{
					"Unknown": 1,
					"Useful":  1,
				},
			},
			{
				AccountLabel:    "personal",
				FetchedCount:    1,
				ClassifiedCount: 1,
				UnreadCount:     1,
				CountsByLabel: map[string]int{
					"Ads": 1,
				},
				Status: "error",
				Error:  "imap timeout",
			},
		},
		Messages: []MessageEntry{
			{
				Subject: "Project update",
				From:    "pm@example.com",
				Date:    now,
				IsRead:  true,
				Classification: mail.Classification{
					Label:      "Useful",
					Confidence: 0.9,
					Reason:     "Important",
				},
				Excerpt: "The project is on track.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	ordered := []string{
		"## Summary",
		"**Fetched:** 3",
		"**Classified:** 2",
		"**Failed:** 1",
		"**Read:** 1",
		"**Unread:** 2",
		"**Labels:** Ads=1 Unknown=1 Useful=1",
		"## Account Stats",
		"### work",
		"**Fetched:** 2 | **Classified:** 1 | **Failed:** 1",
		"**Read:** 1 | **Unread:** 1",
		"**Labels:** Unknown=1 Useful=1",
		"### personal",
		"⚠️ **Fetch error:** imap timeout",
		"## Useful (1)",
		"### 1. Project update",
	}

	last := -1
	for _, want := range ordered {
		idx := strings.Index(result, want)
		if idx == -1 {
			t.Fatalf("result missing %q\n%s", want, result)
		}
		if idx < last {
			t.Fatalf("%q rendered out of order\n%s", want, result)
		}
		last = idx
	}
}
