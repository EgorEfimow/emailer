package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeYAML is a test helper that writes content to a temp file and returns
// the path. Content is treated as a raw YAML string.
func writeYAML(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeYAML: %v", err)
	}
	return path
}

func TestLoadYAML_FullOverride(t *testing.T) { //nolint:gocyclo
	cfg := DefaultConfig()
	yamlContent := `
llm:
  provider: gemini
  api_key: AIzaSyTestKey
  model: gemini-2.0-flash
  endpoint: https://custom.example.com
  timeout: 60s
  max_retries: 5
  max_concurrent: 8

imap:
  accounts:
    - label: work
      host: imap.example.com
      port: 993
      username: user@example.com
      password: s3cret
      folders:
        - INBOX
        - Archive
      use_tls: true
    - label: personal
      host: imap.personal.com
      port: 993
      username: me@personal.com
      password: p4ss
      folders:
        - INBOX
      use_tls: true

notify:
  telegram:
    bot_token: bot:token
    chat_id: -1001234567890

storage:
  state_path: /data/emailer.db
  stateless: true

digest:
  max_message_excerpt: 1000
  include_read_status: false

labels:
  custom:
    - Important
    - FollowUp

prompts:
  classification_prompt: "Classify this email: {{.Body}}"
  system_prompt: "You are a helpful assistant."

concurrency:
  max_accounts: 2
  max_llm_calls: 6

fetch_unread_only: true
max_window: 24h
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	// ── Top-level ────────────────────────────────────────────────────────
	if cfg.FetchUnreadOnly != true {
		t.Errorf("FetchUnreadOnly = %v, want true", cfg.FetchUnreadOnly)
	}
	if cfg.MaxWindow != 24*time.Hour {
		t.Errorf("MaxWindow = %v, want 24h", cfg.MaxWindow)
	}

	// ── LLM ──────────────────────────────────────────────────────────────
	if cfg.LLM.Provider != "gemini" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "gemini")
	}
	if cfg.LLM.APIKey != "AIzaSyTestKey" {
		t.Errorf("LLM.APIKey = %q, want %q", cfg.LLM.APIKey, "AIzaSyTestKey")
	}
	if cfg.LLM.Model != "gemini-2.0-flash" {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, "gemini-2.0-flash")
	}
	if cfg.LLM.Endpoint != "https://custom.example.com" {
		t.Errorf("LLM.Endpoint = %q, want %q", cfg.LLM.Endpoint, "https://custom.example.com")
	}
	if cfg.LLM.Timeout != 60*time.Second {
		t.Errorf("LLM.Timeout = %v, want 60s", cfg.LLM.Timeout)
	}
	if cfg.LLM.MaxRetries != 5 {
		t.Errorf("LLM.MaxRetries = %d, want 5", cfg.LLM.MaxRetries)
	}
	if cfg.LLM.MaxConcurrent != 8 {
		t.Errorf("LLM.MaxConcurrent = %d, want 8", cfg.LLM.MaxConcurrent)
	}

	// ── IMAP ─────────────────────────────────────────────────────────────
	if len(cfg.IMAP.Accounts) != 2 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 2", len(cfg.IMAP.Accounts))
	}

	acct0 := cfg.IMAP.Accounts[0]
	if acct0.Label != "work" {
		t.Errorf("Accounts[0].Label = %q, want %q", acct0.Label, "work")
	}
	if acct0.Host != "imap.example.com" {
		t.Errorf("Accounts[0].Host = %q, want %q", acct0.Host, "imap.example.com")
	}
	if acct0.Port != 993 {
		t.Errorf("Accounts[0].Port = %d, want 993", acct0.Port)
	}
	if acct0.Username != "user@example.com" {
		t.Errorf("Accounts[0].Username = %q, want %q", acct0.Username, "user@example.com")
	}
	if acct0.Password != "s3cret" {
		t.Errorf("Accounts[0].Password = %q, want %q", acct0.Password, "s3cret")
	}
	if len(acct0.Folders) != 2 || acct0.Folders[0] != "INBOX" || acct0.Folders[1] != "Archive" {
		t.Errorf("Accounts[0].Folders = %v, want [INBOX Archive]", acct0.Folders)
	}
	if acct0.UseTLS != true {
		t.Errorf("Accounts[0].UseTLS = %v, want true", acct0.UseTLS)
	}

	acct1 := cfg.IMAP.Accounts[1]
	if acct1.Label != "personal" {
		t.Errorf("Accounts[1].Label = %q, want %q", acct1.Label, "personal")
	}

	// ── Notify ───────────────────────────────────────────────────────────
	if cfg.Notify.Telegram.BotToken != "bot:token" {
		t.Errorf("Telegram.BotToken = %q, want %q", cfg.Notify.Telegram.BotToken, "bot:token")
	}
	if cfg.Notify.Telegram.ChatID != -1001234567890 {
		t.Errorf("Telegram.ChatID = %d, want -1001234567890", cfg.Notify.Telegram.ChatID)
	}

	// ── Storage ──────────────────────────────────────────────────────────
	if cfg.Storage.StatePath != "/data/emailer.db" {
		t.Errorf("Storage.StatePath = %q, want %q", cfg.Storage.StatePath, "/data/emailer.db")
	}
	if cfg.Storage.Stateless != true {
		t.Errorf("Storage.Stateless = %v, want true", cfg.Storage.Stateless)
	}

	// ── Digest ───────────────────────────────────────────────────────────
	if cfg.Digest.MaxMessageExcerpt != 1000 {
		t.Errorf("Digest.MaxMessageExcerpt = %d, want 1000", cfg.Digest.MaxMessageExcerpt)
	}
	if cfg.Digest.IncludeReadStatus != false {
		t.Errorf("Digest.IncludeReadStatus = %v, want false", cfg.Digest.IncludeReadStatus)
	}

	// ── Labels ───────────────────────────────────────────────────────────
	wantLabels := []string{"Important", "FollowUp"}
	if len(cfg.Labels.Custom) != len(wantLabels) {
		t.Fatalf("len(Labels.Custom) = %d, want %d", len(cfg.Labels.Custom), len(wantLabels))
	}
	for i := range wantLabels {
		if cfg.Labels.Custom[i] != wantLabels[i] {
			t.Errorf("Labels.Custom[%d] = %q, want %q", i, cfg.Labels.Custom[i], wantLabels[i])
		}
	}

	// ── Prompts ─────────────────────────────────────────────────────────
	if cfg.Prompts.ClassificationPrompt != "Classify this email: {{.Body}}" {
		t.Errorf("Prompts.ClassificationPrompt = %q, want %q", cfg.Prompts.ClassificationPrompt, "Classify this email: {{.Body}}")
	}
	if cfg.Prompts.SystemPrompt != "You are a helpful assistant." {
		t.Errorf("Prompts.SystemPrompt = %q, want %q", cfg.Prompts.SystemPrompt, "You are a helpful assistant.")
	}

	// ── Concurrency ─────────────────────────────────────────────────────
	if cfg.Concurrency.MaxAccounts != 2 {
		t.Errorf("Concurrency.MaxAccounts = %d, want 2", cfg.Concurrency.MaxAccounts)
	}
	if cfg.Concurrency.MaxLLMCalls != 6 {
		t.Errorf("Concurrency.MaxLLMCalls = %d, want 6", cfg.Concurrency.MaxLLMCalls)
	}
}

func TestLoadYAML_PartialOverride(t *testing.T) {
	// Only set LLM provider and timeout; everything else stays at default.
	cfg := DefaultConfig()
	yamlContent := `
llm:
  provider: ollama
  timeout: 30s
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	if cfg.LLM.Provider != "ollama" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.Timeout != 30*time.Second {
		t.Errorf("LLM.Timeout = %v, want 30s", cfg.LLM.Timeout)
	}

	// Unset fields should remain at defaults.
	if cfg.LLM.APIKey != "" {
		t.Errorf("LLM.APIKey = %q, want empty (default preserved)", cfg.LLM.APIKey)
	}
	if cfg.LLM.Model != "" {
		t.Errorf("LLM.Model = %q, want empty (default preserved)", cfg.LLM.Model)
	}
	if cfg.LLM.MaxRetries != 3 {
		t.Errorf("LLM.MaxRetries = %d, want 3 (default preserved)", cfg.LLM.MaxRetries)
	}
	if cfg.LLM.MaxConcurrent != 4 {
		t.Errorf("LLM.MaxConcurrent = %d, want 4 (default preserved)", cfg.LLM.MaxConcurrent)
	}

	// Non-LLM sections should remain at defaults.
	if cfg.FetchUnreadOnly != false {
		t.Errorf("FetchUnreadOnly = %v, want false (default preserved)", cfg.FetchUnreadOnly)
	}
	if cfg.MaxWindow != 72*time.Hour {
		t.Errorf("MaxWindow = %v, want 72h (default preserved)", cfg.MaxWindow)
	}
	if cfg.Storage.StatePath != "./state/emailer.db" {
		t.Errorf("Storage.StatePath = %q, want %q (default preserved)", cfg.Storage.StatePath, "./state/emailer.db")
	}
}

