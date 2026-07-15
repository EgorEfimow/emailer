package llm

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

// ---------------------------------------------------------------------------
// Valid response parsing
// ---------------------------------------------------------------------------

func TestParseResponse_Valid(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [
		{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.95, "reason": "Important meeting", "summary": "Email summary", "key_points": ["Key point"]},
		{"uid": 2, "account": "personal", "label": "ToDelete", "confidence": 0.8, "reason": "Spam", "summary": "Email summary", "key_points": ["Key point"]}
	]}`

	result, err := ParseResponse(raw, []string{"Useful", "ToDelete", "Ads"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 2 {
		t.Fatalf("got %d classifications, want 2", len(result.Classifications))
	}

	// Check first result.
	if result.Classifications[0].Key.AccountLabel != "work" {
		t.Errorf("first account = %q, want %q", result.Classifications[0].Key.AccountLabel, "work")
	}
	if result.Classifications[0].Key.UID != 1 {
		t.Errorf("first uid = %d, want 1", result.Classifications[0].Key.UID)
	}
	if result.Classifications[0].Label != "Useful" {
		t.Errorf("first label = %q, want %q", result.Classifications[0].Label, "Useful")
	}
	if result.Classifications[0].Confidence != 0.95 {
		t.Errorf("first confidence = %f, want 0.95", result.Classifications[0].Confidence)
	}
	if result.Classifications[0].Reason != "Important meeting" {
		t.Errorf("first reason = %q, want %q", result.Classifications[0].Reason, "Important meeting")
	}
	if result.Classifications[0].Summary != "Email summary" {
		t.Errorf("first summary = %q, want %q", result.Classifications[0].Summary, "Email summary")
	}
	if len(result.Classifications[0].KeyPoints) != 1 || result.Classifications[0].KeyPoints[0] != "Key point" {
		t.Errorf("first key_points = %v, want [Key point]", result.Classifications[0].KeyPoints)
	}

	// Check second result.
	if result.Classifications[1].Key.AccountLabel != "personal" {
		t.Errorf("second account = %q, want %q", result.Classifications[1].Key.AccountLabel, "personal")
	}
	if result.Classifications[1].Key.UID != 2 {
		t.Errorf("second uid = %d, want 2", result.Classifications[1].Key.UID)
	}
	if result.Classifications[1].Label != "ToDelete" {
		t.Errorf("second label = %q, want %q", result.Classifications[1].Label, "ToDelete")
	}
}

func TestParseResponse_WithFences(t *testing.T) {
	// Response with markdown code fences (```json ... ```).
	raw := "```json\n{\"schema_version\": 1, \"classifications\": [\n  {\"uid\": 1, \"account\": \"work\", \"label\": \"Useful\", \"confidence\": 0.9, \"reason\": \"test\", \"summary\": \"Email summary\", \"key_points\": [\"Key point\"]}\n]}\n```"

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.Classifications[0].Label != "Useful" {
		t.Errorf("label = %q, want %q", result.Classifications[0].Label, "Useful")
	}
}

func TestParseResponse_WithFencesNoLang(t *testing.T) {
	// Response with plain fences (``` ... ```).
	raw := "```\n{\"schema_version\": 1, \"classifications\": [\n  {\"uid\": 1, \"account\": \"work\", \"label\": \"Useful\", \"confidence\": 0.9, \"reason\": \"test\", \"summary\": \"Email summary\", \"key_points\": [\"Key point\"]}\n]}\n```"

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
}

func TestParseResponse_SingleItem(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "test", "label": "Ads", "confidence": 0.75, "reason": "promotional", "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(raw, []string{"Ads"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.Classifications[0].Label != "Ads" {
		t.Errorf("label = %q, want %q", result.Classifications[0].Label, "Ads")
	}
}

func TestParseResponse_UnknownLabelBecomesUnknown(t *testing.T) {
	// "Unknown" is always a valid label.
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Unknown", "confidence": 0.5, "reason": "cannot classify", "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.Classifications[0].Label != "Unknown" {
		t.Errorf("label = %q, want %q", result.Classifications[0].Label, "Unknown")
	}
}

