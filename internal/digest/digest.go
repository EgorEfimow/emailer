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
