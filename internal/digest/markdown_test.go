package digest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

func TestMarkdownRenderer_HappyPath(t *testing.T) { //nolint:gocyclo
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
	if adsPos >= toDeletePos || toDeletePos >= usefulPos {
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

func TestMarkdownRenderer_NeedsAttentionSection(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-needs-attn",
		GeneratedAt: now,
		TotalFetched: 3,
		TotalClassified: 3,
		Messages: []MessageEntry{
			{
				Subject:        "Urgent security update",
				From:           "security@example.com",
				Date:           now,
				IsRead:         false,
				Classification: mail.Classification{Label: "Useful", Confidence: 0.98, Reason: "Security vulnerability", Priority: "high"},
				Excerpt:        "Please patch immediately.",
			},
			{
				Subject:        "Low priority newsletter",
				From:           "news@example.com",
				Date:           now.Add(time.Hour),
				IsRead:         true,
				Classification: mail.Classification{Label: "Ads", Confidence: 0.7, Reason: "Weekly newsletter", Priority: "low"},
				Excerpt:        "This week in tech.",
			},
			{
				Subject:        "Medium priority review",
				From:           "pm@example.com",
				Date:           now.Add(2*time.Hour),
				IsRead:         true,
				Classification: mail.Classification{Label: "Useful", Confidence: 0.85, Reason: "Needs review", Priority: "medium"},
				Excerpt:        "Please review the design doc.",
			},
		},
		GlobalStats: DigestStats{
			FetchedCount:     3,
			ClassifiedCount:  3,
			ReadCount:        2,
			UnreadCount:      1,
			HighPriorityCount: 1,
			CountsByLabel: map[string]int{
				"Ads":    1,
				"Useful": 2,
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Verify the Needs Attention section is present.
	if !strings.Contains(result, "Needs Attention") {
		t.Fatalf("result missing 'Needs Attention' section:\n%s", result)
	}

	// High-priority message should appear in the Needs Attention section first,
	// then again in its label group later.
	highPos := strings.Index(result, "Urgent security update")
	needsPos := strings.Index(result, "Needs Attention")
	usefulPos := strings.Index(result, "## Useful")

	if highPos == -1 || needsPos == -1 || usefulPos == -1 {
		t.Fatalf("result missing expected sections:\n%s", result)
	}

	// High-priority message should appear before the Useful section (it's in Needs Attention).
	if highPos >= usefulPos {
		t.Fatalf("high-priority item should appear before label groups:\n%s", result)
	}

	// Verify high-priority count in stats.
	if !strings.Contains(result, "**High priority:** 1") {
		t.Fatalf("result missing high priority count:\n%s", result)
	}

	// Verify the message detail in the Needs Attention section.
	if !strings.Contains(result, "🔴 High") {
		t.Fatalf("result missing high priority badge:\n%s", result)
	}

	// Verify low and medium messages are NOT in the Needs Attention section.
	needsSectionEnd := strings.Index(result, "---")
	if needsSectionEnd > 0 {
		section := result[needsPos:needsSectionEnd]
		if strings.Contains(section, "Low priority newsletter") {
			t.Fatal("low priority message should not appear in Needs Attention section")
		}
		if strings.Contains(section, "Medium priority review") {
			t.Fatal("medium priority message should not appear in Needs Attention section")
		}
	}
}

func TestMarkdownRenderer_NoNeedsAttentionWhenNoHighPriority(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-no-high",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject:        "Medium priority",
				From:           "medium@example.com",
				Date:           now,
				Classification: mail.Classification{Label: "Useful", Confidence: 0.8, Reason: "Medium", Priority: "medium"},
				Excerpt:        "Medium priority content.",
			},
			{
				Subject:        "Low priority",
				From:           "low@example.com",
				Date:           now,
				Classification: mail.Classification{Label: "Ads", Confidence: 0.7, Reason: "Low", Priority: "low"},
				Excerpt:        "Low priority content.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if strings.Contains(result, "Needs Attention") {
		t.Fatal("Needs Attention section should not appear when no high-priority messages")
	}
}

func TestMarkdownRenderer_SortsHighPriorityFirstWithinLabel(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-priority",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject:        "Low priority update",
				From:           "news@example.com",
				Date:           now.Add(2 * time.Hour),
				Classification: mail.Classification{Label: "Useful", Confidence: 0.8, Reason: "Informational", Priority: "low"},
				Excerpt:        "Weekly update.",
			},
			{
				Subject:        "Payment security deadline",
				From:           "billing@example.com",
				Date:           now,
				Classification: mail.Classification{Label: "Useful", Confidence: 0.95, Reason: "Payment risk", Priority: "high"},
				Excerpt:        "Verify payment details by today.",
			},
			{
				Subject:        "Medium priority review",
				From:           "pm@example.com",
				Date:           now.Add(time.Hour),
				Classification: mail.Classification{Label: "Useful", Confidence: 0.9, Reason: "Needs review", Priority: "medium"},
				Excerpt:        "Please review when available.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	highPos := strings.Index(result, "Payment security deadline")
	mediumPos := strings.Index(result, "Medium priority review")
	lowPos := strings.Index(result, "Low priority update")
	if highPos == -1 || mediumPos == -1 || lowPos == -1 {
		t.Fatalf("result missing expected subjects:\n%s", result)
	}
	if highPos >= mediumPos || mediumPos >= lowPos {
		t.Fatalf("messages not sorted by priority:\n%s", result)
	}
	if !strings.Contains(result, "**Priority:** 🔴 High") {
		t.Fatalf("result missing high priority badge:\n%s", result)
	}
}

func TestMarkdownRenderer_RendersSummary(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-summary",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Project Update",
				From:    "alice@example.com",
				Date:    now,
				IsRead:  true,
				Classification: mail.Classification{
					Label:      "Useful",
					Confidence: 0.95,
					Reason:     "Important project update",
					Summary:    "The project is on track for the July release.",
					KeyPoints:  []string{"Release scheduled for July", "All blockers resolved"},
				},
				Excerpt: "The project is on track for the July release. All blockers have been resolved and the team is confident.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Verify summary is rendered.
	if !strings.Contains(result, "**Summary:** The project is on track for the July release.") {
		t.Fatalf("result missing summary:\n%s", result)
	}

	// Verify key points are rendered as bullet list.
	if !strings.Contains(result, "- Release scheduled for July") {
		t.Fatalf("result missing key point 1:\n%s", result)
	}
	if !strings.Contains(result, "- All blockers resolved") {
		t.Fatalf("result missing key point 2:\n%s", result)
	}

	// Verify raw excerpt is NOT rendered (fallback only when no summary).
	if strings.Contains(result, "> The project is on track for the July release. All blockers have been resolved") {
		t.Fatalf("raw excerpt should not be rendered when summary is present:\n%s", result)
	}
}

func TestMarkdownRenderer_RendersActionItems(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-action",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Action Required",
				From:    "boss@example.com",
				Date:    now,
				IsRead:  false,
				Classification: mail.Classification{
					Label:       "Useful",
					Confidence:  0.9,
					Reason:      "Needs follow-up",
					Summary:     "Please review the budget and send the report.",
					KeyPoints:   []string{"Budget review pending"},
					ActionItems: []string{"Review Q3 budget", "Send report by Friday"},
				},
				Excerpt: "Please review the budget and send the report.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Verify action items section is rendered.
	if !strings.Contains(result, "**Action items:**") {
		t.Fatalf("result missing action items section:\n%s", result)
	}
	if !strings.Contains(result, "- Review Q3 budget") {
		t.Fatalf("result missing action item 1:\n%s", result)
	}
	if !strings.Contains(result, "- Send report by Friday") {
		t.Fatalf("result missing action item 2:\n%s", result)
	}
}

func TestMarkdownRenderer_NoActionItemsWhenEmpty(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-no-action",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Informational",
				From:    "news@example.com",
				Date:    now,
				IsRead:  true,
				Classification: mail.Classification{
					Label:       "Useful",
					Confidence:  0.8,
					Reason:      "FYI",
					Summary:     "Weekly newsletter with no action needed.",
					KeyPoints:   []string{"New feature announced"},
					ActionItems: nil, // empty
				},
				Excerpt: "Weekly newsletter with no action needed.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Action items section should NOT be present.
	if strings.Contains(result, "**Action items:**") {
		t.Fatalf("action items section should not appear when list is empty:\n%s", result)
	}

	// Summary should still be present.
	if !strings.Contains(result, "**Summary:** Weekly newsletter with no action needed.") {
		t.Fatalf("result missing summary:\n%s", result)
	}
}

func TestMarkdownRenderer_FallbackToExcerptWhenNoSummary(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-fallback",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "No Summary Available",
				From:    "system@example.com",
				Date:    now,
				IsRead:  true,
				Classification: mail.Classification{
					Label:      "Useful",
					Confidence: 0.7,
					Reason:     "Could not summarize",
					// Summary intentionally empty
				},
				Excerpt: "This is the raw excerpt used as fallback when summary generation fails.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Verify excerpt fallback is rendered.
	if !strings.Contains(result, "> This is the raw excerpt used as fallback when summary generation fails.") {
		t.Fatalf("result missing excerpt fallback:\n%s", result)
	}

	// Verify no summary/key points sections are rendered.
	if strings.Contains(result, "**Summary:**") {
		t.Fatalf("summary section should not appear when summary is empty:\n%s", result)
	}
	if strings.Contains(result, "**Key points:**") {
		t.Fatalf("key points section should not appear when summary is empty:\n%s", result)
	}
}

func TestMarkdownRenderer_RendersTopSendersAndDomainsInGlobalStats(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:           "run-senders",
		GeneratedAt:     now,
		TotalFetched:    3,
		TotalClassified: 3,
		GlobalStats: DigestStats{
			FetchedCount:    3,
			ClassifiedCount: 3,
			CountsByLabel:   map[string]int{"Useful": 3},
			TopSenders:      []string{"alice@example.com (2)", "bob@example.com (1)"},
			TopDomains:      []string{"example.com (3)"},
		},
		Messages: []MessageEntry{
			{Subject: "A", From: "alice@example.com", Date: now, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}, IsRead: true},
			{Subject: "B", From: "alice@example.com", Date: now, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}, IsRead: true},
			{Subject: "C", From: "bob@example.com", Date: now, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}, IsRead: true},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "**Top senders:**") {
		t.Fatalf("result missing 'Top senders' section:\n%s", result)
	}
	if !strings.Contains(result, "- alice@example.com (2)") {
		t.Fatalf("result missing top sender entry:\n%s", result)
	}
	if !strings.Contains(result, "- bob@example.com (1)") {
		t.Fatalf("result missing second top sender entry:\n%s", result)
	}
	if !strings.Contains(result, "**Noisiest domains:**") {
		t.Fatalf("result missing 'Noisiest domains' section:\n%s", result)
	}
	if !strings.Contains(result, "- example.com (3)") {
		t.Fatalf("result missing top domain entry:\n%s", result)
	}
}