func TestParseResponse_TrailingContent(t *testing.T) {
	// Response with trailing commentary after the JSON.
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]} I think this email is important.`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
}

func TestParseResponse_OptionalAnalysisFields(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "reply needed", "summary": "Needs a reply", "key_points": ["Customer asks for an update"], "action_items": ["Reply with status"], "urgency": "high"}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.Classifications[0].ActionItems; len(got) != 1 || got[0] != "Reply with status" {
		t.Errorf("action_items = %v, want [Reply with status]", got)
	}
	if result.Classifications[0].Urgency != "high" {
		t.Errorf("urgency = %q, want high", result.Classifications[0].Urgency)
	}
	if result.Classifications[0].Priority != "high" {
		t.Errorf("priority = %q, want high", result.Classifications[0].Priority)
	}
}

func TestParseResponse_MissingSummary(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "key_points": ["Key point"]}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for missing summary, got nil")
	}
	if !strings.Contains(err.Error(), "summary") {
		t.Errorf("error should mention summary, got: %v", err)
	}
}

func TestParseResponse_MissingKeyPoints(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary"}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for missing key_points, got nil")
	}
	if !strings.Contains(err.Error(), "key_points") {
		t.Errorf("error should mention key_points, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Rejection cases
// ---------------------------------------------------------------------------

func TestParseResponse_EmptyResponse(t *testing.T) {
	_, err := ParseResponse("", []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for empty response, got nil")
	}
}

func TestParseResponse_WhitespaceOnly(t *testing.T) {
	_, err := ParseResponse("   \n\t  ", []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for whitespace-only response, got nil")
	}
}

func TestParseResponse_InvalidJSON(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [broken json]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseResponse_EmptyClassificationsArray(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": []}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for empty classifications array, got nil")
	}
}

func TestParseResponse_UnknownLabel(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "InvalidLabel", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]}`

	_, err := ParseResponse(raw, []string{"Useful", "ToDelete", "Ads"})
	if err == nil {
		t.Fatal("expected error for unknown label, got nil")
	}
	if !strings.Contains(err.Error(), "InvalidLabel") {
		t.Errorf("error should mention the invalid label, got: %v", err)
	}
}

