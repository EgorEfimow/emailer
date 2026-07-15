package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_DefaultsOnly(t *testing.T) {
	cfg, err := Load(LoadOptions{
		Args: []string{}, // no flags
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Spot-check a few defaults.
	if cfg.FetchUnreadOnly != false {
		t.Errorf("FetchUnreadOnly = %v, want false", cfg.FetchUnreadOnly)
	}
	if cfg.MaxWindow != 72*time.Hour {
		t.Errorf("MaxWindow = %v, want 72h", cfg.MaxWindow)
	}
	if cfg.LLM.Timeout != 120*time.Second {
		t.Errorf("LLM.Timeout = %v, want 120s", cfg.LLM.Timeout)
	}
	if cfg.Storage.StatePath != "./state/emailer.db" {
		t.Errorf("Storage.StatePath = %q, want %q", cfg.Storage.StatePath, "./state/emailer.db")
	}
	if cfg.IMAP.Accounts != nil {
		t.Errorf("IMAP.Accounts = %v, want nil", cfg.IMAP.Accounts)
	}
}

func TestLoad_YAMLOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
llm:
  provider: gemini
  timeout: 60s
storage:
  state_path: /custom/state.db
fetch_unread_only: true
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args:       []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LLM.Provider != "gemini" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "gemini")
	}
	if cfg.LLM.Timeout != 60*time.Second {
		t.Errorf("LLM.Timeout = %v, want 60s", cfg.LLM.Timeout)
	}
	if cfg.Storage.StatePath != "/custom/state.db" {
		t.Errorf("Storage.StatePath = %q, want %q", cfg.Storage.StatePath, "/custom/state.db")
	}
	if cfg.FetchUnreadOnly != true {
		t.Errorf("FetchUnreadOnly = %v, want true", cfg.FetchUnreadOnly)
	}

	// Unset fields remain at defaults.
	if cfg.LLM.MaxRetries != 3 {
		t.Errorf("LLM.MaxRetries = %d, want 3 (default preserved)", cfg.LLM.MaxRetries)
	}
}

func TestLoad_JSONOverridesDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	jsonContent := `{
  "llm": {
    "provider": "openrouter",
    "timeout": "90s"
  },
  "digest": {
    "max_message_excerpt": 250
  }
}`
	if err := os.WriteFile(path, []byte(jsonContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args:       []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LLM.Provider != "openrouter" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "openrouter")
	}
	if cfg.LLM.Timeout != 90*time.Second {
		t.Errorf("LLM.Timeout = %v, want 90s", cfg.LLM.Timeout)
	}
	if cfg.Digest.MaxMessageExcerpt != 250 {
		t.Errorf("Digest.MaxMessageExcerpt = %d, want 250", cfg.Digest.MaxMessageExcerpt)
	}

	// Unset fields remain at defaults.
	if cfg.LLM.MaxRetries != 3 {
		t.Errorf("LLM.MaxRetries = %d, want 3 (default preserved)", cfg.LLM.MaxRetries)
	}
}

