package config

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Validate checks the entire Config for correctness and returns an aggregate
// error of all validation failures. A nil return means the config is valid.
func Validate(cfg Config) error {
	var errs []error

	errs = append(errs, validateTopLevel(cfg)...)
	errs = append(errs, validateLLMConfig(cfg.LLM)...)
	errs = append(errs, validateIMAPConfig(cfg.IMAP)...)
	errs = append(errs, validateNotifyConfig(cfg.Notify)...)
	errs = append(errs, validateStorageConfig(cfg.Storage, cfg.FetchUnreadOnly)...)
	errs = append(errs, validateDigestConfig(cfg.Digest)...)
	errs = append(errs, validateConcurrencyConfig(cfg.Concurrency)...)
	errs = append(errs, validateLabelsConfig(cfg.Labels)...)

	return errors.Join(errs...)
}

// ---------------------------------------------------------------------------
// Top-level validation
// ---------------------------------------------------------------------------

func validateTopLevel(cfg Config) (errs []error) {
	if cfg.MaxWindow <= 0 {
		errs = append(errs, fmt.Errorf("max_window must be positive, got %v", cfg.MaxWindow))
	}
	if !cfg.FetchUnreadOnly && !cfg.Storage.Stateless && cfg.Storage.StatePath == "" {
		errs = append(errs, errors.New("storage.state_path is required when fetch_unread_only=false and stateless=false"))
	}
	return errs
}

// ---------------------------------------------------------------------------
// LLM section
// ---------------------------------------------------------------------------

func validateLLMConfig(c LLMConfig) (errs []error) {
	if c.Provider == "" {
		errs = append(errs, errors.New("llm.provider is required"))
	}
	if c.APIKey == "" {
		errs = append(errs, errors.New("llm.api_key is required"))
	}
	if c.Model == "" {
		errs = append(errs, errors.New("llm.model is required"))
	}
	if c.Timeout <= 0 {
		errs = append(errs, fmt.Errorf("llm.timeout must be positive, got %v", c.Timeout))
	}
	if c.MaxRetries < 0 {
		errs = append(errs, fmt.Errorf("llm.max_retries must be >= 0, got %d", c.MaxRetries))
	}
	if c.MaxConcurrent <= 0 {
		errs = append(errs, fmt.Errorf("llm.max_concurrent must be positive, got %d", c.MaxConcurrent))
	}
	return errs
}

// ---------------------------------------------------------------------------
// IMAP section
// ---------------------------------------------------------------------------

func validateIMAPConfig(c IMAPConfig) (errs []error) {
	if len(c.Accounts) == 0 {
		errs = append(errs, errors.New("imap.accounts: at least one account is required"))
	}
	if c.Timeout < 0 {
		errs = append(errs, fmt.Errorf("imap.timeout must be non-negative, got %v", c.Timeout))
	}
	for i := range c.Accounts {
		c.Accounts[i].normalize()
		errs = append(errs, validateIMAPAccount(i, c.Accounts[i])...)
	}
	// Duplicate labels would corrupt the (account_label, uid) dedup key.
	// Check after per-account validation so every label is individually
	// validated first.
	seen := make(map[string]int, len(c.Accounts))
	for i := range c.Accounts {
		label := c.Accounts[i].Label
		if label == "" {
			continue // already reported by validateIMAPAccount
		}
		if first, dup := seen[label]; dup {
			errs = append(errs, fmt.Errorf(
				"imap.accounts[%d] and imap.accounts[%d] have the same label %q; labels must be unique",
				first, i, label,
			))
		} else {
			seen[label] = i
		}
	}
	return errs
}

func validateIMAPAccount(idx int, a IMAPAccount) (errs []error) {
	prefix := fmt.Sprintf("imap.accounts[%d]", idx)

	if strings.TrimSpace(a.Label) == "" {
		errs = append(errs, fmt.Errorf("%s.label is required", prefix))
	}
	if strings.TrimSpace(a.Host) == "" {
		errs = append(errs, fmt.Errorf("%s.host is required", prefix))
	}
	if a.Port < 1 || a.Port > 65535 {
		errs = append(errs, fmt.Errorf("%s.port must be between 1 and 65535, got %d", prefix, a.Port))
	}
	if strings.TrimSpace(a.Username) == "" {
		errs = append(errs, fmt.Errorf("%s.username is required", prefix))
	}
	if a.Password == "" {
		errs = append(errs, fmt.Errorf("%s.password is required", prefix))
	}
	for j, f := range a.Folders {
		if strings.TrimSpace(f) == "" {
			errs = append(errs, fmt.Errorf("%s.folders[%d] must not be empty", prefix, j))
		}
	}
	if a.Host != "" && strings.Contains(a.Host, "://") {
		errs = append(errs, fmt.Errorf("%s.host must not contain a scheme (use hostname or IP only), got %q", prefix, a.Host))
	}
	return errs
}