func TestParseResponse_DuplicateKeys(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [
		{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "first", "summary": "Email summary", "key_points": ["Key point"]},
		{"uid": 1, "account": "work", "label": "ToDelete", "confidence": 0.8, "reason": "second", "summary": "Email summary", "key_points": ["Key point"]}
	]}`

	_, err := ParseResponse(raw, []string{"Useful", "ToDelete"})
	if err == nil {
		t.Fatal("expected error for duplicate keys, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate key") {
		t.Errorf("error should mention duplicate key, got: %v", err)
	}
}

func TestParseResponse_ConfidenceOutOfRange(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 1.5, "reason": "too confident", "summary": "Email summary", "key_points": ["Key point"]}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for out-of-range confidence, got nil")
	}
	if !strings.Contains(err.Error(), "confidence") {
		t.Errorf("error should mention confidence, got: %v", err)
	}
}

func TestParseResponse_NegativeConfidence(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": -0.5, "reason": "negative", "summary": "Email summary", "key_points": ["Key point"]}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for negative confidence, got nil")
	}
}

func TestParseResponse_MissingFields(t *testing.T) {
	// Missing 'reason' field — should still parse (it's optional in Go).
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
}

func TestParseResponse_PartialInvalid(t *testing.T) {
	// One valid, one invalid — should return partial results with error.
	raw := `{"schema_version": 1, "classifications": [
		{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "good", "summary": "Email summary", "key_points": ["Key point"]},
		{"uid": 2, "account": "work", "label": "Nope", "confidence": 0.5, "reason": "bad", "summary": "Email summary", "key_points": ["Key point"]}
	]}`

	result, err := ParseResponse(raw, []string{"Useful", "ToDelete"})
	if err == nil {
		t.Fatal("expected error for partial invalid, got nil")
	}
	if len(result.Classifications) != 1 {
		t.Fatalf("got %d valid classifications, want 1", len(result.Classifications))
	}
	if result.Classifications[0].Label != "Useful" {
		t.Errorf("valid classification label = %q, want %q", result.Classifications[0].Label, "Useful")
	}
}

// ---------------------------------------------------------------------------
// RepairWithPrompt tests
// ---------------------------------------------------------------------------

func TestRepairWithPrompt_ValidInput(t *testing.T) {
	raw := `{"classifications": [broken]}`

	parseErr := errors.New("parse: invalid JSON: invalid character 'b' looking for beginning of value")

	prompt, err := RepairWithPrompt(raw, parseErr, []string{"Useful", "ToDelete", "Ads"})
	if err != nil {
		t.Fatalf("RepairWithPrompt failed: %v", err)
	}

	// Check the prompt contains the original response.
	if !strings.Contains(prompt, raw) {
		t.Error("repair prompt should contain the original response")
	}

	// Check the prompt contains the parse error.
	if !strings.Contains(prompt, parseErr.Error()) {
		t.Error("repair prompt should contain the parse error")
	}

	// Check the prompt contains valid labels.
	if !strings.Contains(prompt, "- Useful") {
		t.Error("repair prompt should contain valid labels")
	}

	// Check the prompt contains the expected JSON format.
	if !strings.Contains(prompt, `"uid":`) {
		t.Error("repair prompt should contain the expected JSON format")
	}

	// Check repair prompt includes schema_version.
	if !strings.Contains(prompt, `"schema_version": 1`) {
		t.Error("repair prompt should contain schema_version in example JSON")
	}
	if !strings.Contains(prompt, "schema_version must be 1") {
		t.Error("repair prompt should mention schema_version rule")
	}

	// Check repair rules are present.
	if !strings.Contains(prompt, "No duplicate") {
		t.Error("repair prompt should mention no-duplicate rule")
	}
	if !strings.Contains(prompt, "0.0") {
		t.Error("repair prompt should mention confidence range")
	}
}

func TestRepairWithPrompt_EmptyRaw(t *testing.T) {
	_, err := RepairWithPrompt("", errors.New("some error"), nil)
	if err == nil {
		t.Fatal("expected error for empty raw, got nil")
	}
}

func TestRepairWithPrompt_NilError(t *testing.T) {
	_, err := RepairWithPrompt("some raw", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil parseErr, got nil")
	}
}

func TestRepairWithPrompt_CustomLabels(t *testing.T) {
	raw := `{"classifications": []}`
	parseErr := errors.New("empty classifications array")

	prompt, err := RepairWithPrompt(raw, parseErr, []string{"Important", "Spam"})
	if err != nil {
		t.Fatalf("RepairWithPrompt failed: %v", err)
	}

	if !strings.Contains(prompt, "- Important") {
		t.Error("repair prompt should contain Important label")
	}
	if !strings.Contains(prompt, "- Spam") {
		t.Error("repair prompt should contain Spam label")
	}
}

// ---------------------------------------------------------------------------
// stripFences tests
// ---------------------------------------------------------------------------

func TestStripFences_LangPrefixed(t *testing.T) {
	input := "```json\n{\"key\": \"value\"}\n```"
	got := stripFences(input)
	if got != `{"key": "value"}` {
		t.Errorf("got %q, want %q", got, `{"key": "value"}`)
	}
}

func TestStripFences_Plain(t *testing.T) {
	input := "```\n{\"key\": \"value\"}\n```"
	got := stripFences(input)
	if got != `{"key": "value"}` {
		t.Errorf("got %q, want %q", got, `{"key": "value"}`)
	}
}

func TestStripFences_NoFences(t *testing.T) {
	input := `{"key": "value"}`
	got := stripFences(input)
	if got != `{"key": "value"}` {
		t.Errorf("got %q, want %q", got, `{"key": "value"}`)
	}
}

func TestStripFences_WhitespaceAround(t *testing.T) {
	input := "  \n```json\n{\"key\": \"value\"}\n```\n  "
	got := stripFences(input)
	if got != `{"key": "value"}` {
		t.Errorf("got %q, want %q", got, `{"key": "value"}`)
	}
}

// ---------------------------------------------------------------------------
// SanitizeResponse tests
// ---------------------------------------------------------------------------

