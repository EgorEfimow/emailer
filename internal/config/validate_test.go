package config

import (
	"strings"
	"testing"
	"time"
)

func mustHaveErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q, got nil", substr)
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got %q", substr, err.Error())
	}
}

func mustNotHaveErr(t *testing.T, err error, substr string) {
	t.Helper()
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), substr) {
		t.Fatalf("unexpected error containing %q: %v", substr, err)
	}
}

func TestValidate_ValidConfig(t *testing.T) {
	cfg := Config{
		FetchUnreadOnly: false,
		MaxWindow:       72 * time.Hour,
		LLM: LLMConfig{
			Provider:      "gemini",
			APIKey:        "test-key",
			Model:         "gemini-2.0-flash",
			Timeout:       30 * time.Second,
			MaxRetries:    3,
			MaxConcurrent: 4,
		},
		IMAP: IMAPConfig{
			Accounts: []IMAPAccount{
				{
					Label:    "work",
					Host:     "imap.example.com",
					Port:     993,
					Username: "user@example.com",
					Password: "secret",
					Folders:  []string{"INBOX"},
					UseTLS:   true,
				},
			},
		},
		Notify: NotifyConfig{
			Telegram: TelegramConfig{
				BotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
				ChatID:   -1001234567890,
			},
		},
		Storage: StorageConfig{
			StatePath: "./state/emailer.db",
			Stateless: false,
		},
		Digest: DigestConfig{
			MaxMessageExcerpt: 500,
			IncludeReadStatus: true,
		},
		Labels: LabelsConfig{
			Custom: []string{"Urgent", "Reference"},
		},
		Concurrency: ConcurrencyConfig{
			MaxAccounts: 4,
			MaxLLMCalls: 4,
		},
	}

	if err := Validate(cfg); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestValidate_TopLevel(t *testing.T) {
	t.Run("max_window zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.MaxWindow = 0
		mustHaveErr(t, Validate(cfg), "max_window must be positive")
	})

	t.Run("max_window negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.MaxWindow = -1 * time.Hour
		mustHaveErr(t, Validate(cfg), "max_window must be positive")
	})

	t.Run("state_path empty when fetch_unread_only false", func(t *testing.T) {
		cfg := validConfig()
		cfg.FetchUnreadOnly = false
		cfg.Storage.StatePath = ""
		cfg.Storage.Stateless = false
		mustHaveErr(t, Validate(cfg), "storage.state_path is required")
	})

	t.Run("state_path empty when stateless true is ok", func(t *testing.T) {
		cfg := validConfig()
		cfg.FetchUnreadOnly = true
		cfg.Storage.StatePath = ""
		cfg.Storage.Stateless = true
		if err := Validate(cfg); err != nil {
			t.Fatalf("expected no error for stateless with fetch_unread_only=true, got: %v", err)
		}
	})
}

func TestValidate_LLMConfig(t *testing.T) {
	t.Run("missing provider", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.Provider = ""
		mustHaveErr(t, Validate(cfg), "llm.provider is required")
	})

	t.Run("missing api_key", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.APIKey = ""
		mustHaveErr(t, Validate(cfg), "llm.api_key is required")
	})

	t.Run("missing model", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.Model = ""
		mustHaveErr(t, Validate(cfg), "llm.model is required")
	})

	t.Run("timeout zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.Timeout = 0
		mustHaveErr(t, Validate(cfg), "llm.timeout must be positive")
	})

	t.Run("timeout negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.Timeout = -1
		mustHaveErr(t, Validate(cfg), "llm.timeout must be positive")
	})

	t.Run("max_retries negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.MaxRetries = -1
		mustHaveErr(t, Validate(cfg), "llm.max_retries must be >= 0")
	})

	t.Run("max_concurrent zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.MaxConcurrent = 0
		mustHaveErr(t, Validate(cfg), "llm.max_concurrent must be positive")
	})

	t.Run("max_retries zero is ok", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.MaxRetries = 0
		if err := Validate(cfg); err != nil {
			mustNotHaveErr(t, err, "max_retries")
		}
	})

	t.Run("analysis_repair_max_attempts negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.AnalysisRepairMaxAttempts = -1
		mustHaveErr(t, Validate(cfg), "analysis_repair_max_attempts must be >= 0")
	})

	t.Run("analysis_repair_max_attempts zero is ok", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.AnalysisRepairMaxAttempts = 0
		if err := Validate(cfg); err != nil {
			mustNotHaveErr(t, err, "analysis_repair_max_attempts")
		}
	})

	t.Run("analysis_repair_max_attempts positive is ok", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.AnalysisRepairMaxAttempts = 5
		if err := Validate(cfg); err != nil {
			mustNotHaveErr(t, err, "analysis_repair_max_attempts")
		}
	})
}

