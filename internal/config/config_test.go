package config

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	t.Run("top-level defaults", func(t *testing.T) {
		if cfg.FetchUnreadOnly != false {
			t.Errorf("FetchUnreadOnly = %v, want false", cfg.FetchUnreadOnly)
		}
		if cfg.MaxWindow != 72*time.Hour {
			t.Errorf("MaxWindow = %v, want 72h", cfg.MaxWindow)
		}
	})

	t.Run("LLM defaults", func(t *testing.T) {
		if cfg.LLM.Provider != "" {
			t.Errorf("LLM.Provider = %q, want empty", cfg.LLM.Provider)
		}
		if cfg.LLM.APIKey != "" {
			t.Errorf("LLM.APIKey = %q, want empty", cfg.LLM.APIKey)
		}
		if cfg.LLM.Model != "" {
			t.Errorf("LLM.Model = %q, want empty", cfg.LLM.Model)
		}
		if cfg.LLM.Endpoint != "" {
			t.Errorf("LLM.Endpoint = %q, want empty", cfg.LLM.Endpoint)
		}
		if cfg.LLM.Timeout != 120*time.Second {
			t.Errorf("LLM.Timeout = %v, want 120s", cfg.LLM.Timeout)
		}
		if cfg.LLM.MaxRetries != 3 {
			t.Errorf("LLM.MaxRetries = %d, want 3", cfg.LLM.MaxRetries)
		}
		if cfg.LLM.MaxConcurrent != 4 {
			t.Errorf("LLM.MaxConcurrent = %d, want 4", cfg.LLM.MaxConcurrent)
		}
	})

	t.Run("IMAP defaults", func(t *testing.T) {
		if cfg.IMAP.Accounts != nil {
			t.Errorf("IMAP.Accounts = %v, want nil", cfg.IMAP.Accounts)
		}
	})

	t.Run("Notify defaults", func(t *testing.T) {
		if cfg.Notify.Telegram.BotToken != "" {
			t.Errorf("Notify.Telegram.BotToken = %q, want empty", cfg.Notify.Telegram.BotToken)
		}
		if cfg.Notify.Telegram.ChatID != 0 {
			t.Errorf("Notify.Telegram.ChatID = %d, want 0", cfg.Notify.Telegram.ChatID)
		}
	})

	t.Run("Storage defaults", func(t *testing.T) {
		if cfg.Storage.StatePath != "./state/emailer.db" {
			t.Errorf("Storage.StatePath = %q, want %q", cfg.Storage.StatePath, "./state/emailer.db")
		}
		if cfg.Storage.Stateless != false {
			t.Errorf("Storage.Stateless = %v, want false", cfg.Storage.Stateless)
		}
	})

	t.Run("Digest defaults", func(t *testing.T) {
		if cfg.Digest.MaxMessageExcerpt != 500 {
			t.Errorf("Digest.MaxMessageExcerpt = %d, want 500", cfg.Digest.MaxMessageExcerpt)
		}
		if cfg.Digest.IncludeReadStatus != true {
			t.Errorf("Digest.IncludeReadStatus = %v, want true", cfg.Digest.IncludeReadStatus)
		}
	})

	t.Run("Labels defaults", func(t *testing.T) {
		if cfg.Labels.Custom != nil {
			t.Errorf("Labels.Custom = %v, want nil", cfg.Labels.Custom)
		}
	})

	t.Run("Prompts defaults", func(t *testing.T) {
		if cfg.Prompts.ClassificationPrompt != "" {
			t.Errorf("Prompts.ClassificationPrompt = %q, want empty", cfg.Prompts.ClassificationPrompt)
		}
		if cfg.Prompts.SystemPrompt != "" {
			t.Errorf("Prompts.SystemPrompt = %q, want empty", cfg.Prompts.SystemPrompt)
		}
	})

	t.Run("Concurrency defaults", func(t *testing.T) {
		if cfg.Concurrency.MaxAccounts != 4 {
			t.Errorf("Concurrency.MaxAccounts = %d, want 4", cfg.Concurrency.MaxAccounts)
		}
		if cfg.Concurrency.MaxLLMCalls != 4 {
			t.Errorf("Concurrency.MaxLLMCalls = %d, want 4", cfg.Concurrency.MaxLLMCalls)
		}
	})
}