func TestSanitizeResponse_NoTrailingContent(t *testing.T) {
	input := `{"key": "value"}`
	got := SanitizeResponse(input)
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestSanitizeResponse_WithTrailingContent(t *testing.T) {
	input := `{"key": "value"} and some extra text`
	got := SanitizeResponse(input)
	if got != `{"key": "value"}` {
		t.Errorf("got %q, want %q", got, `{"key": "value"}`)
	}
}

func TestSanitizeResponse_NoBraces(t *testing.T) {
	input := `just text`
	got := SanitizeResponse(input)
	if got != input {
		t.Errorf("got %q, want %q", got, input)
	}
}

func TestSanitizeResponse_EmptyString(t *testing.T) {
	got := SanitizeResponse("")
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// ValidateResponse tests
// ---------------------------------------------------------------------------

func TestValidateResponse_Valid(t *testing.T) {
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "work", UID: 1}, Label: "Useful", Confidence: 0.9},
	}

	err := ValidateResponse(classifications, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateResponse_Empty(t *testing.T) {
	err := ValidateResponse(nil, 0)
	if err == nil {
		t.Fatal("expected error for empty classifications, got nil")
	}
}

func TestValidateResponse_CountMismatch(t *testing.T) {
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "work", UID: 1}, Label: "Useful", Confidence: 0.9},
	}

	err := ValidateResponse(classifications, 2)
	if err == nil {
		t.Fatal("expected error for count mismatch, got nil")
	}
}

func TestValidateResponse_MissingLabel(t *testing.T) {
	classifications := []mail.Classification{
		{Key: mail.MessageKey{AccountLabel: "work", UID: 1}, Confidence: 0.9},
	}

	err := ValidateResponse(classifications, 0)
	if err == nil {
		t.Fatal("expected error for empty label, got nil")
	}
}

// ---------------------------------------------------------------------------
// Schema version tests
// ---------------------------------------------------------------------------

func TestParseResponse_SchemaVersion1(t *testing.T) {
	// Valid response with schema_version: 1
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", result.SchemaVersion)
	}
}

func TestParseResponse_MissingSchemaVersion(t *testing.T) {
	// Missing schema_version (backward compatible - treated as version 1)
	raw := `{"classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	// Missing schema_version defaults to 0 (Go zero value), which we treat as v1
	if result.SchemaVersion != 0 {
		t.Errorf("SchemaVersion = %d, want 0 (missing defaults to 0)", result.SchemaVersion)
	}
}

func TestParseResponse_ExplicitSchemaVersion0(t *testing.T) {
	// Explicit schema_version: 0 (should be treated as version 1 for backward compat)
	raw := `{"schema_version": 0, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.SchemaVersion != 0 {
		t.Errorf("SchemaVersion = %d, want 0", result.SchemaVersion)
	}
}

func TestParseResponse_UnsupportedSchemaVersion(t *testing.T) {
	// Future version > current should be rejected
	raw := `{"schema_version": 99, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for unsupported schema version, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported schema_version") {
		t.Errorf("error should mention unsupported schema_version, got: %v", err)
	}
}

func TestParseResponse_NegativeSchemaVersion(t *testing.T) {
	// Negative version should be rejected
	raw := `{"schema_version": -1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"]}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for negative schema version, got nil")
	}
	if !strings.Contains(err.Error(), "invalid schema_version") {
		t.Errorf("error should mention invalid schema_version, got: %v", err)
	}
}

func TestParseResponse_SchemaVersionWithFences(t *testing.T) {
	// Schema version with markdown fences
	raw := "```json\n{\"schema_version\": 1, \"classifications\": [\n  {\"uid\": 1, \"account\": \"work\", \"label\": \"Useful\", \"confidence\": 0.9, \"reason\": \"test\", \"summary\": \"Email summary\", \"key_points\": [\"Key point\"]}\n]}\n```"

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", result.SchemaVersion)
	}
}

// ---------------------------------------------------------------------------
// Integration: round-trip from prompt to parse
// ---------------------------------------------------------------------------

