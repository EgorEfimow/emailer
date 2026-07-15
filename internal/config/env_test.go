package config

import (
	"testing"
	"time"
)

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestLoadEnv_TopLevel(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_FETCH_UNREAD_ONLY": "true",
		"EMAILER_MAX_WINDOW":        "24h",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.FetchUnreadOnly != true {
		t.Errorf("FetchUnreadOnly = %v, want true", cfg.FetchUnreadOnly)
	}
	if cfg.MaxWindow != 24*time.Hour {
		t.Errorf("MaxWindow = %v, want 24h", cfg.MaxWindow)
	}
}

func TestLoadEnv_LLM(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_LLM_PROVIDER":      "gemini",
		"EMAILER_LLM_API_KEY":       "AIzaSy...",
		"EMAILER_LLM_MODEL":         "gemini-2.0-flash",
		"EMAILER_LLM_ENDPOINT":      "https://custom.example.com",
		"EMAILER_LLM_TIMEOUT":       "60s",
		"EMAILER_LLM_MAX_RETRIES":   "5",
		"EMAILER_LLM_MAX_CONCURRENT": "8",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.LLM.Provider != "gemini" {
		t.Errorf("LLM.Provider = %q, want %q", cfg.LLM.Provider, "gemini")
	}
	if cfg.LLM.APIKey != "AIzaSy..." {
		t.Errorf("LLM.APIKey = %q, want %q", cfg.LLM.APIKey, "AIzaSy...")
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
}

func TestLoadEnv_IMAPSingleAccount(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_IMAP_HOST":     "imap.example.com",
		"EMAILER_IMAP_LABEL":    "work",
		"EMAILER_IMAP_PORT":     "993",
		"EMAILER_IMAP_USERNAME": "user@example.com",
		"EMAILER_IMAP_PASSWORD": "s3cret",
		"EMAILER_IMAP_FOLDERS":  "INBOX,Archive",
		"EMAILER_IMAP_USE_TLS":  "true",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 1 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 1", len(cfg.IMAP.Accounts))
	}

	acct := cfg.IMAP.Accounts[0]
	if acct.Host != "imap.example.com" {
		t.Errorf("Host = %q, want %q", acct.Host, "imap.example.com")
	}
	if acct.Label != "work" {
		t.Errorf("Label = %q, want %q", acct.Label, "work")
	}
	if acct.Port != 993 {
		t.Errorf("Port = %d, want 993", acct.Port)
	}
	if acct.Username != "user@example.com" {
		t.Errorf("Username = %q, want %q", acct.Username, "user@example.com")
	}
	if acct.Password != "s3cret" {
		t.Errorf("Password = %q, want %q", acct.Password, "s3cret")
	}
	if len(acct.Folders) != 2 || acct.Folders[0] != "INBOX" || acct.Folders[1] != "Archive" {
		t.Errorf("Folders = %v, want [INBOX Archive]", acct.Folders)
	}
	if acct.UseTLS != true {
		t.Errorf("UseTLS = %v, want true", acct.UseTLS)
	}
}

func TestLoadEnv_IMAPOnlyWhenHostSet(t *testing.T) {
	// When no IMAP_HOST is set, no account should be created even if
	// other IMAP env vars are present.
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_IMAP_USERNAME": "user@example.com",
		"EMAILER_IMAP_PASSWORD": "s3cret",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 0 {
		t.Errorf("len(IMAP.Accounts) = %d, want 0 (no HOST set)", len(cfg.IMAP.Accounts))
	}
}

func TestLoadEnv_Telegram(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_TELEGRAM_BOT_TOKEN": "bot:token",
		"EMAILER_TELEGRAM_CHAT_ID":   "-1001234567890",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.Notify.Telegram.BotToken != "bot:token" {
		t.Errorf("Telegram.BotToken = %q, want %q", cfg.Notify.Telegram.BotToken, "bot:token")
	}
	if cfg.Notify.Telegram.ChatID != -1001234567890 {
		t.Errorf("Telegram.ChatID = %d, want -1001234567890", cfg.Notify.Telegram.ChatID)
	}
}

func TestLoadEnv_Storage(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_STATE_PATH": "/data/emailer.db",
		"EMAILER_STATELESS":  "true",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.Storage.StatePath != "/data/emailer.db" {
		t.Errorf("Storage.StatePath = %q, want %q", cfg.Storage.StatePath, "/data/emailer.db")
	}
	if cfg.Storage.Stateless != true {
		t.Errorf("Storage.Stateless = %v, want true", cfg.Storage.Stateless)
	}
}

