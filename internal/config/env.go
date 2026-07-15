package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// envPrefix is the prefix for all emailer environment variables.
const envPrefix = "EMAILER_"

// loadEnv overrides cfg fields from environment variables.
// Returns an aggregated error of all malformed values. The field for a
// malformed value keeps its current (default or previously loaded) value,
// allowing the run to proceed, but the caller (Load) surfaces every
// problem so the user can fix them.
func loadEnv(cfg *Config) error {
	var errs []error

	// ── Top-level ────────────────────────────────────────────────────
	errs = appendErr(errs, loadBool(envPrefix+"FETCH_UNREAD_ONLY", &cfg.FetchUnreadOnly))
	errs = appendErr(errs, loadDuration(envPrefix+"MAX_WINDOW", &cfg.MaxWindow))

	// ── LLM ──────────────────────────────────────────────────────────
	loadString(envPrefix+"LLM_PROVIDER", &cfg.LLM.Provider)
	loadString(envPrefix+"LLM_API_KEY", &cfg.LLM.APIKey)
	loadString(envPrefix+"LLM_MODEL", &cfg.LLM.Model)
	loadString(envPrefix+"LLM_ENDPOINT", &cfg.LLM.Endpoint)
	errs = appendErr(errs, loadDuration(envPrefix+"LLM_TIMEOUT", &cfg.LLM.Timeout))
	errs = appendErr(errs, loadInt(envPrefix+"LLM_MAX_RETRIES", &cfg.LLM.MaxRetries))
	errs = appendErr(errs, loadInt(envPrefix+"LLM_MAX_CONCURRENT", &cfg.LLM.MaxConcurrent))
	errs = appendErr(errs, loadInt(envPrefix+"LLM_ANALYSIS_REPAIR_MAX_ATTEMPTS", &cfg.LLM.AnalysisRepairMaxAttempts))

	// ── IMAP single account ──────────────────────────────────────────
	// If EMAILER_IMAP_HOST is set, create one account from env vars.
	var imapAcct IMAPAccount
	imapPresent := loadString(envPrefix+"IMAP_HOST", &imapAcct.Host)
	loadString(envPrefix+"IMAP_LABEL", &imapAcct.Label)
	errs = appendErr(errs, loadInt(envPrefix+"IMAP_PORT", &imapAcct.Port))
	loadString(envPrefix+"IMAP_USERNAME", &imapAcct.Username)
	loadString(envPrefix+"IMAP_PASSWORD", &imapAcct.Password)
	loadStringSlice(envPrefix+"IMAP_FOLDERS", &imapAcct.Folders)
	errs = appendErr(errs, loadBool(envPrefix+"IMAP_USE_TLS", &imapAcct.UseTLS))
	if imapPresent {
		imapAcct.normalize()
		cfg.IMAP.Accounts = append(cfg.IMAP.Accounts, imapAcct)
	}

	// Global IMAP command timeout (applies to every account).
	errs = appendErr(errs, loadDuration(envPrefix+"IMAP_TIMEOUT", &cfg.IMAP.Timeout))

	// ── Notify / Telegram ────────────────────────────────────────────
	loadString(envPrefix+"TELEGRAM_BOT_TOKEN", &cfg.Notify.Telegram.BotToken)
	errs = appendErr(errs, loadInt64(envPrefix+"TELEGRAM_CHAT_ID", &cfg.Notify.Telegram.ChatID))

	// ── Storage ──────────────────────────────────────────────────────
	loadString(envPrefix+"STATE_PATH", &cfg.Storage.StatePath)
	errs = appendErr(errs, loadBool(envPrefix+"STATELESS", &cfg.Storage.Stateless))

	// ── Digest ───────────────────────────────────────────────────────
	errs = appendErr(errs, loadInt(envPrefix+"DIGEST_MAX_MESSAGE_EXCERPT", &cfg.Digest.MaxMessageExcerpt))
	errs = appendErr(errs, loadBool(envPrefix+"DIGEST_INCLUDE_READ_STATUS", &cfg.Digest.IncludeReadStatus))

	// ── Labels ───────────────────────────────────────────────────────
	loadStringSlice(envPrefix+"LABELS_CUSTOM", &cfg.Labels.Custom)

	// ── Concurrency ──────────────────────────────────────────────────
	errs = appendErr(errs, loadInt(envPrefix+"CONCURRENCY_MAX_ACCOUNTS", &cfg.Concurrency.MaxAccounts))
	errs = appendErr(errs, loadInt(envPrefix+"CONCURRENCY_MAX_LLM_CALLS", &cfg.Concurrency.MaxLLMCalls))
	errs = appendErr(errs, loadInt(envPrefix+"CONCURRENCY_FETCH_BATCH_SIZE", &cfg.Concurrency.FetchBatchSize))

	return errors.Join(errs...)
}

// appendErr appends err to the slice if err is non-nil.
func appendErr(errs []error, err error) []error {
	if err != nil {
		return append(errs, err)
	}
	return errs
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
func loadBool(key string, dst *bool) error {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("env %q: %w", key, err)
	}
	*dst = b
	return nil
}

// loadInt reads an int env var via strconv.Atoi.
func loadInt(key string, dst *int) error {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fmt.Errorf("env %q: %w", key, err)
	}
	*dst = n
	return nil
}

// loadInt64 reads an int64 env var via strconv.ParseInt.
func loadInt64(key string, dst *int64) error {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fmt.Errorf("env %q: %w", key, err)
	}
	*dst = n
	return nil
}

// loadDuration reads a duration env var via time.ParseDuration.
func loadDuration(key string, dst *time.Duration) error {
	v, ok := os.LookupEnv(key)
	if !ok {
		return nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return fmt.Errorf("env %q: %w", key, err)
	}
	*dst = d
	return nil
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