func TestMarkdownRenderer_OmitsTopSendersWhenEmpty(t *testing.T) {
	now := time.Now()
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-no-senders",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{Subject: "A", From: "", Date: now, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if strings.Contains(result, "**Top senders:**") {
		t.Fatal("'Top senders' should not appear when empty")
	}
	if strings.Contains(result, "**Noisiest domains:**") {
		t.Fatal("'Noisiest domains' should not appear when empty")
	}
}

func TestMarkdownRenderer_RendersTopSendersAndDomainsInAccountStats(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:           "run-acct-senders",
		GeneratedAt:     now,
		TotalFetched:    2,
		TotalClassified: 2,
		GlobalStats: DigestStats{
			FetchedCount: 2, ClassifiedCount: 2,
			CountsByLabel: map[string]int{"Useful": 2},
			TopSenders:    []string{"alice@example.com (2)"},
			TopDomains:    []string{"example.com (2)"},
		},
		AccountStats: []AccountStats{
			{
				AccountLabel:    "work",
				FetchedCount:    2,
				ClassifiedCount: 2,
				CountsByLabel:   map[string]int{"Useful": 2},
				TopSenders:      []string{"alice@example.com (2)"},
				TopDomains:      []string{"example.com (2)"},
			},
		},
		Messages: []MessageEntry{
			{Subject: "A", From: "alice@example.com", Date: now, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}, IsRead: true},
			{Subject: "B", From: "alice@example.com", Date: now, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}, IsRead: true},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	if !strings.Contains(result, "### work") {
		t.Fatalf("result missing account section:\n%s", result)
	}
	// Both global and per-account top senders appear; verify there are at
	// least two occurrences (global + account).
	count := strings.Count(result, "**Top senders:**")
	if count < 2 {
		t.Fatalf("expected at least 2 'Top senders' sections (global + account), got %d:\n%s", count, result)
	}
	// Verify the account section has its own top sender entries.
	acctSection := result[strings.Index(result, "### work"):]
	if !strings.Contains(acctSection, "**Top senders:**") {
		t.Fatalf("account section missing 'Top senders':\n%s", acctSection)
	}
	if !strings.Contains(acctSection, "- alice@example.com (2)") {
		t.Fatalf("account section missing top sender entry:\n%s", acctSection)
	}
	if !strings.Contains(acctSection, "**Noisiest domains:**") {
		t.Fatalf("account section missing 'Noisiest domains':\n%s", acctSection)
	}
	if !strings.Contains(acctSection, "- example.com (2)") {
		t.Fatalf("account section missing top domain entry:\n%s", acctSection)
	}
}