func TestLoadEnv_Digest(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_DIGEST_MAX_MESSAGE_EXCERPT":       "1000",
		"EMAILER_DIGEST_INCLUDE_READ_STATUS":       "false",
		"EMAILER_DIGEST_INCLUDE_GLOBAL_STATS":      "false",
		"EMAILER_DIGEST_INCLUDE_ACCOUNT_STATS":     "false",
		"EMAILER_DIGEST_INCLUDE_SUMMARIES":         "false",
		"EMAILER_DIGEST_INCLUDE_KEY_POINTS":        "false",
		"EMAILER_DIGEST_INCLUDE_ACTION_ITEMS":      "false",
		"EMAILER_DIGEST_INCLUDE_RAW_EXCERPT_FALLBACK": "false",
		"EMAILER_DIGEST_MAX_MESSAGES":              "50",
		"EMAILER_DIGEST_MAX_KEY_POINTS_PER_MESSAGE": "3",
		"EMAILER_DIGEST_MAX_ACTION_ITEMS_PER_MESSAGE": "2",
		"EMAILER_DIGEST_PRIORITY_ONLY":             "true",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.Digest.MaxMessageExcerpt != 1000 {
		t.Errorf("Digest.MaxMessageExcerpt = %d, want 1000", cfg.Digest.MaxMessageExcerpt)
	}
	if cfg.Digest.IncludeReadStatus != false {
		t.Errorf("Digest.IncludeReadStatus = %v, want false", cfg.Digest.IncludeReadStatus)
	}
	if cfg.Digest.IncludeGlobalStats != false {
		t.Errorf("Digest.IncludeGlobalStats = %v, want false", cfg.Digest.IncludeGlobalStats)
	}
	if cfg.Digest.IncludeAccountStats != false {
		t.Errorf("Digest.IncludeAccountStats = %v, want false", cfg.Digest.IncludeAccountStats)
	}
	if cfg.Digest.IncludeSummaries != false {
		t.Errorf("Digest.IncludeSummaries = %v, want false", cfg.Digest.IncludeSummaries)
	}
	if cfg.Digest.IncludeKeyPoints != false {
		t.Errorf("Digest.IncludeKeyPoints = %v, want false", cfg.Digest.IncludeKeyPoints)
	}
	if cfg.Digest.IncludeActionItems != false {
		t.Errorf("Digest.IncludeActionItems = %v, want false", cfg.Digest.IncludeActionItems)
	}
	if cfg.Digest.IncludeRawExcerptFallback != false {
		t.Errorf("Digest.IncludeRawExcerptFallback = %v, want false", cfg.Digest.IncludeRawExcerptFallback)
	}
	if cfg.Digest.MaxMessages != 50 {
		t.Errorf("Digest.MaxMessages = %d, want 50", cfg.Digest.MaxMessages)
	}
	if cfg.Digest.MaxKeyPointsPerMessage != 3 {
		t.Errorf("Digest.MaxKeyPointsPerMessage = %d, want 3", cfg.Digest.MaxKeyPointsPerMessage)
	}
	if cfg.Digest.MaxActionItemsPerMessage != 2 {
		t.Errorf("Digest.MaxActionItemsPerMessage = %d, want 2", cfg.Digest.MaxActionItemsPerMessage)
	}
	if cfg.Digest.PriorityOnly != true {
		t.Errorf("Digest.PriorityOnly = %v, want true", cfg.Digest.PriorityOnly)
	}
}

func TestLoadEnv_Labels(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_LABELS_CUSTOM": "Important,FollowUp,Spam",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	want := []string{"Important", "FollowUp", "Spam"}
	if len(cfg.Labels.Custom) != len(want) {
		t.Fatalf("len(Labels.Custom) = %d, want %d", len(cfg.Labels.Custom), len(want))
	}
	for i := range want {
		if cfg.Labels.Custom[i] != want[i] {
			t.Errorf("Labels.Custom[%d] = %q, want %q", i, cfg.Labels.Custom[i], want[i])
		}
	}
}

func TestLoadEnv_Concurrency(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_CONCURRENCY_MAX_ACCOUNTS": "2",
		"EMAILER_CONCURRENCY_MAX_LLM_CALLS": "6",
	})

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.Concurrency.MaxAccounts != 2 {
		t.Errorf("Concurrency.MaxAccounts = %d, want 2", cfg.Concurrency.MaxAccounts)
	}
	if cfg.Concurrency.MaxLLMCalls != 6 {
		t.Errorf("Concurrency.MaxLLMCalls = %d, want 6", cfg.Concurrency.MaxLLMCalls)
	}
}

