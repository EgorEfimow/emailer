package llm

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// Schema version
// ---------------------------------------------------------------------------

// CurrentSchemaVersion is the LLM response schema version this code understands.
// Increment when the JSON structure changes in a backward-incompatible way.
const CurrentSchemaVersion = 1

// ---------------------------------------------------------------------------
// Classification schema
// ---------------------------------------------------------------------------

// ParseResult holds the result of parsing an LLM response.
type ParseResult struct {
	Classifications []mail.Classification
	SchemaVersion   int
}

// classificationsWrapper is the JSON structure returned by the LLM.
type classificationsWrapper struct {
	SchemaVersion   int                  `json:"schema_version"`
	Classifications []classificationItem `json:"classifications"`
}

// classificationItem is a single classification in the LLM response JSON.
type classificationItem struct {
	UID         uint32   `json:"uid"`
	Account     string   `json:"account"`
	Label       string   `json:"label"`
	Confidence  float64  `json:"confidence"`
	Reason      string   `json:"reason"`
	Summary     string   `json:"summary"`
	KeyPoints   []string `json:"key_points"`
	ActionItems []string `json:"action_items,omitempty"`
	Priority    string   `json:"priority,omitempty"`
	Urgency     string   `json:"urgency,omitempty"`
}

// ---------------------------------------------------------------------------
// ParseResponse
// ---------------------------------------------------------------------------

// ParseResponse strips markdown code fences from the LLM output, parses the
// JSON, and validates each classification against the configured labels.
//
// Returns the parsed classifications and schema version. If the response is
// empty or malformed, or if any classification is invalid, an error is
// returned with details. The caller can use the error to construct a repair
// prompt.
func ParseResponse(raw string, validLabels []string) (ParseResult, error) { //nolint:gocyclo
	if strings.TrimSpace(raw) == "" {
		return ParseResult{}, fmt.Errorf("parse: empty response")
	}

	// Strip markdown code fences and trailing content.
	cleaned := SanitizeResponse(stripFences(raw))

	// Parse the JSON wrapper.
	var wrapper classificationsWrapper
	if err := json.Unmarshal([]byte(cleaned), &wrapper); err != nil {
		return ParseResult{}, fmt.Errorf("parse: invalid JSON: %w", err)
	}

	// Validate schema version.
	if wrapper.SchemaVersion < 0 {
		return ParseResult{}, fmt.Errorf("parse: invalid schema_version %d (must be >= 0)", wrapper.SchemaVersion)
	}
	if wrapper.SchemaVersion > CurrentSchemaVersion {
		return ParseResult{}, fmt.Errorf("parse: unsupported schema_version %d (max supported: %d)", wrapper.SchemaVersion, CurrentSchemaVersion)
	}
	// Missing schema_version (0) is treated as version 1 for backward compatibility.

	if len(wrapper.Classifications) == 0 {
		return ParseResult{}, fmt.Errorf("parse: empty classifications array")
	}

	// Build the valid label set.
	labelSet := make(map[string]bool, len(validLabels))
	for _, l := range validLabels {
		labelSet[l] = true
	}
	// Always include Unknown as a fallback label.
	labelSet["Unknown"] = true

	// Track seen keys for duplicate detection.
	seen := make(map[mail.MessageKey]bool)

	results := make([]mail.Classification, 0, len(wrapper.Classifications))
	var errs []string

	for i, item := range wrapper.Classifications {
		key := mail.MessageKey{AccountLabel: item.Account, UID: item.UID}

		// Check for duplicate key.
		if seen[key] {
			errs = append(errs, fmt.Sprintf("item %d: duplicate key %s/%d", i, item.Account, item.UID))
			continue
		}
		seen[key] = true

		// Validate label.
		if !labelSet[item.Label] {
			errs = append(errs, fmt.Sprintf("item %d: unknown label %q for %s/%d", i, item.Label, item.Account, item.UID))
			continue
		}

		// Validate confidence range.
		if item.Confidence < 0.0 || item.Confidence > 1.0 {
			errs = append(errs, fmt.Sprintf("item %d: confidence %f out of range [0,1] for %s/%d", i, item.Confidence, item.Account, item.UID))
			continue
		}

		priority := normalizePriority(item.Priority)
		if priority == "" && strings.TrimSpace(item.Urgency) != "" {
			priority = normalizeLegacyUrgency(item.Urgency)
		}
		if priority != "" && !validPriority(priority) {
			errs = append(errs, fmt.Sprintf("item %d: invalid priority %q for %s/%d", i, firstNonEmpty(item.Priority, item.Urgency), item.Account, item.UID))
			continue
		}

		// Validate required analysis fields.
		if strings.TrimSpace(item.Summary) == "" {
			errs = append(errs, fmt.Sprintf("item %d: empty summary for %s/%d", i, item.Account, item.UID))
			continue
		}
		if len(item.KeyPoints) == 0 {
			errs = append(errs, fmt.Sprintf("item %d: empty key_points for %s/%d", i, item.Account, item.UID))
			continue
		}
		invalidKeyPoint := false
		for j, point := range item.KeyPoints {
			if strings.TrimSpace(point) == "" {
				errs = append(errs, fmt.Sprintf("item %d: empty key_points[%d] for %s/%d", i, j, item.Account, item.UID))
				invalidKeyPoint = true
			}
		}
		if invalidKeyPoint {
			continue
		}

		results = append(results, mail.Classification{
			Key:         key,
			Label:       item.Label,
			Confidence:  item.Confidence,
			Reason:      item.Reason,
			Summary:     item.Summary,
			KeyPoints:   item.KeyPoints,
			ActionItems: item.ActionItems,
			Priority:    priority,
			Urgency:     item.Urgency,
		})
	}

	if len(errs) > 0 {
		// Return partial results along with the error for repair.
		if len(results) > 0 {
			return ParseResult{
				Classifications: results,
				SchemaVersion:   wrapper.SchemaVersion,
			}, fmt.Errorf("parse: %d/%d items invalid: %s", len(errs), len(wrapper.Classifications), strings.Join(errs, "; "))
		}
		return ParseResult{}, fmt.Errorf("parse: all %d items invalid: %s", len(wrapper.Classifications), strings.Join(errs, "; "))
	}

	return ParseResult{
		Classifications: results,
		SchemaVersion:   wrapper.SchemaVersion,
	}, nil
}

