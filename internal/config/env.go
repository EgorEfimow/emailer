package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

// envPrefix is the prefix for all emailer environment variables.
const envPrefix = "EMAILER_"

// loadEnv overrides cfg fields from environment variables.
// Returns nil — all parse errors are silently ignored (the field keeps its
// current value). This keeps the loader lenient: a malformed env var does
// not abort the entire configuration load.
func loadEnv(cfg *Config) error {
	// ── Top-level ────────────────────────────────────────────────────
	loadBool(envPrefix+"FETCH_UNREAD_ONLY", &cfg.FetchUnreadOnly)
	loadDuration(envPrefix+"MAX_WINDOW", &cfg.MaxWindow)

	// ── LLM ──────────────────────────────────────────────────────────
	loadString(envPrefix+"LLM_PROVIDER", &cfg.LLM.Provider)
	loadString(envPrefix+"LLM_API_KEY", &cfg.LLM.APIKey)
	loadString(envPrefix+"LLM_MODEL", &cfg.LLM.Model)
	loadString(envPrefix+"LLM_ENDPOINT", &cfg.LLM.Endpoint)
	loadDuration(envPrefix+"LLM_TIMEOUT", &cfg.LLM.Timeout)
	loadInt(envPrefix+"LLM_MAX_RETRIES", &cfg.LLM.MaxRetries)
	loadInt(envPrefix+"LLM_MAX_CONCURRENT", &cfg.LLM.MaxConcurrent)

	// ── IMAP single account ──────────────────────────────────────────
	// If EMAILER_IMAP_HOST is set, create one account from env vars.
	var imapAcct IMAPAccount
	imapPresent := loadString(envPrefix+"IMAP_HOST", &imapAcct.Host)
	loadString(envPrefix+"IMAP_LABEL", &imapAcct.Label)
	loadInt(envPrefix+"IMAP_PORT", &imapAcct.Port)
	loadString(envPrefix+"IMAP_USERNAME", &imapAcct.Username)
	loadString(envPrefix+"IMAP_PASSWORD", &imapAcct.Password)
	loadStringSlice(envPrefix+"IMAP_FOLDERS", &imapAcct.Folders)
	loadBool(envPrefix+"IMAP_USE_TLS", &imapAcct.UseTLS)
	if imapPresent {
		cfg.IMAP.Accounts = append(cfg.IMAP.Accounts, imapAcct)
	}

	// ── Notify / Telegram ────────────────────────────────────────────
	loadString(envPrefix+"TELEGRAM_BOT_TOKEN", &cfg.Notify.Telegram.BotToken)
	loadInt64(envPrefix+"TELEGRAM_CHAT_ID", &cfg.Notify.Telegram.ChatID)

	// ── Storage ──────────────────────────────────────────────────────
	loadString(envPrefix+"STATE_PATH", &cfg.Storage.StatePath)
	loadBool(envPrefix+"STATELESS", &cfg.Storage.Stateless)

	// ── Digest ───────────────────────────────────────────────────────
	loadInt(envPrefix+"DIGEST_MAX_MESSAGE_EXCERPT", &cfg.Digest.MaxMessageExcerpt)
	loadBool(envPrefix+"DIGEST_INCLUDE_READ_STATUS", &cfg.Digest.IncludeReadStatus)

	// ── Labels ───────────────────────────────────────────────────────
	loadStringSlice(envPrefix+"LABELS_CUSTOM", &cfg.Labels.Custom)

	// ── Concurrency ──────────────────────────────────────────────────
	loadInt(envPrefix+"CONCURRENCY_MAX_ACCOUNTS", &cfg.Concurrency.MaxAccounts)
	loadInt(envPrefix+"CONCURRENCY_MAX_LLM_CALLS", &cfg.Concurrency.MaxLLMCalls)

	return nil
}

// ── load helpers ──────────────────────────────────────────────────────────

// loadString reads a string env var. Returns true if the var was set.
func loadString(key string, dst *string) bool {
	v, ok := os.LookupEnv(key)
	if ok {
		*dst = v
	}
	return ok
}

// loadBool reads a bool env var via strconv.ParseBool.
func loadBool(key string, dst *bool) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	b, err := strconv.ParseBool(v)
	if err == nil {
		*dst = b
	}
}

// loadInt reads an int env var via strconv.Atoi.
func loadInt(key string, dst *int) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	n, err := strconv.Atoi(v)
	if err == nil {
		*dst = n
	}
}

// loadInt64 reads an int64 env var via strconv.ParseInt.
func loadInt64(key string, dst *int64) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err == nil {
		*dst = n
	}
}

// loadDuration reads a duration env var via time.ParseDuration.
func loadDuration(key string, dst *time.Duration) {
	v, ok := os.LookupEnv(key)
	if !ok {
		return
	}
	d, err := time.ParseDuration(v)
	if err == nil {
		*dst = d
	}
}

// loadStringSlice reads a comma-separated env var into a string slice.
// Empty values in the list are skipped.
func loadStringSlice(key string, dst *[]string) {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return
	}
	parts := strings.Split(v, ",")
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) > 0 {
		*dst = cleaned
	}
}