// ---------------------------------------------------------------------------
// Notify section
// ---------------------------------------------------------------------------

func validateNotifyConfig(c NotifyConfig) (errs []error) {
	return append(errs, validateTelegramConfig(c.Telegram)...)
}

func validateTelegramConfig(c TelegramConfig) (errs []error) {
	if c.BotToken == "" {
		errs = append(errs, errors.New("notify.telegram.bot_token is required"))
	}
	if c.ChatID == 0 {
		errs = append(errs, errors.New("notify.telegram.chat_id must be non-zero"))
	}
	if c.BotToken != "" {
		parts := strings.SplitN(c.BotToken, ":", 2)
		if len(parts) != 2 || len(parts[0]) == 0 || len(parts[1]) == 0 {
			errs = append(errs, errors.New("notify.telegram.bot_token does not look like a valid bot token (expected format: <digits>:<token>)"))
		}
	}
	return errs
}

// ---------------------------------------------------------------------------
// Storage section
// ---------------------------------------------------------------------------

func validateStorageConfig(c StorageConfig, fetchUnreadOnly bool) (errs []error) {
	if c.Stateless && !fetchUnreadOnly {
		errs = append(errs, errors.New("storage.stateless requires fetch_unread_only=true: without persistence, a stateless run cannot deduplicate across all messages"))
	}
	return errs
}

// ---------------------------------------------------------------------------
// Digest section
// ---------------------------------------------------------------------------

func validateDigestConfig(c DigestConfig) (errs []error) {
	if c.MaxMessageExcerpt <= 0 {
		errs = append(errs, fmt.Errorf("digest.max_message_excerpt must be positive, got %d", c.MaxMessageExcerpt))
	}
	return errs
}

// ---------------------------------------------------------------------------
// Concurrency section
// ---------------------------------------------------------------------------

func validateConcurrencyConfig(c ConcurrencyConfig) (errs []error) {
	if c.MaxAccounts <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max_accounts must be positive, got %d", c.MaxAccounts))
	}
	if c.MaxLLMCalls <= 0 {
		errs = append(errs, fmt.Errorf("concurrency.max_llm_calls must be positive, got %d", c.MaxLLMCalls))
	}
	if c.FetchBatchSize < 0 {
		errs = append(errs, fmt.Errorf("concurrency.fetch_batch_size must be non-negative, got %d", c.FetchBatchSize))
	}
	return errs
}

// ---------------------------------------------------------------------------
// Labels section
// ---------------------------------------------------------------------------

func validateLabelsConfig(c LabelsConfig) (errs []error) {
	for i, l := range c.Custom {
		if strings.TrimSpace(l) == "" {
			errs = append(errs, fmt.Errorf("labels.custom[%d] must not be empty", i))
		}
	}
	return errs
}

// ---------------------------------------------------------------------------
// Secret redaction patterns
// ---------------------------------------------------------------------------

// SecretRedactionPatterns returns compiled regexp patterns matching secret
// values in the config, suitable for the log package's redaction helper.
func SecretRedactionPatterns(cfg Config) []*regexp.Regexp {
	var patterns []*regexp.Regexp

	if cfg.LLM.APIKey != "" {
		patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(cfg.LLM.APIKey)))
	}
	if cfg.Notify.Telegram.BotToken != "" {
		patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(cfg.Notify.Telegram.BotToken)))
	}
	for i := range cfg.IMAP.Accounts {
		if cfg.IMAP.Accounts[i].Password != "" {
			patterns = append(patterns, regexp.MustCompile(regexp.QuoteMeta(cfg.IMAP.Accounts[i].Password)))
		}
	}
	return patterns
}