func TestLoadEnv_UnsetVarsDoNotOverride(t *testing.T) {
	// When no env vars are set, the config should remain at defaults.
	cfg := DefaultConfig()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Timeout = 30 * time.Second
	cfg.Storage.StatePath = "/custom/path"

	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if cfg.LLM.Provider != "ollama" {
		t.Errorf("LLM.Provider = %q, want %q (preserved)", cfg.LLM.Provider, "ollama")
	}
	if cfg.LLM.Timeout != 30*time.Second {
		t.Errorf("LLM.Timeout = %v, want 30s (preserved)", cfg.LLM.Timeout)
	}
	if cfg.Storage.StatePath != "/custom/path" {
		t.Errorf("Storage.StatePath = %q, want %q (preserved)", cfg.Storage.StatePath, "/custom/path")
	}
}

func TestLoadEnv_MalformedValuesReturnError(t *testing.T) {
	cfg := DefaultConfig()

	setEnv(t, map[string]string{
		"EMAILER_MAX_WINDOW":                     "not-a-duration",
		"EMAILER_LLM_MAX_RETRIES":                "not-an-int",
		"EMAILER_FETCH_UNREAD_ONLY":              "not-a-bool",
		"EMAILER_TELEGRAM_CHAT_ID":               "not-an-int64",
		"EMAILER_DIGEST_MAX_MESSAGE_EXCERPT":      "not-an-int",
	})

	err := loadEnv(&cfg)
	if err == nil {
		t.Fatal("loadEnv: expected error for malformed env values, got nil")
	}

	// All should remain at defaults despite errors.
	if cfg.MaxWindow != 72*time.Hour {
		t.Errorf("MaxWindow = %v, want 72h (default preserved)", cfg.MaxWindow)
	}
	if cfg.LLM.MaxRetries != 3 {
		t.Errorf("LLM.MaxRetries = %d, want 3 (default preserved)", cfg.LLM.MaxRetries)
	}
	if cfg.FetchUnreadOnly != false {
		t.Errorf("FetchUnreadOnly = %v, want false (default preserved)", cfg.FetchUnreadOnly)
	}
	if cfg.Notify.Telegram.ChatID != 0 {
		t.Errorf("Telegram.ChatID = %d, want 0 (default preserved)", cfg.Notify.Telegram.ChatID)
	}
	if cfg.Digest.MaxMessageExcerpt != 500 {
		t.Errorf("Digest.MaxMessageExcerpt = %d, want 500 (default preserved)", cfg.Digest.MaxMessageExcerpt)
	}
}

func TestLoadEnv_EmptyStringSlice(t *testing.T) {
	cfg := DefaultConfig()

	// Empty string should not set anything.
	t.Setenv("EMAILER_LABELS_CUSTOM", "")
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
	if cfg.Labels.Custom != nil {
		t.Errorf("Labels.Custom = %v, want nil", cfg.Labels.Custom)
	}

	// Whitespace-only string should not set anything.
	t.Setenv("EMAILER_LABELS_CUSTOM", "  ,  , ")
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}
	if cfg.Labels.Custom != nil {
		t.Errorf("Labels.Custom = %v, want nil", cfg.Labels.Custom)
	}
}

func TestLoadEnv_IMAPDefaultsApplied(t *testing.T) {
	// Only set HOST; the rest should get documented defaults via normalize().
	cfg := DefaultConfig()

	t.Setenv("EMAILER_IMAP_HOST", "imap.example.com")
	if err := loadEnv(&cfg); err != nil {
		t.Fatalf("loadEnv: %v", err)
	}

	if len(cfg.IMAP.Accounts) != 1 {
		t.Fatalf("len(IMAP.Accounts) = %d, want 1", len(cfg.IMAP.Accounts))
	}

	acct := cfg.IMAP.Accounts[0]
	if acct.Host != "imap.example.com" {
		t.Errorf("Host = %q, want %q", acct.Host, "imap.example.com")
	}
	if acct.Port != 993 {
		t.Errorf("Port = %d, want 993 (default)", acct.Port)
	}
	if len(acct.Folders) != 1 || acct.Folders[0] != "INBOX" {
		t.Errorf("Folders = %v, want [INBOX]", acct.Folders)
	}
	if acct.UseTLS != true {
		t.Errorf("UseTLS = %v, want true (default)", acct.UseTLS)
	}
}