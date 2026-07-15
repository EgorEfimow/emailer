// Package digest provides renderers that transform classification results into
// human-readable digest formats. The primary renderer is Markdown, used for
// Telegram delivery. A FallbackRenderer is used when the LLM classification
// step fails, ensuring the user still receives a run summary.
package digest

import (
	"context"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// MessageEntry
// ---------------------------------------------------------------------------

// MessageEntry represents a single classified message in the digest.
type MessageEntry struct {
	Subject        string
	From           string
	Date           time.Time
	IsRead         bool
	Classification mail.Classification
	Excerpt        string
}

// ---------------------------------------------------------------------------
// Stats
// ---------------------------------------------------------------------------

// DigestStats captures aggregate counts for a digest run.
type DigestStats struct {
	FetchedCount    int
	ClassifiedCount int
	FailedCount     int
	ReadCount       int
	UnreadCount     int
	CountsByLabel   map[string]int

	// AccountsChecked is the total number of accounts considered this run.
	AccountsChecked int
	// AccountsSucceeded is the number of accounts that fetched successfully.
	AccountsSucceeded int
	// AccountsFailed is the number of accounts whose fetch failed.
	AccountsFailed int
	// HighPriorityCount is the number of high-priority classifications.
	HighPriorityCount int
	// TopSenders lists the most frequent senders (format: "addr (count)").
	TopSenders []string
	// TopDomains lists the most frequent domains (format: "domain (count)").
	TopDomains []string
}

// AccountStats captures aggregate counts and fetch status for one account.
type AccountStats struct {
	AccountLabel    string
	FetchedCount    int
	ClassifiedCount int
	FailedCount     int
	ReadCount       int
	UnreadCount     int
	CountsByLabel   map[string]int
	Status          string
	Error           string
	// TopSenders lists the most frequent senders for this account.
	TopSenders []string
	// TopDomains lists the most frequent domains for this account.
	TopDomains []string
}

// ---------------------------------------------------------------------------
// DigestData
// ---------------------------------------------------------------------------

// DigestData is the input data for a digest renderer.
type DigestData struct {
	RunID           string
	GeneratedAt     time.Time
	AccountLabel    string
	Messages        []MessageEntry
	TotalFetched    int
	TotalClassified int
	FailedCount     int
	GlobalStats     DigestStats
	AccountStats    []AccountStats

	// Highlights is a short, deterministic list of notable observations for
	// this run (e.g. "3 high-priority emails", "1 account failed", "Ads up
	// by 5 vs last run"). The render order is preserved exactly as supplied.
	// An empty slice means "no notable highlights this run"; the renderer is
	// responsible for showing a neutral message in that case.
	Highlights []string
}

// ---------------------------------------------------------------------------
// Renderer
// ---------------------------------------------------------------------------

// Renderer produces a human-readable digest string from classification data.
//
// The output is expected to be plain text or Markdown suitable for delivery
// via a notification channel such as Telegram.
type Renderer interface {
	// Render produces a digest string from the given data.
	Render(ctx context.Context, data DigestData) (string, error)

	// Name returns a human-readable name for this renderer (e.g. "markdown").
	Name() string
}