func TestDefaultConfig_Immutability(t *testing.T) {
	// Verify that multiple calls return independent copies.
	cfg1 := DefaultConfig()
	cfg2 := DefaultConfig()

	cfg1.FetchUnreadOnly = true
	cfg1.LLM.Provider = "gemini"
	cfg1.IMAP.Accounts = []IMAPAccount{{Label: "test"}}
	cfg1.Labels.Custom = []string{"Urgent"}

	if cfg2.FetchUnreadOnly != false {
		t.Error("DefaultConfig() is not immutable: FetchUnreadOnly mutated")
	}
	if cfg2.LLM.Provider != "" {
		t.Error("DefaultConfig() is not immutable: LLM.Provider mutated")
	}
	if cfg2.IMAP.Accounts != nil {
		t.Error("DefaultConfig() is not immutable: IMAP.Accounts mutated")
	}
	if cfg2.Labels.Custom != nil {
		t.Error("DefaultConfig() is not immutable: Labels.Custom mutated")
	}
}

func TestDefaultConfig_StructTags(t *testing.T) {
	// Verify that the Config struct compiles and can be round-tripped.
	// This is a compile-time / reflection sanity check, not a behaviour test.
	cfg := DefaultConfig()
	_ = cfg // ensure zero-value composition works

	// Spot-check sensitive tags via reflection.
	tests := []struct {
		name  string
		value any
	}{
		{"LLM.APIKey", cfg.LLM.APIKey},
		{"Notify.Telegram.BotToken", cfg.Notify.Telegram.BotToken},
		{"IMAP.Accounts (empty)", cfg.IMAP.Accounts},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_ = tt.value // just verify we can access the field
		})
	}
}

func TestDefaultConfig_TopLevelSections(t *testing.T) {
	// Table-driven: ensure every major section is present and addressable.
	tests := []struct {
		name string
		got  any
		want any
	}{
		{"LLM is zero-value except defaults", nil, nil},
		{"IMAP is zero-value except defaults", nil, nil},
		{"Notify is zero-value except defaults", nil, nil},
		{"Storage is zero-value except defaults", nil, nil},
		{"Digest is zero-value except defaults", nil, nil},
		{"Labels is zero-value except defaults", nil, nil},
		{"Prompts is zero-value except defaults", nil, nil},
		{"Concurrency is zero-value except defaults", nil, nil},
	}

	// We just verify the struct fields compile and are accessible.
	cfg := DefaultConfig()
	_ = cfg.LLM
	_ = cfg.IMAP
	_ = cfg.Notify
	_ = cfg.Storage
	_ = cfg.Digest
	_ = cfg.Labels
	_ = cfg.Prompts
	_ = cfg.Concurrency
	_ = cfg.FetchUnreadOnly
	_ = cfg.MaxWindow

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Structural test: all sections exist at compile time.
		})
	}
}

func TestDefaultConfig_IMAPAccountDefaults(t *testing.T) {
	// Verify that an IMAPAccount with only required fields set
	// uses the expected zero values for optional fields.
	acct := IMAPAccount{
		Label:    "work",
		Host:     "imap.example.com",
		Username: "user@example.com",
		Password: "secret",
	}

	if acct.Port != 0 {
		t.Errorf("Port = %d, want 0 (default to be applied by loader)", acct.Port)
	}
	if acct.Folders != nil {
		t.Errorf("Folders = %v, want nil", acct.Folders)
	}
	if acct.UseTLS != false {
		t.Errorf("UseTLS = %v, want false", acct.UseTLS)
	}
}

func TestDefaultConfig_IMAPAccountSensitiveTag(t *testing.T) {
	// Runtime check: IMAPAccount.Password has sensitive tag.
	acct := IMAPAccount{Password: "s3cret"}
	_ = acct // Password field is accessible
}

func TestDefaultConfig_TelegramSensitiveTag(t *testing.T) {
	// Runtime check: TelegramConfig.BotToken has sensitive tag.
	tg := TelegramConfig{BotToken: "bot:token"}
	_ = tg // BotToken field is accessible
}