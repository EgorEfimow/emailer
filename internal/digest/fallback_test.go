package digest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/egorefimow/emailer/internal/mail"
)

func TestFallbackRenderer_HappyPath(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 30, 0, 0, time.UTC)
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  true,
		MaxMessageExcerpt:  200,
	})

	data := DigestData{
		RunID:        "fallback-run-1",
		GeneratedAt:  now,
		AccountLabel: "personal",
		TotalFetched: 2,
		FailedCount:  1,
		Messages: []MessageEntry{
			{
				Subject: "Hello",
				From:    "friend@example.com",
				Date:    time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC),
				IsRead:  true,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "personal", UID: 1},
					Label:      "",
					Confidence: 0,
					Reason:     "",
				},
				Excerpt: "Hey, how are you doing?",
			},
			{
				Subject: "Newsletter",
				From:    "news@example.com",
				Date:    time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC),
				IsRead:  false,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "personal", UID: 2},
					Label:      "",
					Confidence: 0,
					Reason:     "",
				},
				Excerpt: "This week in tech news...",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Check header.
	if !strings.Contains(result, "Fallback") {
		t.Error("result missing 'Fallback' indicator")
	}
	if !strings.Contains(result, "fallback-run-1") {
		t.Error("result missing run ID")
	}
	if !strings.Contains(result, "personal") {
		t.Error("result missing account label")
	}

	// Check warning about LLM.
	if !strings.Contains(result, "LLM classification was unavailable") {
		t.Error("result missing LLM unavailable warning")
	}

	// Check message details.
	if !strings.Contains(result, "Hello") {
		t.Error("result missing message subject")
	}
	if !strings.Contains(result, "friend@example.com") {
		t.Error("result missing from address")
	}

	// Check read/unread status.
	if !strings.Contains(result, "✅ Read") {
		t.Error("result missing read status badge")
	}
	if !strings.Contains(result, "🆕 Unread") {
		t.Error("result missing unread status badge")
	}

	// Check dates.
	if !strings.Contains(result, "2026-07-14") {
		t.Error("result missing date rendering")
	}

	// Check excerpts.
	if !strings.Contains(result, "Hey, how are you doing?") {
		t.Error("result missing message excerpt")
	}
}

func TestFallbackRenderer_EmptyMessages(t *testing.T) {
	now := time.Now()
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  true,
		MaxMessageExcerpt:  200,
	})

	data := DigestData{
		RunID:        "fallback-empty",
		GeneratedAt:  now,
		TotalFetched: 0,
		FailedCount:  0,
		Messages:     nil,
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "0") {
		t.Error("fallback result should show message count even when empty")
	}
}

func TestFallbackRenderer_NoReadStatus(t *testing.T) {
	now := time.Now()
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  false,
		MaxMessageExcerpt:  200,
	})

	data := DigestData{
		RunID:       "fallback-no-status",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Test",
				From:    "test@example.com",
				Date:    time.Now(),
				IsRead:  true,
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

func TestFallbackRenderer_ExcerptTruncation(t *testing.T) {
	now := time.Now()
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  true,
		MaxMessageExcerpt:  10, // Very short limit.
	})

	longBody := "This is a very long body that should be truncated."
	data := DigestData{
		RunID:       "fallback-truncate",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Test",
				From:    "test@example.com",
				Date:    time.Now(),
				IsRead:  false,
				Excerpt: longBody,
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "…") {
		t.Error("truncated excerpt should end with …")
	}
}

func TestFallbackRenderer_Name(t *testing.T) {
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  true,
		MaxMessageExcerpt:  200,
	})
	if r.Name() != "fallback" {
		t.Errorf("Name() = %q, want %q", r.Name(), "fallback")
	}
}

func TestFallbackRenderer_ContextCancelled(t *testing.T) {
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  true,
		MaxMessageExcerpt:  200,
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	data := DigestData{
		RunID:       "fallback-cancel",
		GeneratedAt: time.Now(),
	}

	_, err := r.Render(ctx, data)
	if err != nil {
		t.Logf("Render() with cancelled context returned error: %v", err)
	}
}

func TestMarkdownRenderer_MessageCount(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(config.DigestConfig{
		IncludeReadStatus:         true,
		MaxMessageExcerpt:         200,
		IncludeGlobalStats:        true,
		IncludeAccountStats:       true,
		IncludeSummaries:          true,
		IncludeKeyPoints:          true,
		IncludeActionItems:        true,
		IncludeRawExcerptFallback: true,
		MaxMessages:               100,
		MaxKeyPointsPerMessage:    5,
		MaxActionItemsPerMessage:  3,
		PriorityOnly:              false,
	})

	data := DigestData{
		RunID:        "run-count",
		GeneratedAt:  now,
		TotalFetched: 10,
		TotalClassified: 8,
		FailedCount:  2,
		Messages: []MessageEntry{
			{
				Subject: "Test",
				From:    "a@b.com",
				Date:    time.Now(),
				IsRead:  true,
				Classification: mail.Classification{
					Key:        mail.MessageKey{AccountLabel: "test", UID: 1},
					Label:      "Useful",
					Confidence: 0.9,
					Reason:     "Test",
				},
				Excerpt: "Test",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "10 fetched") {
		t.Error("result missing fetched count")
	}
	if !strings.Contains(result, "2 failed") {
		t.Error("result missing failed count")
	}
}

func TestFallbackRenderer_MessageCount(t *testing.T) {
	now := time.Now()
	r := NewFallbackRenderer(config.DigestConfig{
		IncludeReadStatus:  true,
		MaxMessageExcerpt:  200,
	})

	data := DigestData{
		RunID:        "fallback-count",
		GeneratedAt:  now,
		TotalFetched: 5,
		FailedCount:  3,
		Messages: []MessageEntry{
			{
				Subject: "Test",
				From:    "a@b.com",
				Date:    time.Now(),
				IsRead:  true,
				Excerpt: "Test",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "(fallback mode)") {
		t.Error("result missing fallback mode indicator")
	}
}