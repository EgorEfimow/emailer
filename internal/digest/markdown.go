package digest

import (
	"bytes"
	"context"
	"fmt"
	"sort"
	"strings"
	"text/template"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// MarkdownRenderer
// ---------------------------------------------------------------------------

// MarkdownRenderer renders digest data as Markdown using configurable
// templates. It explicitly renders the Date and Read/Unread status for
// each message.
type MarkdownRenderer struct {
	// IncludeReadStatus controls whether the read/unread badge is shown.
	IncludeReadStatus bool

	// MaxMessageExcerpt limits the excerpt length in characters.
	MaxMessageExcerpt int
}

// compile-time check: *MarkdownRenderer satisfies Renderer.
var _ Renderer = (*MarkdownRenderer)(nil)

// NewMarkdownRenderer creates a new MarkdownRenderer with the given options.
func NewMarkdownRenderer(includeReadStatus bool, maxMessageExcerpt int) *MarkdownRenderer {
	return &MarkdownRenderer{
		IncludeReadStatus: includeReadStatus,
		MaxMessageExcerpt: maxMessageExcerpt,
	}
}

// Name returns "markdown".
func (r *MarkdownRenderer) Name() string {
	return "markdown"
}

// Render produces a Markdown digest from the provided data.
func (r *MarkdownRenderer) Render(_ context.Context, data DigestData) (string, error) {
	tmpl, err := template.New("digest").
		Funcs(template.FuncMap{
			"formatTime":  formatTime,
			"readBadge":   r.readBadge,
			"truncate":    r.truncate,
			"joinLabels":  joinLabels,
			"labelCounts": labelCounts,
			"add1":        func(n int) int { return n + 1 },
			"mul":         func(a, b float64) float64 { return a * b },
			"priority":    displayPriority,
			"hasSummary":  func(c mail.Classification) bool { return strings.TrimSpace(c.Summary) != "" },
			"now":         time.Now,
		}).
		Parse(markdownTemplate)
	if err != nil {
		return "", fmt.Errorf("digest.markdown.parse_template: %w", err)
	}

	stats := r.prepareStats(data)

	// Collect high-priority messages for the "Needs attention" section.
	highPriority := collectHighPriorityMessages(data.Messages)

	// Group messages by classification label.
	groups := groupByLabel(data.Messages)
	// Sort groups alphabetically for consistent output.
	labels := make([]string, 0, len(groups))
	for l := range groups {
		labels = append(labels, l)
	}
	sort.Strings(labels)

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]any{
		"RunID":               data.RunID,
		"GeneratedAt":         data.GeneratedAt,
		"AccountLabel":        data.AccountLabel,
		"TotalFetched":        data.TotalFetched,
		"TotalClassified":     data.TotalClassified,
		"FailedCount":         data.FailedCount,
		"Groups":              groups,
		"Labels":              labels,
		"TotalMessages":       len(data.Messages),
		"GlobalStats":         stats,
		"AccountStats":        data.AccountStats,
		"IncludeReadStatus":   r.IncludeReadStatus,
		"HighPriority":        highPriority,
		"HasHighPriority":     len(highPriority) > 0,
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
	if !r.IncludeReadStatus {
		return ""
	}
	if isRead {
		return "✅ Read"
	}
	return "🆕 Unread"
}

// truncate shortens a string to the configured maximum length.
func (r *MarkdownRenderer) truncate(s string) string {
	if r.MaxMessageExcerpt <= 0 || len(s) <= r.MaxMessageExcerpt {
		return s
	}
	return s[:r.MaxMessageExcerpt] + "…"
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
**Messages:** {{.TotalMessages}} classified ({{.TotalFetched}} fetched, {{.FailedCount}} failed)

## Account Stats

{{- if .AccountStats}}
{{- range .AccountStats}}
### {{.AccountLabel}}

**Fetched:** {{.FetchedCount}} | **Classified:** {{.ClassifiedCount}} | **Failed:** {{.FailedCount}}
{{- if $.IncludeReadStatus}}
**Read:** {{.ReadCount}} | **Unread:** {{.UnreadCount}}
{{- end}}
**Labels:**{{range labelCounts .CountsByLabel}} {{.Label}}={{.Count}}{{end}}
{{- if .Error}}
⚠️ **Fetch error:** {{.Error}}
{{- end}}

{{- end}}
{{- else}}
No account stats available.
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

{{- if hasSummary $entry.Classification}}
**Summary:** {{$entry.Classification.Summary}}

**Key points:**
{{- range $entry.Classification.KeyPoints}}
- {{.}}
{{- end}}
{{- if $entry.Classification.ActionItems}}

**Action items:**
{{- range $entry.Classification.ActionItems}}
- {{.}}
{{- end}}
{{- end}}
{{- else}}
> {{truncate $entry.Excerpt}}
{{- end}}

{{- end}}
{{- end}}

---

*Generated by Email AI Agent*
`