func TestMarkdownRenderer_MixedContent(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-mixed",
		GeneratedAt: now,
		Messages: []MessageEntry{
			{
				Subject: "Has Summary",
				From:    "alice@example.com",
				Date:    now,
				IsRead:  true,
				Classification: mail.Classification{
					Label:      "Useful",
					Confidence: 0.95,
					Reason:     "Good summary",
					Summary:    "Well summarized email.",
					KeyPoints:  []string{"Point A", "Point B"},
				},
				Excerpt: "Well summarized email with full details.",
			},
			{
				Subject: "No Summary",
				From:    "bob@example.com",
				Date:    now.Add(time.Hour),
				IsRead:  false,
				Classification: mail.Classification{
					Label:      "Ads",
					Confidence: 0.8,
					Reason:     "No summary available",
				},
				Excerpt: "This message has no summary and should use the excerpt fallback.",
			},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// First message: summary rendered, excerpt NOT rendered.
	if !strings.Contains(result, "**Summary:** Well summarized email.") {
		t.Fatalf("result missing summary for first message:\n%s", result)
	}
	if strings.Contains(result, "> Well summarized email with full details.") {
		t.Fatalf("excerpt should not render when summary present:\n%s", result)
	}

	// Second message: excerpt fallback rendered, summary NOT rendered.
	if !strings.Contains(result, "> This message has no summary and should use the excerpt fallback.") {
		t.Fatalf("result missing excerpt fallback for second message:\n%s", result)
	}

	// Count occurrences of "**Summary:**" — should appear exactly once
	// (for the "Has Summary" message only, not "No Summary").
	summaryCount := strings.Count(result, "**Summary:**")
	if summaryCount != 1 {
		t.Fatalf("expected exactly 1 summary section, got %d:\n%s", summaryCount, result)
	}
}

// ---------------------------------------------------------------------------
// Tests: Highlights rendering (Phase 8)
// ---------------------------------------------------------------------------

func TestMarkdownRenderer_HighlightsSection(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-highlights",
		GeneratedAt: now,
		Highlights:  []string{"3 high-priority emails require attention", "Account \"work\" failed: connection refused"},
		Messages: []MessageEntry{
			{Subject: "Test", From: "a@b.com", Date: now, IsRead: true, Classification: mail.Classification{Label: "Useful", Confidence: 0.9, Reason: "Test", Priority: "high"}},
		},
		GlobalStats: DigestStats{
			FetchedCount: 1, ClassifiedCount: 1,
			CountsByLabel: map[string]int{"Useful": 1},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Verify highlights section appears near the top
	if !strings.Contains(result, "## Highlights") {
		t.Fatalf("result missing 'Highlights' section:\n%s", result)
	}

	// Verify highlights are rendered
	if !strings.Contains(result, "3 high-priority emails require attention") {
		t.Fatalf("result missing first highlight:\n%s", result)
	}
	if !strings.Contains(result, "Account \"work\" failed: connection refused") {
		t.Fatalf("result missing second highlight:\n%s", result)
	}

	// Verify order: Highlights before Summary
	highlightsPos := strings.Index(result, "## Highlights")
	summaryPos := strings.Index(result, "## Summary")
	if highlightsPos == -1 || summaryPos == -1 || highlightsPos >= summaryPos {
		t.Fatalf("Highlights section should appear before Summary:\n%s", result)
	}
}

func TestMarkdownRenderer_NoHighlightsWhenNil(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-no-highlights",
		GeneratedAt: now,
		Highlights:  nil, // nil = don't render section at all
		Messages: []MessageEntry{
			{Subject: "Test", From: "a@b.com", Date: now, IsRead: true, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}},
		},
		GlobalStats: DigestStats{
			FetchedCount: 1, ClassifiedCount: 1,
			CountsByLabel: map[string]int{"Useful": 1},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Highlights section should NOT appear when nil
	if strings.Contains(result, "## Highlights") {
		t.Fatalf("Highlights section should not appear when Highlights is nil:\n%s", result)
	}
}

func TestMarkdownRenderer_NeutralHighlightsWhenEmpty(t *testing.T) {
	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	r := NewMarkdownRenderer(true, 200)

	data := DigestData{
		RunID:       "run-empty-highlights",
		GeneratedAt: now,
		Highlights:  []string{}, // empty slice = render section with neutral message
		Messages: []MessageEntry{
			{Subject: "Test", From: "a@b.com", Date: now, IsRead: true, Classification: mail.Classification{Label: "Useful", Confidence: 0.9}},
		},
		GlobalStats: DigestStats{
			FetchedCount: 1, ClassifiedCount: 1,
			CountsByLabel: map[string]int{"Useful": 1},
		},
	}

	result, err := r.Render(context.Background(), data)
	if err != nil {
		t.Fatalf("Render() returned error: %v", err)
	}

	// Highlights section SHOULD appear (empty slice != nil)
	if !strings.Contains(result, "## Highlights") {
		t.Fatalf("Highlights section should appear for empty slice:\n%s", result)
	}

	// Should show neutral message
	if !strings.Contains(result, "Nothing notable this run") {
		t.Fatalf("expected neutral message for empty highlights:\n%s", result)
	}
}