func TestLoadYAML_EmptyFile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LLM.Provider = "custom"
	path := writeYAML(t, "")

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML on empty file: %v", err)
	}

	// Config should be unchanged.
	if cfg.LLM.Provider != "custom" {
		t.Errorf("LLM.Provider = %q, want %q (preserved after empty file)", cfg.LLM.Provider, "custom")
	}
}

func TestLoadYAML_CommentsOnly(t *testing.T) {
	cfg := DefaultConfig()
	// YAML with only comments is effectively empty.
	path := writeYAML(t, "# This is a comment\n# Another comment\n")

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML on comments-only file: %v", err)
	}

	// Top-level config should be unchanged.
	if cfg.MaxWindow != 72*time.Hour {
		t.Errorf("MaxWindow = %v, want 72h (preserved)", cfg.MaxWindow)
	}
}

func TestLoadYAML_MissingFile(t *testing.T) {
	cfg := DefaultConfig()

	err := loadYAML("/nonexistent/path/config.yaml", &cfg)
	if err == nil {
		t.Fatal("loadYAML: expected error for missing file, got nil")
	}
}

func TestLoadYAML_MalformedYAML(t *testing.T) {
	cfg := DefaultConfig()
	path := writeYAML(t, "llm:\n  provider: [unclosed bracket\n")

	err := loadYAML(path, &cfg)
	if err == nil {
		t.Fatal("loadYAML: expected error for malformed YAML, got nil")
	}
}