func TestRoundTrip_PromptToParse(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	req := Request{
		Labels: []string{"Useful", "ToDelete", "Ads"},
		Messages: []InputMessage{
			{
				Key:     mail.MessageKey{AccountLabel: "work", UID: 1},
				Subject: "Team meeting",
				From:    "alice@example.com",
				Date:    now,
				Body:    "Please join the meeting tomorrow.",
				IsRead:  true,
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// Verify the prompt contains the expected structure.
	if !strings.Contains(prompt, "work/1") {
		t.Error("prompt missing message key")
	}
	if !strings.Contains(prompt, "Team meeting") {
		t.Error("prompt missing subject")
	}
	if !strings.Contains(prompt, "Status: Read") {
		t.Error("prompt missing read status")
	}

	// Simulate a valid LLM response.
	llmResponse := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.95, "reason": "Meeting invitation", "summary": "Email summary", "key_points": ["Key point"]}]}`

	result, err := ParseResponse(llmResponse, req.Labels)
	if err != nil {
		t.Fatalf("ParseResponse failed: %v", err)
	}

	if len(result.Classifications) != 1 {
		t.Fatalf("got %d classifications, want 1", len(result.Classifications))
	}
	if result.Classifications[0].Key != req.Messages[0].Key {
		t.Errorf("key mismatch: got %v, want %v", result.Classifications[0].Key, req.Messages[0].Key)
	}
	if result.Classifications[0].Label != "Useful" {
		t.Errorf("label = %q, want %q", result.Classifications[0].Label, "Useful")
	}
	if result.Classifications[0].Confidence != 0.95 {
		t.Errorf("confidence = %f, want 0.95", result.Classifications[0].Confidence)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestParseResponse_AllItemsInvalid(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [
		{"uid": 1, "account": "work", "label": "Invalid1", "confidence": 0.9, "reason": "bad", "summary": "Email summary", "key_points": ["Key point"]},
		{"uid": 2, "account": "work", "label": "Invalid2", "confidence": 0.5, "reason": "also bad", "summary": "Email summary", "key_points": ["Key point"]}
	]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error when all items are invalid")
	}
	if !strings.Contains(err.Error(), "all") {
		t.Errorf("error should indicate all items are invalid, got: %v", err)
	}
}

func TestParseResponse_ConfidenceAtBounds(t *testing.T) {
	// Test confidence at the exact bounds.
	raw := `{"schema_version": 1, "classifications": [
		{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.0, "reason": "zero", "summary": "Email summary", "key_points": ["Key point"]},
		{"uid": 2, "account": "work", "label": "Useful", "confidence": 1.0, "reason": "one", "summary": "Email summary", "key_points": ["Key point"]}
	]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 2 {
		t.Fatalf("got %d classifications, want 2", len(result.Classifications))
	}
	if result.Classifications[0].Confidence != 0.0 {
		t.Errorf("first confidence = %f, want 0.0", result.Classifications[0].Confidence)
	}
	if result.Classifications[1].Confidence != 1.0 {
		t.Errorf("second confidence = %f, want 1.0", result.Classifications[1].Confidence)
	}
}

func TestParseResponse_DifferentAccountsSameUID(t *testing.T) {
	// Same UID but different accounts — should be valid.
	raw := `{"schema_version": 1, "classifications": [
		{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "work", "summary": "Email summary", "key_points": ["Key point"]},
		{"uid": 1, "account": "personal", "label": "ToDelete", "confidence": 0.8, "reason": "personal", "summary": "Email summary", "key_points": ["Key point"]}
	]}`

	result, err := ParseResponse(raw, []string{"Useful", "ToDelete"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Classifications) != 2 {
		t.Fatalf("got %d classifications, want 2", len(result.Classifications))
	}
}

func TestParseResponse_PriorityValidation(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "deadline", "summary": "Due tomorrow", "key_points": ["Deadline tomorrow"], "priority": "HIGH"}]}`

	result, err := ParseResponse(raw, []string{"Useful"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Classifications[0].Priority != "high" {
		t.Errorf("priority = %q, want high", result.Classifications[0].Priority)
	}
}

func TestParseResponse_InvalidPriority(t *testing.T) {
	raw := `{"schema_version": 1, "classifications": [{"uid": 1, "account": "work", "label": "Useful", "confidence": 0.9, "reason": "test", "summary": "Email summary", "key_points": ["Key point"], "priority": "urgent"}]}`

	_, err := ParseResponse(raw, []string{"Useful"})
	if err == nil {
		t.Fatal("expected error for invalid priority, got nil")
	}
	if !strings.Contains(err.Error(), "invalid priority") {
		t.Errorf("error should mention invalid priority, got: %v", err)
	}
}
