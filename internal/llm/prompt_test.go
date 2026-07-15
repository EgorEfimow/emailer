package llm

import (
	"strings"
	"testing"
	"time"

	"github.com/egorefimow/emailer/internal/mail"
)

func TestBuildPrompt_Structure(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)

	req := Request{
		Model: "test-model",
		Messages: []InputMessage{
			{
				Key:     mail.MessageKey{AccountLabel: "work", UID: 1},
				Subject: "Hello",
				From:    "alice@example.com",
				Date:    now,
				Body:    "This is the email body.",
				IsRead:  true,
			},
			{
				Key:     mail.MessageKey{AccountLabel: "personal", UID: 2},
				Subject: "Party",
				From:    "bob@example.com",
				Date:    now.Add(-1 * time.Hour),
				Body:    "See you there!",
				IsRead:  false,
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// Check delimiters are present.
	if !strings.Contains(prompt, "<<< EMAIL work/1 >>>") {
		t.Error("missing delimiter for work/1")
	}
	if !strings.Contains(prompt, "<<< END EMAIL work/1 >>>") {
		t.Error("missing end delimiter for work/1")
	}
	if !strings.Contains(prompt, "<<< EMAIL personal/2 >>>") {
		t.Error("missing delimiter for personal/2")
	}
	if !strings.Contains(prompt, "<<< END EMAIL personal/2 >>>") {
		t.Error("missing end delimiter for personal/2")
	}

	// Check metadata is present.
	if !strings.Contains(prompt, "From: alice@example.com") {
		t.Error("missing From metadata")
	}
	if !strings.Contains(prompt, "Subject: Hello") {
		t.Error("missing Subject metadata")
	}
	if !strings.Contains(prompt, "Status: Read") {
		t.Error("missing Read status")
	}
	if !strings.Contains(prompt, "Status: Unread") {
		t.Error("missing Unread status")
	}

	// Check email body is present.
	if !strings.Contains(prompt, "This is the email body.") {
		t.Error("missing email body")
	}
	if !strings.Contains(prompt, "See you there!") {
		t.Error("missing second email body")
	}

	// Check JSON schema is present.
	if !strings.Contains(prompt, `"uid"`) {
		t.Error("missing uid field in schema")
	}
	if !strings.Contains(prompt, `"account"`) {
		t.Error("missing account field in schema")
	}
	if !strings.Contains(prompt, `"label"`) {
		t.Error("missing label field in schema")
	}
	if !strings.Contains(prompt, `"confidence"`) {
		t.Error("missing confidence field in schema")
	}
	if !strings.Contains(prompt, `"reason"`) {
		t.Error("missing reason field in schema")
	}

	if !strings.Contains(prompt, `"summary"`) {
		t.Error("missing summary field in schema")
	}
	if !strings.Contains(prompt, `"key_points"`) {
		t.Error("missing key_points field in schema")
	}
	if !strings.Contains(prompt, `"action_items"`) {
		t.Error("missing action_items field in schema")
	}
	if !strings.Contains(prompt, `"priority"`) {
		t.Error("missing priority field in schema")
	}
	if !strings.Contains(prompt, "payment or security risk") {
		t.Error("missing priority assessment guidance")
	}

	// Check labels are listed.
	if !strings.Contains(prompt, "- Useful") {
		t.Error("missing Useful label")
	}
	if !strings.Contains(prompt, "- ToDelete") {
		t.Error("missing ToDelete label")
	}
	if !strings.Contains(prompt, "- Ads") {
		t.Error("missing Ads label")
	}

	// Check date is formatted.
	if !strings.Contains(prompt, "Tue, 14 Jul 2026") {
		t.Errorf("expected formatted date, got: %s", prompt)
	}
}

func TestBuildPrompt_CustomLabels(t *testing.T) {
	req := Request{
		Labels: []string{"Important", "Spam", "Newsletter"},
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: "test body",
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	if !strings.Contains(prompt, "- Important") {
		t.Error("missing Important label")
	}
	if strings.Contains(prompt, "- Useful") {
		t.Error("default Useful label should not appear with custom labels")
	}
}

func TestBuildPrompt_CustomTemplate(t *testing.T) {
	customTmpl := "Custom template: {{.Labels}}\n\n{{range .Messages}}MSG: {{.Body}}\n{{end}}"

	req := Request{
		ClassificationPrompt: customTmpl,
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: "hello world",
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	if !strings.Contains(prompt, "Custom template") {
		t.Error("custom template prefix not found")
	}
	if !strings.Contains(prompt, "hello world") {
		t.Error("message body not found in custom template")
	}
	if !strings.Contains(prompt, "- Useful") {
		t.Error("labels not found in custom template")
	}
}

func TestBuildPrompt_EmptyMessages(t *testing.T) {
	req := Request{
		Messages: nil,
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// Should still have the classification instructions.
	if !strings.Contains(prompt, "email classifier") {
		t.Error("missing classification instructions")
	}
}

func TestBuildPrompt_NoTemplateInjection(t *testing.T) {
	// Email body contains text that looks like template syntax.
	// The template engine should output it literally.
	body := "Hello {{.World}} — this is not a template directive. {{if .x}}hidden{{end}}"
	req := Request{
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: body,
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// The body should appear literally in the output.
	if !strings.Contains(prompt, "{{.World}}") {
		t.Error("template-like syntax in body was altered or removed")
	}
	if !strings.Contains(prompt, "{{if .x}}hidden{{end}}") {
		t.Error("template-like control structure in body was altered or removed")
	}
}

func TestBuildPrompt_NewlinesInBody(t *testing.T) {
	body := "line1\nline2\nline3"
	req := Request{
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: body,
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	if !strings.Contains(prompt, "line1\nline2\nline3") {
		t.Error("newlines in body not preserved")
	}
}

func TestBuildPrompt_DateFormats(t *testing.T) {
	now := time.Date(2026, 12, 25, 9, 5, 30, 0, time.UTC)
	req := Request{
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: "test",
				Date: now,
			},
		},
	}

	prompt, err := BuildPrompt(req)
	if err != nil {
		t.Fatalf("BuildPrompt failed: %v", err)
	}

	// Check RFC 2822 format.
	if !strings.Contains(prompt, "Fri, 25 Dec 2026 09:05:30 +0000") {
		t.Errorf("date not in RFC 2822 format, got: %s", prompt)
	}
}

func TestSanitizeBody(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "normal text preserved",
			input: "Hello, World!",
			want:  "Hello, World!",
		},
		{
			name:  "newlines preserved",
			input: "line1\nline2\nline3",
			want:  "line1\nline2\nline3",
		},
		{
			name:  "tabs preserved",
			input: "col1\tcol2",
			want:  "col1\tcol2",
		},
		{
			name:  "control characters stripped",
			input: "Hello\x00World\x01\x02Test",
			want:  "HelloWorldTest",
		},
		{
			name:  "leading and trailing whitespace trimmed",
			input: "  hello  ",
			want:  "hello",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "only whitespace",
			input: "   \n\t  ",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeBody(tt.input)
			if got != tt.want {
				t.Errorf("SanitizeBody() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatDate(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	formatted := FormatDate(now)
	if !strings.Contains(formatted, "2026") {
		t.Errorf("expected 2026 in formatted date, got %q", formatted)
	}
}

func TestDefaultLabelsList(t *testing.T) {
	labels := DefaultLabelsList()
	if !strings.Contains(labels, "Useful") {
		t.Error("missing Useful in default labels")
	}
	if !strings.Contains(labels, "ToDelete") {
		t.Error("missing ToDelete in default labels")
	}
	if !strings.Contains(labels, "Ads") {
		t.Error("missing Ads in default labels")
	}
	if !strings.Contains(labels, "Unknown") {
		t.Error("missing Unknown in default labels")
	}
}

func TestBuildPromptFromMessages(t *testing.T) {
	now := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	messages := []InputMessage{
		{
			Key:     mail.MessageKey{AccountLabel: "work", UID: 1},
			Subject: "Test",
			From:    "alice@example.com",
			Date:    now,
			Body:    "Hello!",
			IsRead:  true,
		},
	}

	prompt, err := BuildPromptFromMessages("system", "", []string{"Useful", "Spam"}, messages)
	if err != nil {
		t.Fatalf("BuildPromptFromMessages failed: %v", err)
	}

	if !strings.Contains(prompt, "Hello!") {
		t.Error("missing message body")
	}
	if !strings.Contains(prompt, "- Spam") {
		t.Error("missing custom label")
	}
}

func TestBuildPrompt_MissingFieldInTemplate(t *testing.T) {
	// Template with a field that doesn't exist in the data should fail
	// because we use the missingkey=error option.
	req := Request{
		ClassificationPrompt: "{{.InvalidField}}",
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: "test",
			},
		},
	}

	_, err := BuildPrompt(req)
	if err == nil {
		t.Error("expected error for missing template field, got nil")
	}
}

func TestBuildPrompt_MissingKeyError(t *testing.T) {
	// Template referencing a field that doesn't exist in promptData.
	req := Request{
		ClassificationPrompt: "{{.NonExistent}}",
		Messages: []InputMessage{
			{
				Key:  mail.MessageKey{AccountLabel: "test", UID: 1},
				Body: "test",
			},
		},
	}

	_, err := BuildPrompt(req)
	if err == nil {
		t.Error("expected error for missing template key, got nil")
	}
}