// ---------------------------------------------------------------------------
// stripFences
// ---------------------------------------------------------------------------

// stripFences removes markdown code fences (```json ... ``` or ``` ... ```)
// from the beginning and end of the response. If no fences are detected,
// the input is returned as-is.
func validPriority(priority string) bool {
	switch priority {
	case "high", "medium", "low":
		return true
	default:
		return false
	}
}

func normalizePriority(priority string) string {
	return strings.ToLower(strings.TrimSpace(priority))
}

func normalizeLegacyUrgency(urgency string) string {
	normalized := normalizePriority(urgency)
	if normalized == "normal" {
		return "medium"
	}
	return normalized
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func stripFences(s string) string {
	s = strings.TrimSpace(s)

	// Remove ```json prefix.
	if strings.HasPrefix(s, "```json") {
		s = strings.TrimPrefix(s, "```json")
	} else if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```")
	}

	// Remove trailing ```.
	if idx := strings.LastIndex(s, "```"); idx >= 0 {
		s = s[:idx]
	}

	return strings.TrimSpace(s)
}

// ---------------------------------------------------------------------------
// RepairWithPrompt
// ---------------------------------------------------------------------------

// RepairWithPrompt constructs a repair prompt that asks the LLM to fix a
// malformed classification response.
//
// The original raw response and the parse error are embedded in the prompt
// so the LLM can see what went wrong and produce corrected output.
//
// Returns the repair prompt text. The caller should send this to the LLM
// and call ParseResponse on the result.
func RepairWithPrompt(raw string, parseErr error, validLabels []string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", fmt.Errorf("repair: empty raw response")
	}
	if parseErr == nil {
		return "", fmt.Errorf("repair: parseErr is nil — nothing to repair")
	}

	labels := formatLabels(validLabels)

	var b strings.Builder
	b.WriteString("The following JSON response from an email classifier is invalid:\n\n")
	b.WriteString("```\n")
	b.WriteString(raw)
	b.WriteString("\n```\n\n")
	fmt.Fprintf(&b, "Parse error: %s\n\n", parseErr.Error())
	b.WriteString("Please fix the JSON and output ONLY valid JSON in this exact format (no markdown fences, no extra text):\n\n")
	fmt.Fprintf(&b, `{"schema_version": %d, "classifications": [`, CurrentSchemaVersion)
	b.WriteString("\n  ")
	b.WriteString(`{"uid": ..., "account": "...", "label": "...", "confidence": 0.0, "reason": "...", "summary": "...", "key_points": ["..."], "action_items": ["..."], "priority": "medium"}`)
	b.WriteString("\n]}\n\n")
	b.WriteString("Valid labels:\n")
	b.WriteString(labels)
	b.WriteString("\n\nRules:\n")
	b.WriteString("- Each item must have: uid (number), account (string), label (string from the list above), confidence (0.0–1.0), reason (string), summary (non-empty string), key_points (non-empty array of strings)\n")
	b.WriteString("- No duplicate (account, uid) pairs\n")
	b.WriteString("- Confidence must be between 0.0 and 1.0\n")
	b.WriteString("- action_items are optional\n")
	b.WriteString("- priority is optional when unavailable; when present it must be one of: high, medium, low\n")
	b.WriteString("- schema_version must be ")
	fmt.Fprintf(&b, "%d", CurrentSchemaVersion)
	b.WriteString("\n")
	b.WriteString("- Output ONLY the JSON object, nothing else\n")

	return b.String(), nil
}

// ---------------------------------------------------------------------------
// ValidateResponse
// ---------------------------------------------------------------------------

// ValidateResponse performs additional validation on the parsed
// classifications beyond what ParseResponse checks. This is useful
// for provider-specific validation.
func ValidateResponse(classifications []mail.Classification, expectedCount int) error {
	if len(classifications) == 0 {
		return fmt.Errorf("validate: no classifications")
	}

	if expectedCount > 0 && len(classifications) != expectedCount {
		return fmt.Errorf("validate: expected %d classifications, got %d", expectedCount, len(classifications))
	}

	var errs []string
	for i, c := range classifications {
		if c.Label == "" {
			errs = append(errs, fmt.Sprintf("item %d: empty label for %s/%d", i, c.Key.AccountLabel, c.Key.UID))
		}
		if c.Confidence < 0.0 || c.Confidence > 1.0 {
			errs = append(errs, fmt.Sprintf("item %d: confidence %f out of range", i, c.Confidence))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("validate: %s", strings.Join(errs, "; "))
	}

	return nil
}

// ---------------------------------------------------------------------------
// SanitizeResponse
// ---------------------------------------------------------------------------

// SanitizeResponse strips any content that appears after the closing JSON
// brace. Some LLMs append commentary after the JSON output. This helper
// extracts only the JSON portion.
func SanitizeResponse(raw string) string {
	raw = strings.TrimSpace(raw)

	// Find the first '{' and last '}'.
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")

	if start == -1 || end == -1 || start >= end {
		return raw
	}

	return raw[start : end+1]
}