func TestValidate_IMAPConfig(t *testing.T) {
	t.Run("no accounts", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts = nil
		mustHaveErr(t, Validate(cfg), "at least one account is required")
	})

	t.Run("empty accounts slice", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts = []IMAPAccount{}
		mustHaveErr(t, Validate(cfg), "at least one account is required")
	})

	t.Run("missing label", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Label = ""
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].label is required")
	})

	t.Run("blank label", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Label = "  "
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].label is required")
	})

	t.Run("missing host", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Host = ""
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].host is required")
	})

	t.Run("host with scheme", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Host = "imaps://imap.example.com"
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].host must not contain a scheme")
	})

	t.Run("port out of range low", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Port = -1
		mustHaveErr(t, Validate(cfg), "port must be between 1 and 65535")
	})

	t.Run("port out of range high", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Port = 99999
		mustHaveErr(t, Validate(cfg), "port must be between 1 and 65535")
	})

	t.Run("port zero is normalized to 993", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Port = 0
		if err := Validate(cfg); err != nil {
			mustNotHaveErr(t, err, "port")
		}
	})

	t.Run("missing username", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Username = ""
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].username is required")
	})

	t.Run("missing password", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Password = ""
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].password is required")
	})

	t.Run("empty folder name", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts[0].Folders = []string{"INBOX", ""}
		mustHaveErr(t, Validate(cfg), "imap.accounts[0].folders[1] must not be empty")
	})

	t.Run("multiple accounts, one invalid", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts = append(cfg.IMAP.Accounts, IMAPAccount{
			Label: "",
			Host:  "imap.other.com",
		})
		err := Validate(cfg)
		// accounts[0] is valid from validConfig(); only accounts[1] should fail.
		mustHaveErr(t, err, "imap.accounts[1].label is required")
		mustHaveErr(t, err, "imap.accounts[1].username is required")
		mustHaveErr(t, err, "imap.accounts[1].password is required")
		})

		t.Run("duplicate labels", func(t *testing.T) {
			cfg := validConfig()
			cfg.IMAP.Accounts = append(cfg.IMAP.Accounts, IMAPAccount{
				Label:    cfg.IMAP.Accounts[0].Label,
				Host:     "imap.other.com",
				Username: "user@other.com",
				Password: "otherpass",
			})
			err := Validate(cfg)
			mustHaveErr(t, err, "labels must be unique")
		})
	}

func TestValidate_NotifyConfig(t *testing.T) {
	t.Run("missing bot_token", func(t *testing.T) {
		cfg := validConfig()
		cfg.Notify.Telegram.BotToken = ""
		mustHaveErr(t, Validate(cfg), "notify.telegram.bot_token is required")
	})

	t.Run("chat_id zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Notify.Telegram.ChatID = 0
		mustHaveErr(t, Validate(cfg), "notify.telegram.chat_id must be non-zero")
	})

	t.Run("malformed bot_token", func(t *testing.T) {
		cfg := validConfig()
		cfg.Notify.Telegram.BotToken = "not-a-token"
		mustHaveErr(t, Validate(cfg), "notify.telegram.bot_token does not look like a valid bot token")
	})

	t.Run("bot_token missing second part", func(t *testing.T) {
		cfg := validConfig()
		cfg.Notify.Telegram.BotToken = "12345:"
		mustHaveErr(t, Validate(cfg), "notify.telegram.bot_token does not look like a valid bot token")
	})
}

func TestValidate_StorageConfig(t *testing.T) {
	t.Run("stateless without fetch_unread_only", func(t *testing.T) {
		cfg := validConfig()
		cfg.FetchUnreadOnly = false
		cfg.Storage.Stateless = true
		cfg.Storage.StatePath = ""
		mustHaveErr(t, Validate(cfg), "storage.stateless requires fetch_unread_only=true")
	})

	t.Run("stateless with fetch_unread_only passes", func(t *testing.T) {
		cfg := validConfig()
		cfg.FetchUnreadOnly = true
		cfg.Storage.Stateless = true
		cfg.Storage.StatePath = ""
		if err := Validate(cfg); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})
}

func TestValidate_DigestConfig(t *testing.T) {
	t.Run("max_message_excerpt zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Digest.MaxMessageExcerpt = 0
		mustHaveErr(t, Validate(cfg), "digest.max_message_excerpt must be positive")
	})

	t.Run("max_message_excerpt negative", func(t *testing.T) {
		cfg := validConfig()
		cfg.Digest.MaxMessageExcerpt = -1
		mustHaveErr(t, Validate(cfg), "digest.max_message_excerpt must be positive")
	})
}