func TestLoad_EnvOverridesFile(t *testing.T) {
	// Env should override values set in the YAML file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
llm:
  provider: gemini
  timeout: 120s
  max_retries: 5
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("EMAILER_LLM_PROVIDER", "ollama")
	t.Setenv("EMAILER_LLM_TIMEOUT", "30s")

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args:       []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Env overrides file.
	if cfg.LLM.Provider != "ollama" {
		t.Errorf("LLM.Provider = %q, want %q (env overrides file)", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.Timeout != 30*time.Second {
		t.Errorf("LLM.Timeout = %v, want 30s (env overrides file)", cfg.LLM.Timeout)
	}

	// Fields only in file (not overridden by env) stay.
	if cfg.LLM.MaxRetries != 5 {
		t.Errorf("LLM.MaxRetries = %d, want 5 (file value preserved)", cfg.LLM.MaxRetries)
	}
}

func TestLoad_FlagsOverrideEnv(t *testing.T) {
	// CLI flags should override env vars.
	t.Setenv("EMAILER_LLM_PROVIDER", "ollama")
	t.Setenv("EMAILER_LLM_TIMEOUT", "30s")
	t.Setenv("EMAILER_MAX_WINDOW", "48h")

	cfg, err := Load(LoadOptions{
		Args: []string{
			"--llm-provider", "gemini",
			"--max-window", "24h",
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Flags override env.
	if cfg.LLM.Provider != "gemini" {
		t.Errorf("LLM.Provider = %q, want %q (flags override env)", cfg.LLM.Provider, "gemini")
	}
	if cfg.MaxWindow != 24*time.Hour {
		t.Errorf("MaxWindow = %v, want 24h (flags override env)", cfg.MaxWindow)
	}

	// Env-only field (not overridden by flags) stays.
	if cfg.LLM.Timeout != 30*time.Second {
		t.Errorf("LLM.Timeout = %v, want 30s (env value preserved)", cfg.LLM.Timeout)
	}
}

func TestLoad_FullPrecedenceChain(t *testing.T) {
	// Complete chain: defaults → file → env → flags.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
llm:
  provider: gemini
  timeout: 120s
  max_retries: 5
  max_concurrent: 8
max_window: 72h
fetch_unread_only: false
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("EMAILER_LLM_PROVIDER", "ollama")     // overrides file
	t.Setenv("EMAILER_LLM_TIMEOUT", "60s")          // overrides file
	t.Setenv("EMAILER_MAX_WINDOW", "48h")            // overrides file

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args: []string{
			"--llm-provider", "openrouter", // overrides env
			"--llm-max-retries", "3",       // overrides file
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Flags win.
	if cfg.LLM.Provider != "openrouter" {
		t.Errorf("LLM.Provider = %q, want %q (flags win)", cfg.LLM.Provider, "openrouter")
	}
	if cfg.LLM.MaxRetries != 3 {
		t.Errorf("LLM.MaxRetries = %d, want 3 (flags win)", cfg.LLM.MaxRetries)
	}

	// Env wins over file, loses to flags.
	if cfg.LLM.Timeout != 60*time.Second {
		t.Errorf("LLM.Timeout = %v, want 60s (env wins over file)", cfg.LLM.Timeout)
	}
	if cfg.MaxWindow != 48*time.Hour {
		t.Errorf("MaxWindow = %v, want 48h (env wins over file)", cfg.MaxWindow)
	}

	// File-only field (not touched by env or flags).
	if cfg.LLM.MaxConcurrent != 8 {
		t.Errorf("LLM.MaxConcurrent = %d, want 8 (file preserved)", cfg.LLM.MaxConcurrent)
	}

	// Default-only field (not touched by file, env, or flags).
	if cfg.Storage.StatePath != "./state/emailer.db" {
		t.Errorf("Storage.StatePath = %q, want %q (default preserved)", cfg.Storage.StatePath, "./state/emailer.db")
	}

	// File value not overridden by env or flags.
	if cfg.FetchUnreadOnly != false {
		t.Errorf("FetchUnreadOnly = %v, want false (file preserved)", cfg.FetchUnreadOnly)
	}
}

func TestLoad_UnknownExtension(t *testing.T) {
	_, err := Load(LoadOptions{
		ConfigPath: "config.toml",
		Args:       []string{},
	})
	if err == nil {
		t.Fatal("Load: expected error for .toml extension, got nil")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(LoadOptions{
		ConfigPath: "/nonexistent/config.yaml",
		Args:       []string{},
	})
	if err == nil {
		t.Fatal("Load: expected error for missing file, got nil")
	}
}

func TestLoad_IMAPAccountsFromFile(t *testing.T) {
	// IMAP accounts defined in YAML file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
imap:
  accounts:
    - label: work
      host: imap.work.com
      username: user@work.com
      password: workpass
    - label: personal
      host: imap.personal.com
      username: me@personal.com
      password: personalpass
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args:       []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 2 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 2", len(cfg.IMAP.Accounts))
	}
	if cfg.IMAP.Accounts[0].Label != "work" {
		t.Errorf("Accounts[0].Label = %q, want %q", cfg.IMAP.Accounts[0].Label, "work")
	}
	if cfg.IMAP.Accounts[1].Label != "personal" {
		t.Errorf("Accounts[1].Label = %q, want %q", cfg.IMAP.Accounts[1].Label, "personal")
	}
}

func TestLoad_IMAPAccountsFilePlusEnv(t *testing.T) {
	// Env adds an IMAP account on top of what's in the file.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
imap:
  accounts:
    - label: work
      host: imap.work.com
      username: user@work.com
      password: workpass
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("EMAILER_IMAP_HOST", "imap.personal.com")
	t.Setenv("EMAILER_IMAP_LABEL", "personal")
	t.Setenv("EMAILER_IMAP_USERNAME", "me@personal.com")
	t.Setenv("EMAILER_IMAP_PASSWORD", "personalpass")

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args:       []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 2 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 2", len(cfg.IMAP.Accounts))
	}
}

func TestLoad_IMAPAccountsFilePlusEnvPlusFlags(t *testing.T) {
	// Flags add a third IMAP account on top of file + env.
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	yamlContent := `
imap:
  accounts:
    - label: work
      host: imap.work.com
      username: user@work.com
      password: workpass
`
	if err := os.WriteFile(path, []byte(yamlContent), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	t.Setenv("EMAILER_IMAP_HOST", "imap.personal.com")
	t.Setenv("EMAILER_IMAP_LABEL", "personal")
	t.Setenv("EMAILER_IMAP_USERNAME", "me@personal.com")
	t.Setenv("EMAILER_IMAP_PASSWORD", "personalpass")

	cfg, err := Load(LoadOptions{
		ConfigPath: path,
		Args: []string{
			"--imap-host", "imap.extra.com",
			"--imap-label", "extra",
			"--imap-username", "extra@test.com",
			"--imap-password", "extrapass",
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 3 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 3", len(cfg.IMAP.Accounts))
	}
	if cfg.IMAP.Accounts[0].Label != "work" {
		t.Errorf("Accounts[0].Label = %q, want %q", cfg.IMAP.Accounts[0].Label, "work")
	}
	if cfg.IMAP.Accounts[1].Label != "personal" {
		t.Errorf("Accounts[1].Label = %q, want %q", cfg.IMAP.Accounts[1].Label, "personal")
	}
	if cfg.IMAP.Accounts[2].Label != "extra" {
		t.Errorf("Accounts[2].Label = %q, want %q", cfg.IMAP.Accounts[2].Label, "extra")
	}
}

func TestLoad_FlagsDoNotCreateIMAPAccountWithoutHost(t *testing.T) {
	// Setting --imap-label without --imap-host should NOT create an account.
	cfg, err := Load(LoadOptions{
		Args: []string{
			"--imap-label", "orphan",
			"--imap-username", "test",
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 0 {
		t.Errorf("len(IMAP.Accounts) = %d, want 0 (no host set)", len(cfg.IMAP.Accounts))
	}
}

func TestLoad_FlagAliasesAndHyphens(t *testing.T) { //nolint:gocyclo
	// Verify that all expected flag names are parseable.
	cfg, err := Load(LoadOptions{
		Args: []string{
			"--fetch-unread-only",
			"--max-window", "12h",
			"--llm-provider", "gemini",
			"--llm-api-key", "test-key",
			"--llm-model", "gemini-2.0-flash",
			"--llm-endpoint", "https://example.com",
			"--llm-timeout", "30s",
			"--llm-max-retries", "2",
			"--llm-max-concurrent", "6",
			"--telegram-bot-token", "bot:token",
			"--telegram-chat-id", "-100123",
			"--state-path", "/tmp/test.db",
			"--stateless",
			"--digest-max-message-excerpt", "300",
			"--digest-include-read-status=false",
			"--labels-custom", "Urgent,Reference",
			"--concurrency-max-accounts", "8",
			"--concurrency-max-llm-calls", "10",
		},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.FetchUnreadOnly != true {
		t.Errorf("FetchUnreadOnly = %v, want true", cfg.FetchUnreadOnly)
	}
	if cfg.MaxWindow != 12*time.Hour {
		t.Errorf("MaxWindow = %v, want 12h", cfg.MaxWindow)
	}
	if cfg.LLM.Provider != "gemini" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "gemini")
	}
	if cfg.LLM.APIKey != "test-key" {
		t.Errorf("LLM.APIKey = %q, want %q", cfg.LLM.APIKey, "test-key")
	}
	if cfg.LLM.Model != "gemini-2.0-flash" {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, "gemini-2.0-flash")
	}
	if cfg.LLM.Endpoint != "https://example.com" {
		t.Errorf("LLM.Endpoint = %q, want %q", cfg.LLM.Endpoint, "https://example.com")
	}
	if cfg.LLM.Timeout != 30*time.Second {
		t.Errorf("LLM.Timeout = %v, want 30s", cfg.LLM.Timeout)
	}
	if cfg.LLM.MaxRetries != 2 {
		t.Errorf("LLM.MaxRetries = %d, want 2", cfg.LLM.MaxRetries)
	}
	if cfg.LLM.MaxConcurrent != 6 {
		t.Errorf("LLM.MaxConcurrent = %d, want 6", cfg.LLM.MaxConcurrent)
	}
	if cfg.Notify.Telegram.BotToken != "bot:token" {
		t.Errorf("Telegram.BotToken = %q, want %q", cfg.Notify.Telegram.BotToken, "bot:token")
	}
	if cfg.Notify.Telegram.ChatID != -100123 {
		t.Errorf("Telegram.ChatID = %d, want -100123", cfg.Notify.Telegram.ChatID)
	}
	if cfg.Storage.StatePath != "/tmp/test.db" {
		t.Errorf("Storage.StatePath = %q, want %q", cfg.Storage.StatePath, "/tmp/test.db")
	}
	if cfg.Storage.Stateless != true {
		t.Errorf("Storage.Stateless = %v, want true", cfg.Storage.Stateless)
	}
	if cfg.Digest.MaxMessageExcerpt != 300 {
		t.Errorf("Digest.MaxMessageExcerpt = %d, want 300", cfg.Digest.MaxMessageExcerpt)
	}
	if cfg.Digest.IncludeReadStatus != false {
		t.Errorf("Digest.IncludeReadStatus = %v, want false", cfg.Digest.IncludeReadStatus)
	}
	if len(cfg.Labels.Custom) != 2 || cfg.Labels.Custom[0] != "Urgent" || cfg.Labels.Custom[1] != "Reference" {
		t.Errorf("Labels.Custom = %v, want [Urgent Reference]", cfg.Labels.Custom)
	}
	if cfg.Concurrency.MaxAccounts != 8 {
		t.Errorf("Concurrency.MaxAccounts = %d, want 8", cfg.Concurrency.MaxAccounts)
	}
	if cfg.Concurrency.MaxLLMCalls != 10 {
		t.Errorf("Concurrency.MaxLLMCalls = %d, want 10", cfg.Concurrency.MaxLLMCalls)
	}
}

func TestLoad_UnknownFlagReturnsError(t *testing.T) {
	_, err := Load(LoadOptions{
		Args: []string{"--unknown-flag", "value"},
	})
	if err == nil {
		t.Fatal("Load: expected error for unknown flag, got nil")
	}
}

func TestLoad_EnvKeyFormatCase(t *testing.T) {
	// Verify that env vars use the expected EMAILER_ prefix and key format.
	t.Setenv("EMAILER_LLM_MODEL", "custom-model")
	t.Setenv("EMAILER_DIGEST_INCLUDE_READ_STATUS", "false")

	cfg, err := Load(LoadOptions{
		Args: []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.LLM.Model != "custom-model" {
		t.Errorf("LLM.Model = %q, want %q", cfg.LLM.Model, "custom-model")
	}
	if cfg.Digest.IncludeReadStatus != false {
		t.Errorf("Digest.IncludeReadStatus = %v, want false", cfg.Digest.IncludeReadStatus)
	}
}

func TestLoad_EmptyArgs(t *testing.T) {
	// Empty args slice should not error and should not override defaults.
	cfg, err := Load(LoadOptions{
		Args: []string{},
	})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.FetchUnreadOnly != false {
		t.Errorf("FetchUnreadOnly = %v, want false", cfg.FetchUnreadOnly)
	}
}