package llm

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
	"time"
)

// ---------------------------------------------------------------------------
// Prompt template
// ---------------------------------------------------------------------------

// defaultPromptTemplate is the built-in classification prompt. It wraps each
// email in unique delimiters, includes metadata (Date, Read/Unread status),
// and isolates the classification instruction.
const defaultPromptTemplate = `You are an email classifier. Your task is to classify each email below into one of the following labels:

{{.Labels}}

For each email, output a JSON object with these fields:
  - "uid": the email's UID (number)
  - "account": the account label (string)
  - "label": the classification label from the list above (string)
  - "confidence": your confidence in this classification, 0.0 to 1.0 (float)
  - "reason": a short justification for the classification (string)
  - "summary": a concise summary of the email (string)
  - "key_points": important facts or details from the email (array of strings)
  - "action_items": follow-up tasks requested by the email, if any (optional array of strings)
  - "priority": one of "high", "medium", or "low" (string)

Assess priority using these signals:
  - high: explicit deadlines or due dates soon, payment or security risk, direct requests to the recipient, calendar/time-sensitive commitments, or important sender context.
  - medium: useful business or personal updates that may need review but have no immediate deadline or risk.
  - low: informational, promotional, automated, or non-actionable messages with little time sensitivity.

{{range $i, $msg := .Messages}}
<<< EMAIL {{$msg.Key.AccountLabel}}/{{$msg.Key.UID}} >>>
From: {{$msg.From}}
Subject: {{$msg.Subject}}
Date: {{$msg.Date.Format "Mon, 02 Jan 2006 15:04:05 -0700"}}
Status: {{if $msg.IsRead}}Read{{else}}Unread{{end}}

Body:
{{$msg.Body}}
<<< END EMAIL {{$msg.Key.AccountLabel}}/{{$msg.Key.UID}} >>>
{{end}}

Output ONLY valid JSON in this exact format (no markdown fences, no extra text):

{"schema_version": 1, "classifications": [
  {"uid": ..., "account": "...", "label": "...", "confidence": 0.0, "reason": "...", "summary": "...", "key_points": ["..."], "action_items": ["..."], "priority": "medium"}
]}

If you cannot classify an email, use label "Unknown" with confidence 0.0.`

// ---------------------------------------------------------------------------
// promptData
// ---------------------------------------------------------------------------

// promptData is the data structure passed to the text/template.
type promptData struct {
	Labels   string
	Messages []InputMessage
}

// BuildPrompt generates a classification prompt from the given request.
// The prompt wraps each email in unique delimiters, includes metadata
// (Date, Read/Unread status), and embeds the classification schema.
func BuildPrompt(req Request) (string, error) {
	labels := formatLabels(req.Labels)

	data := promptData{
		Labels:   labels,
		Messages: req.Messages,
	}

	tmpl := defaultPromptTemplate
	if req.ClassificationPrompt != "" {
		tmpl = req.ClassificationPrompt
	}

	// Parse the template. We use text/template, not html/template, because
	// the output is a plain-text prompt, not HTML. The template is fixed
	// (not user-supplied), so template injection is not a concern at the
	// template level — values are rendered as strings, not interpreted.
	t, err := template.New("classification").Option("missingkey=error").Parse(tmpl)
	if err != nil {
		return "", fmt.Errorf("prompt.parse_template: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("prompt.execute: %w", err)
	}

	return buf.String(), nil
}

// formatLabels converts a label slice into a formatted list string.
// If no labels are provided, the defaults are used.
func formatLabels(labels []string) string {
	if len(labels) == 0 {
		labels = []string{"Useful", "ToDelete", "Ads", "Unknown"}
	}
	parts := make([]string, len(labels))
	for i, l := range labels {
		parts[i] = "- " + l
	}
	return strings.Join(parts, "\n")
}

// ---------------------------------------------------------------------------
// BuildPromptFromMessages is a convenience wrapper that creates a Request
// from a slice of messages and a system prompt, then calls BuildPrompt.
// ---------------------------------------------------------------------------

// BuildPromptFromMessages builds a classification prompt directly from a
// list of emails. This is a convenience for the pipeline orchestrator.
func BuildPromptFromMessages(systemPrompt string, classificationPrompt string, labels []string, messages []InputMessage) (string, error) {
	req := Request{
		SystemPrompt:         systemPrompt,
		ClassificationPrompt: classificationPrompt,
		Labels:               labels,
		Messages:             messages,
	}
	return BuildPrompt(req)
}

// ---------------------------------------------------------------------------
// FormatDate is a helper for formatting dates in prompts.
// ---------------------------------------------------------------------------

// FormatDate formats a time.Time as an RFC 2822 date string.
func FormatDate(t time.Time) string {
	return t.Format("Mon, 02 Jan 2006 15:04:05 -0700")
}

// ---------------------------------------------------------------------------
// DefaultLabelsList returns the default classification labels as a string.
// ---------------------------------------------------------------------------

func DefaultLabelsList() string {
	return strings.Join([]string{"Useful", "ToDelete", "Ads", "Unknown"}, ", ")
}

// ---------------------------------------------------------------------------
// SanitizeBody ensures the email body is safe for inclusion in the prompt.
// It strips NUL bytes and normalizes whitespace.
// ---------------------------------------------------------------------------

// SanitizeBody prepares an email body for inclusion in a prompt by
// stripping control characters (except newlines and tabs) and trimming
// whitespace.
func SanitizeBody(body string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\t' || r == '\r' {
			return r
		}
		if r < 32 {
			return -1 // drop
		}
		return r
	}, strings.TrimSpace(body))
}
