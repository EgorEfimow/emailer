package digest

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// MarkdownRenderer
// ---------------------------------------------------------------------------

// MarkdownRenderer renders digest data as Markdown using configurable
// templates. It explicitly renders the Date and Read/Unread status for
// each message.
type MarkdownRenderer struct {
	cfg config.DigestConfig
}

// compile-time check: *MarkdownRenderer satisfies Renderer.
var _ Renderer = (*MarkdownRenderer)(nil)

// NewMarkdownRenderer creates a new MarkdownRenderer with the given options.
func NewMarkdownRenderer(cfg config.DigestConfig) *MarkdownRenderer {
	return &MarkdownRenderer{cfg: cfg}
}

// Name returns "markdown".
func (r *MarkdownRenderer) Name() string {
	return "markdown"
}

// Render produces a Markdown digest from the provided data.
func (r *MarkdownRenderer) Render(_ context.Context, data DigestData) (string, error) {
	// Apply priority-only filter first.
	messages := data.Messages
	if r.cfg.PriorityOnly {
		var filtered []MessageEntry
		for _, m := range messages {
			if strings.EqualFold(strings.TrimSpace(m.Classification.Priority), "high") {
				filtered = append(filtered, m)
			}
		}
		messages = filtered
	}

	// Apply max_messages limit (0 = no limit).
	if r.cfg.MaxMessages > 0 && len(messages) > r.cfg.MaxMessages {
		// Prefer high-priority, then most recent.
		sort.SliceStable(messages, func(i, j int) bool {
			pi := priorityRank(messages[i].Classification.Priority)
			pj := priorityRank(messages[j].Classification.Priority)
			if pi != pj {
				return pi < pj
			}
			return messages[i].Date.After(messages[j].Date)
		})
		messages = messages[:r.cfg.MaxMessages]
	}

	// Also apply priority-only and max_messages to high-priority list for the "Needs Attention" section.
	highPriority := collectHighPriorityMessages(messages)

	// Group messages by classification label.
	groups := groupByLabel(messages)
	// Sort groups alphabetically for consistent output.
	labels := make([]string, 0, len(groups))
	for l := range groups {
		labels = append(labels, l)
	}
	sort.Strings(labels)

	stats := r.prepareStats(data)

	tmpl, err := template.New("digest").
		Funcs(template.FuncMap{
			"formatTime":          formatTime,
			"readBadge":           r.readBadge,
			"truncate":            r.truncate,
			"truncateKeyPoints":   r.truncateKeyPoints,
			"truncateActionItems": r.truncateActionItems,
			"joinLabels":          joinLabels,
			"labelCounts":         labelCounts,
			"add1":                func(n int) int { return n + 1 },
			"mul":                 func(a, b float64) float64 { return a * b },
			"priority":            displayPriority,
			"hasSummary":          func(c mail.Classification) bool { return strings.TrimSpace(c.Summary) != "" },
			"hasAnalysisError":    func(c mail.Classification) bool { return c.AnalysisError != nil },
			"includeGlobalStats":  func() bool { return r.cfg.IncludeGlobalStats },
			"includeAccountStats": func() bool { return r.cfg.IncludeAccountStats },
			"includeSummaries":    func() bool { return r.cfg.IncludeSummaries },
			"includeKeyPoints":    func() bool { return r.cfg.IncludeKeyPoints },
			"includeActionItems":  func() bool { return r.cfg.IncludeActionItems },
			"includeRawFallback":  func() bool { return r.cfg.IncludeRawExcerptFallback },
			"now":                 time.Now,
		}).
		Parse(markdownTemplate)
	if err != nil {
		return "", fmt.Errorf("digest.markdown.parse_template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"RunID":             data.RunID,
		"GeneratedAt":       data.GeneratedAt,
		"AccountLabel":      data.AccountLabel,
		"TotalFetched":      data.TotalFetched,
		"TotalClassified":   data.TotalClassified,
		"FailedCount":       data.FailedCount,
		"Groups":            groups,
		"Labels":            labels,
		"TotalMessages":     len(messages),
		"GlobalStats":       stats,
		"AccountStats":      data.AccountStats,
		"IncludeReadStatus": r.cfg.IncludeReadStatus,
		"HighPriority":      highPriority,
		"HasHighPriority":   len(highPriority) > 0,
		"Highlights":        data.Highlights,
		"HasHighlights":     data.Highlights != nil,
	}); err != nil {
		return "", fmt.Errorf("digest.markdown.execute: %w", err)
	}

	return buf.String(), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// prepareStats derives the global stats to render, filling in counts from the
// DigestData totals when the provided stats are not fully populated (e.g. when
// produced by the fallback renderer). It also computes label and read/unread
// counts from the message list when CountsByLabel is absent.
func (r *MarkdownRenderer) prepareStats(data DigestData) DigestStats {
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

// collectHighPriorityMessages returns the subset of entries whose classification
// priority is "high" (case-insensitive), used for the "Needs attention" section.
func collectHighPriorityMessages(messages []MessageEntry) []MessageEntry {
	var high []MessageEntry
	for _, m := range messages {
		if strings.EqualFold(strings.TrimSpace(m.Classification.Priority), "high") {
			high = append(high, m)
		}
	}
	return high
}

// readBadge returns a short string indicating the read/unread status.
func (r *MarkdownRenderer) readBadge(isRead bool) string {
	if !r.cfg.IncludeReadStatus {
		return ""
	}
	if isRead {
		return "✅ Read"
	}
	return "🆕 Unread"
}

// truncate shortens a string to the configured maximum length.
func (r *MarkdownRenderer) truncate(s string) string {
	if r.cfg.MaxMessageExcerpt <= 0 || len(s) <= r.cfg.MaxMessageExcerpt {
		return s
	}
	return s[:r.cfg.MaxMessageExcerpt] + "…"
}

// truncateKeyPoints truncates the key points slice to the configured maximum.
func (r *MarkdownRenderer) truncateKeyPoints(points []string) []string {
	if r.cfg.MaxKeyPointsPerMessage <= 0 || len(points) <= r.cfg.MaxKeyPointsPerMessage {
		return points
	}
	return points[:r.cfg.MaxKeyPointsPerMessage]
}

// truncateActionItems truncates the action items slice to the configured maximum.
func (r *MarkdownRenderer) truncateActionItems(items []string) []string {
	if r.cfg.MaxActionItemsPerMessage <= 0 || len(items) <= r.cfg.MaxActionItemsPerMessage {
		return items
	}
	return items[:r.cfg.MaxActionItemsPerMessage]
}

// formatTime formats a time.Time for display in the digest.
func formatTime(t time.Time) string {
	return t.Format("2006-01-02 15:04")
}

// joinLabels joins a list of strings with a comma separator.
func joinLabels(labels []string) string {
	return strings.Join(labels, ", ")
}

// labelCount is a stable display row for a label count map.
type labelCount struct {
	Label string
	Count int
}

// labelCounts returns label counts sorted by label for deterministic rendering.
func labelCounts(counts map[string]int) []labelCount {
	labels := make([]string, 0, len(counts))
	for label := range counts {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	out := make([]labelCount, 0, len(labels))
	for _, label := range labels {
		out = append(out, labelCount{Label: label, Count: counts[label]})
	}
	return out
}

// groupByLabel groups message entries by their classification label.
func groupByLabel(entries []MessageEntry) map[string][]MessageEntry {
	groups := make(map[string][]MessageEntry)
	for _, e := range entries {
		label := e.Classification.Label
		if label == "" {
			label = "Unknown"
		}
		groups[label] = append(groups[label], e)
	}
	for label := range groups {
		sort.SliceStable(groups[label], func(i, j int) bool {
			left := priorityRank(groups[label][i].Classification.Priority)
			right := priorityRank(groups[label][j].Classification.Priority)
			if left != right {
				return left < right
			}
			return groups[label][i].Date.After(groups[label][j].Date)
		})
	}
	return groups
}

func priorityRank(priority string) int {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

func displayPriority(priority string) string {
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "high":
		return "🔴 High"
	case "medium":
		return "🟡 Medium"
	case "low":
		return "🟢 Low"
	default:
		return "Unspecified"
	}
}

// ---------------------------------------------------------------------------
// Template
// ---------------------------------------------------------------------------

// markdownTemplate is the default Markdown template for the digest.
// It renders Date and Read/Unread status explicitly.
const markdownTemplate = `# 📧 Email Digest

**Run ID:** {{.RunID}}
**Generated:** {{formatTime .GeneratedAt}}
{{- if .AccountLabel}}
**Account:** {{.AccountLabel}}
{{- end}}
{{- if .HasHighlights}}
## Highlights

{{- if .Highlights}}
{{- range .Highlights}}
- {{.}}
{{- end}}
{{- else}}
— Nothing notable this run.
{{- end}}

{{- end}}
{{- if includeGlobalStats}}
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
**Labels:**{{range labelCounts .GlobalStats.CountsByLabel}} {{.Label}}={{.Count}}{{end}}
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
**Messages:** {{.TotalMessages}} classified ({{.TotalFetched}} fetched, {{.FailedCount}} failed)

{{- end}}
{{- if includeAccountStats}}
## Account Stats

{{- if .AccountStats}}
{{- range .AccountStats}}
### {{.AccountLabel}}

**Fetched:** {{.FetchedCount}} | **Classified:** {{.ClassifiedCount}} | **Failed:** {{.FailedCount}}
{{- if $.IncludeReadStatus}}
**Read:** {{.ReadCount}} | **Unread:** {{.UnreadCount}}
{{- end}}
**Labels:**{{range labelCounts .CountsByLabel}} {{.Label}}={{.Count}}{{end}}
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
{{- else}}
No account stats available.
{{- end}}

{{- end}}
{{- if $.HasHighPriority}}
## 🚨 Needs Attention

{{- range $i, $entry := $.HighPriority}}
### {{$i | add1}}. {{$entry.Subject}}

**From:** {{$entry.From}} | **Date:** {{formatTime $entry.Date}}
{{- if $.IncludeReadStatus}}
**Status:** {{readBadge $entry.IsRead}}
{{- end}}
**Priority:** {{priority $entry.Classification.Priority}}
**Reason:** {{$entry.Classification.Reason}}

{{- end}}
---

{{- end}}

{{- range $label := .Labels}}
{{- $entries := index $.Groups $label}}

## {{$label}} ({{len $entries}})

{{- range $i, $entry := $entries}}
### {{$i | add1}}. {{$entry.Subject}}

**From:** {{$entry.From}} | **Date:** {{formatTime $entry.Date}}
{{- if $.IncludeReadStatus}}
**Status:** {{readBadge $entry.IsRead}}
{{- end}}
**Priority:** {{priority $entry.Classification.Priority}}
**Confidence:** {{printf "%.0f" (mul $entry.Classification.Confidence 100)}}%
**Reason:** {{$entry.Classification.Reason}}

{{- if includeSummaries}}
{{- if hasSummary $entry.Classification}}
**Summary:** {{$entry.Classification.Summary}}

**Key points:**
{{- range truncateKeyPoints $entry.Classification.KeyPoints}}
- {{.}}
{{- end}}
{{- if includeActionItems}}
{{- if $entry.Classification.ActionItems}}

**Action items:**
{{- range truncateActionItems $entry.Classification.ActionItems}}
- {{.}}
{{- end}}
{{- end}}
{{- end}}
{{- else if hasAnalysisError $entry.Classification}}
⚠️ **Analysis failed ({{$entry.Classification.AnalysisError.Stage}}):** {{$entry.Classification.AnalysisError.Error}}

{{- if includeRawFallback}}
> {{truncate $entry.Excerpt}}
{{- else}}
> [Raw excerpt omitted — enable include_raw_excerpt_fallback to view]
{{- end}}
{{- else}}
{{- if includeRawFallback}}
> {{truncate $entry.Excerpt}}
{{- else}}
> [Raw excerpt omitted — enable include_raw_excerpt_fallback or include_summaries to view]
{{- end}}
{{- end}}
{{- end}}

{{- end}}
{{- end}}

---

*Generated by Email AI Agent*
`