func TestLoadYAML_IMAPAccountsOverrideReplaces(t *testing.T) {
	// When YAML specifies imap.accounts, it should replace the entire
	// accounts slice, not append.
	cfg := DefaultConfig()
	cfg.IMAP.Accounts = []IMAPAccount{
		{Label: "old", Host: "old.example.com"},
	}

	yamlContent := `
imap:
  accounts:
    - label: new
      host: new.example.com
      username: user
      password: pass
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 1 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 1 (replaced, not appended)", len(cfg.IMAP.Accounts))
	}
	if cfg.IMAP.Accounts[0].Label != "new" {
		t.Errorf("Accounts[0].Label = %q, want %q", cfg.IMAP.Accounts[0].Label, "new")
	}
}

func TestLoadYAML_NoAccounts(t *testing.T) {
	// YAML without imap section leaves accounts nil.
	cfg := DefaultConfig()
	path := writeYAML(t, "llm:\n  provider: gemini\n")

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	if cfg.IMAP.Accounts != nil {
		t.Errorf("IMAP.Accounts = %v, want nil", cfg.IMAP.Accounts)
	}
}

func TestLoadYAML_DurationFormats(t *testing.T) {
	// Verify that various duration formats are accepted.
	cfg := DefaultConfig()
	yamlContent := `
max_window: 48h
llm:
  timeout: 90s
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	if cfg.MaxWindow != 48*time.Hour {
		t.Errorf("MaxWindow = %v, want 48h", cfg.MaxWindow)
	}
	if cfg.LLM.Timeout != 90*time.Second {
		t.Errorf("LLM.Timeout = %v, want 90s", cfg.LLM.Timeout)
	}
}

func TestLoadYAML_SensitiveFields(t *testing.T) {
	// Verify that sensitive fields are loadable from YAML (they are loaders,
	// not displayers — redaction is the log package's concern).
	cfg := DefaultConfig()
	yamlContent := `
llm:
  api_key: super-secret-key
notify:
  telegram:
    bot_token: bot:secret
imap:
  accounts:
    - label: main
      host: imap.example.com
      username: u
      password: p4ssw0rd
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	if cfg.LLM.APIKey != "super-secret-key" {
		t.Errorf("LLM.APIKey = %q, want %q", cfg.LLM.APIKey, "super-secret-key")
	}
	if cfg.Notify.Telegram.BotToken != "bot:secret" {
		t.Errorf("Telegram.BotToken = %q, want %q", cfg.Notify.Telegram.BotToken, "bot:secret")
	}
	if len(cfg.IMAP.Accounts) != 1 || cfg.IMAP.Accounts[0].Password != "p4ssw0rd" {
		t.Errorf("IMAP.Accounts[0].Password = %q, want %q", cfg.IMAP.Accounts[0].Password, "p4ssw0rd")
	}
}

func TestLoadYAML_CustomLabels(t *testing.T) {
	cfg := DefaultConfig()
	yamlContent := `
labels:
  custom:
    - Urgent
    - Reference
    - Spam
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	want := []string{"Urgent", "Reference", "Spam"}
	if len(cfg.Labels.Custom) != len(want) {
		t.Fatalf("len(Labels.Custom) = %d, want %d", len(cfg.Labels.Custom), len(want))
	}
	for i := range want {
		if cfg.Labels.Custom[i] != want[i] {
			t.Errorf("Labels.Custom[%d] = %q, want %q", i, cfg.Labels.Custom[i], want[i])
		}
	}
}

func TestLoadYAML_EmptyAccounts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IMAP.Accounts = []IMAPAccount{
		{Label: "old", Host: "old.example.com", Username: "u", Password: "p"},
	}
	yamlContent := `
imap:
  accounts: []
`
	path := writeYAML(t, yamlContent)

	if err := loadYAML(path, &cfg); err != nil {
		t.Fatalf("loadYAML: %v", err)
	}

	// An explicitly empty accounts list should clear the slice.
	if len(cfg.IMAP.Accounts) != 0 {
		t.Errorf("len(IMAP.Accounts) = %d, want 0 (explicitly emptied)", len(cfg.IMAP.Accounts))
	}
}