func TestValidate_ConcurrencyConfig(t *testing.T) {
	t.Run("max_accounts zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Concurrency.MaxAccounts = 0
		mustHaveErr(t, Validate(cfg), "concurrency.max_accounts must be positive")
	})

	t.Run("max_llm_calls zero", func(t *testing.T) {
		cfg := validConfig()
		cfg.Concurrency.MaxLLMCalls = 0
		mustHaveErr(t, Validate(cfg), "concurrency.max_llm_calls must be positive")
	})
}

func TestValidate_LabelsConfig(t *testing.T) {
	t.Run("empty custom label", func(t *testing.T) {
		cfg := validConfig()
		cfg.Labels.Custom = []string{"Valid", "", "AlsoValid"}
		mustHaveErr(t, Validate(cfg), "labels.custom[1] must not be empty")
	})
}

func TestValidate_AggregatedErrors(t *testing.T) {
	// Multiple failures at once should all be reported.
	cfg := Config{} // zero value — many things wrong
	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected aggregated errors, got nil")
	}

	expected := []string{
		"max_window must be positive",
		"llm.provider is required",
		"llm.api_key is required",
		"llm.model is required",
		"llm.timeout must be positive",
		"llm.max_concurrent must be positive",
		"at least one account is required",
		"notify.telegram.bot_token is required",
		"notify.telegram.chat_id must be non-zero",
		"digest.max_message_excerpt must be positive",
		"concurrency.max_accounts must be positive",
		"concurrency.max_llm_calls must be positive",
	}

	for _, s := range expected {
		if !strings.Contains(err.Error(), s) {
			t.Errorf("expected error to contain %q, got: %v", s, err)
		}
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// validConfig returns a Config that passes Validate. Tests mutate one field
// at a time to trigger specific failures.
func validConfig() Config {
	return Config{
		FetchUnreadOnly: false,
		MaxWindow:       72 * time.Hour,
		LLM: LLMConfig{
			Provider:      "gemini",
			APIKey:        "test-api-key",
			Model:         "gemini-2.0-flash",
			Timeout:       30 * time.Second,
			MaxRetries:    3,
			MaxConcurrent: 4,
		},
		IMAP: IMAPConfig{
			Accounts: []IMAPAccount{
				{
					Label:    "work",
					Host:     "imap.example.com",
					Port:     993,
					Username: "user@example.com",
					Password: "s3cret",
					Folders:  []string{"INBOX"},
					UseTLS:   true,
				},
			},
		},
		Notify: NotifyConfig{
			Telegram: TelegramConfig{
				BotToken: "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11",
				ChatID:   -1001234567890,
			},
		},
		Storage: StorageConfig{
			StatePath: "./state/emailer.db",
			Stateless: false,
		},
		Digest: DigestConfig{
			MaxMessageExcerpt: 500,
			IncludeReadStatus: true,
		},
		Labels: LabelsConfig{
			Custom: nil,
		},
		Concurrency: ConcurrencyConfig{
			MaxAccounts: 4,
			MaxLLMCalls: 4,
		},
	}
}

func TestSecretRedactionPatterns(t *testing.T) {
	t.Run("all secrets collected", func(t *testing.T) {
		cfg := validConfig()
		patterns := SecretRedactionPatterns(cfg)
		if len(patterns) != 3 {
			t.Fatalf("expected 3 patterns (api_key, bot_token, password), got %d", len(patterns))
		}

		testStr := cfg.LLM.APIKey + " " + cfg.Notify.Telegram.BotToken + " " + cfg.IMAP.Accounts[0].Password
		for _, p := range patterns {
			if !p.MatchString(testStr) {
				t.Errorf("pattern %q did not match test string", p.String())
			}
		}
	})

	t.Run("empty secrets produce no patterns", func(t *testing.T) {
		cfg := validConfig()
		cfg.LLM.APIKey = ""
		cfg.Notify.Telegram.BotToken = ""
		cfg.IMAP.Accounts[0].Password = ""

		patterns := SecretRedactionPatterns(cfg)
		if len(patterns) != 0 {
			t.Errorf("expected 0 patterns, got %d", len(patterns))
		}
	})

	t.Run("multiple accounts produce multiple password patterns", func(t *testing.T) {
		cfg := validConfig()
		cfg.IMAP.Accounts = append(cfg.IMAP.Accounts, IMAPAccount{
			Label:    "personal",
			Host:     "imap.personal.com",
			Username: "me@personal.com",
			Password: "another-password",
		})

		// Remove the top-level password requirement (work account still has one).
		patterns := SecretRedactionPatterns(cfg)
		passwordPatternCount := 0
		for _, p := range patterns {
			if p.MatchString("s3cret") || p.MatchString("another-password") {
				passwordPatternCount++
			}
		}
		if passwordPatternCount != 2 {
			t.Errorf("expected 2 password patterns, got %d", passwordPatternCount)
		}
	})
}