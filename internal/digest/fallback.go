package digest

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/egorefimow/emailer/internal/config"
)

// ---------------------------------------------------------------------------
// FallbackRenderer
// ---------------------------------------------------------------------------

// FallbackRenderer produces a simplified digest when the LLM classification
// step fails. It still reports the run metadata and a list of fetched
// messages, but without classification labels or confidence scores.
type FallbackRenderer struct {
	// Cfg holds the digest configuration.
	Cfg config.DigestConfig
}

// compile-time check: *FallbackRenderer satisfies Renderer.
var _ Renderer = (*FallbackRenderer)(nil)

// NewFallbackRenderer creates a new FallbackRenderer with the given config.
func NewFallbackRenderer(cfg config.DigestConfig) *FallbackRenderer {
	return &FallbackRenderer{Cfg: cfg}
}

// Name returns "fallback".
func (r *FallbackRenderer) Name() string {
	return "fallback"
}

// Render produces a fallback digest when LLM classification is unavailable.
func (r *FallbackRenderer) Render(_ context.Context, data DigestData) (string, error) {
	// Apply priority_only and max_messages filtering before rendering.
	messages := data.Messages
	if r.Cfg.PriorityOnly {
		messages = filterHighPriority(messages)
	}
	if r.Cfg.MaxMessages > 0 && len(messages) > r.Cfg.MaxMessages {
		messages = messages[:r.Cfg.MaxMessages]
	}

	tmpl, err := template.New("fallback").
		Funcs(template.FuncMap{
			"formatTime":   formatTime,
			"readBadge":    r.readBadge,
			"truncate":     r.truncate,
			"add1":         func(n int) int { return n + 1 },
			"now":          time.Now,
			"includeStats": func() bool { return r.Cfg.IncludeGlobalStats || r.Cfg.IncludeAccountStats },
		}).
		Parse(fallbackTemplate)
	if err != nil {
		return "", fmt.Errorf("digest.fallback.parse_template: %w", err)
	}

	stats := r.prepareStats(data)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"RunID":               data.RunID,
		"GeneratedAt":         data.GeneratedAt,
		"AccountLabel":        data.AccountLabel,
		"TotalFetched":        data.TotalFetched,
		"Messages":            messages,
		"TotalMessages":       len(messages),
		"IncludeReadStatus":   r.Cfg.IncludeReadStatus,
		"IncludeGlobalStats":  r.Cfg.IncludeGlobalStats,
		"IncludeAccountStats": r.Cfg.IncludeAccountStats,
		"GlobalStats":         stats,
		"AccountStats":        data.AccountStats,
	}); err != nil {
		return "", fmt.Errorf("digest.fallback.execute: %w", err)
	}

	return buf.String(), nil
}

// prepareStats derives global stats for the fallback template.
func (r *FallbackRenderer) prepareStats(data DigestData) DigestStats {
	stats := data.GlobalStats
	if stats.FetchedCount == 0 && data.TotalFetched > 0 {
		stats.FetchedCount = data.TotalFetched
	}
	if stats.ClassifiedCount == 0 && data.TotalClassified > 0 {
		stats.ClassifiedCount = data.TotalClassified
	}
	if stats.FailedCount == 0 && data.FailedCount > 0 {
		stats.FailedCount = data.FailedCount
	}
	if stats.CountsByLabel == nil {
		stats.CountsByLabel = make(map[string]int)
		for _, msg := range data.Messages {
			label := msg.Classification.Label
			if label == "" {
				label = "Unknown"
			}
			stats.CountsByLabel[label]++
			if msg.IsRead {
				stats.ReadCount++
			} else {
				stats.UnreadCount++
			}
		}
	}
	return stats
}

// filterHighPriority returns only messages with high priority.
func filterHighPriority(messages []MessageEntry) []MessageEntry {
	var high []MessageEntry
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Classification.Priority), "high") {
			high = append(high, m)
		}
	}
	return high
}

// readBadge returns a short string indicating the read/unread status.
func (r *FallbackRenderer) readBadge(isRead bool) string {
	if !r.Cfg.IncludeReadStatus {
		return ""
	}
	if isRead {
		return "✅ Read"
	}
	return "🆕 Unread"
}

// truncate shortens a string to the configured maximum length.
func (r *FallbackRenderer) truncate(s string) string {
	if r.Cfg.MaxMessageExcerpt <= 0 || len(s) <= r.Cfg.MaxMessageExcerpt {
		return s
	}
	return s[:r.Cfg.MaxMessageExcerpt] + "…"
}

// ---------------------------------------------------------------------------
// Template
// ---------------------------------------------------------------------------

// fallbackTemplate is the template used when LLM classification failed.
// It lists all fetched messages without classification data.
const fallbackTemplate = `# ⚠️ Email Digest (Fallback)

**Run ID:** {{.RunID}}
**Generated:** {{formatTime .GeneratedAt}}
{{- if .AccountLabel}}
**Account:** {{.AccountLabel}}
{{- end}}
**Status:** ⚠️ LLM classification was unavailable — messages are listed without labels.

---
{{- if includeStats}}
## Summary

**Fetched:** {{.GlobalStats.FetchedCount}}
**Classified:** {{.GlobalStats.ClassifiedCount}}
**Failed:** {{.GlobalStats.FailedCount}}
**Accounts:** {{.GlobalStats.AccountsChecked}} checked, {{.GlobalStats.AccountsSucceeded}} succeeded, {{.GlobalStats.AccountsFailed}} failed
**High priority:** {{.GlobalStats.HighPriorityCount}}
{{- if $.IncludeReadStatus}}
**Read:** {{.GlobalStats.ReadCount}}
**Unread:** {{.GlobalStats.UnreadCount}}
{{- end}}
**Labels:**{{range $label, $count := .GlobalStats.CountsByLabel}} {{$label}}={{$count}}{{end}}
{{- if .GlobalStats.TopSenders}}
**Top senders:**
{{- range .GlobalStats.TopSenders}}
- {{.}}
{{- end}}
{{- end}}
{{- if .GlobalStats.TopDomains}}
**Noisiest domains:**
{{- range .GlobalStats.TopDomains}}
- {{.}}
{{- end}}
{{- end}}
{{- if .AccountStats}}
## Account Stats
{{- range .AccountStats}}
### {{.AccountLabel}}

**Fetched:** {{.FetchedCount}} | **Classified:** {{.ClassifiedCount}} | **Failed:** {{.FailedCount}}
{{- if $.IncludeReadStatus}}
**Read:** {{.ReadCount}} | **Unread:** {{.UnreadCount}}
{{- end}}
**Labels:**{{range $label, $count := .CountsByLabel}} {{$label}}={{$count}}{{end}}
{{- if .TopSenders}}
**Top senders:**
{{- range .TopSenders}}
- {{.}}
{{- end}}
{{- end}}
{{- if .TopDomains}}
**Noisiest domains:**
{{- range .TopDomains}}
- {{.}}
{{- end}}
{{- end}}
{{- if .Error}}
⚠️ **Fetch error:** {{.Error}}
{{- end}}

{{- end}}
{{- end}}
{{- else}}
No stats available.
{{- end}}

---

## Fetched Messages ({{.TotalMessages}})

{{- range $i, $entry := .Messages}}
### {{$i | add1}}. {{$entry.Subject}}

**From:** {{$entry.From}} | **Date:** {{formatTime $entry.Date}}
{{- if $.IncludeReadStatus}}
**Status:** {{readBadge $entry.IsRead}}
{{- end}}

> {{truncate $entry.Excerpt}}

{{- end}}

---

*Generated by Email AI Agent (fallback mode)*
`