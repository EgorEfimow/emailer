// Package config provides the typed configuration schema for the emailer
// application. Configuration is loaded from layered sources in precedence
// order: defaults → YAML file → env vars → CLI flags.
//
// Every secret field carries the struct tag `sensitive:"true"` and is
// automatically redacted in structured logs by the log package.
package config

import "time"

// Config is the top-level application configuration.
type Config struct {
	LLM         LLMConfig         `yaml:"llm" json:"llm"`
	IMAP        IMAPConfig        `yaml:"imap" json:"imap"`
	Notify      NotifyConfig      `yaml:"notify" json:"notify"`
	Storage     StorageConfig     `yaml:"storage" json:"storage"`
	Digest      DigestConfig      `yaml:"digest" json:"digest"`
	Labels      LabelsConfig      `yaml:"labels" json:"labels"`
	Prompts     PromptConfig      `yaml:"prompts" json:"prompts"`
	Concurrency ConcurrencyConfig `yaml:"concurrency" json:"concurrency"`

	// FetchUnreadOnly restricts fetching to unread messages only.
	// When false (default), all messages in the time window are fetched,
	// and the SQLite store is required for deduplication.
	FetchUnreadOnly bool `yaml:"fetch_unread_only" json:"fetch_unread_only"`

	// MaxWindow caps the dynamic lookback period to prevent overwhelming
	// the LLM after prolonged host downtime. Default is 72h.
	MaxWindow time.Duration `yaml:"max_window" json:"max_window"`
}

// ---------------------------------------------------------------------------
// LLM section
// ---------------------------------------------------------------------------

// LLMConfig configures the LLM provider and request behaviour.
type LLMConfig struct {
	// Provider selects the LLM backend (e.g. "gemini", "ollama", "openrouter").
	Provider string `yaml:"provider" json:"provider"`

	// APIKey is the provider authentication token.
	APIKey string `yaml:"api_key" json:"api_key" sensitive:"true"`

	// Model is the model identifier (e.g. "gemini-2.0-flash", "gpt-4o").
	Model string `yaml:"model" json:"model"`

	// Endpoint overrides the default provider URL. Optional.
	Endpoint string `yaml:"endpoint" json:"endpoint"`

	// Timeout is the per-request timeout for LLM calls.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`

	// MaxRetries is the number of retry attempts on transient failures.
	MaxRetries int `yaml:"max_retries" json:"max_retries"`

	// MaxConcurrent limits the number of simultaneous LLM provider calls.
	MaxConcurrent int `yaml:"max_concurrent" json:"max_concurrent"`

	// AnalysisRepairMaxAttempts is the number of repair attempts for
	// individual message analyses that fail validation. Each attempt
	// sends a repair prompt to the LLM. Default is 1. Set to 0 to disable.
	AnalysisRepairMaxAttempts int `yaml:"analysis_repair_max_attempts" json:"analysis_repair_max_attempts"`
}

// ---------------------------------------------------------------------------
// IMAP section
// ---------------------------------------------------------------------------

// IMAPConfig holds the list of IMAP accounts to ingest.
type IMAPConfig struct {
	Accounts []IMAPAccount `yaml:"accounts" json:"accounts"`

	// Timeout bounds each IMAP command (dial, login, select, fetch, store).
	// A zero value means no timeout. Default is 30s.
	Timeout time.Duration `yaml:"timeout" json:"timeout"`
}

// IMAPAccount represents a single IMAP mailbox to fetch messages from.
type IMAPAccount struct {
	// Label is a unique human-readable identifier for this account
	// (e.g. "work", "personal"). Used as part of the composite dedup key
	// (account_label, uid).
	Label string `yaml:"label" json:"label"`

	// Host is the IMAP server hostname.
	Host string `yaml:"host" json:"host"`

	// Port is the IMAP server port. Default is 993 (IMAPS).
	Port int `yaml:"port" json:"port"`

	// Username for IMAP authentication.
	Username string `yaml:"username" json:"username"`

	// Password for IMAP authentication (app password recommended).
	Password string `yaml:"password" json:"password" sensitive:"true"`

	// Folders is the list of mailbox folders to inspect. Default is ["INBOX"].
	Folders []string `yaml:"folders" json:"folders"`

	// UseTLS enables TLS for the IMAP connection. Default is true.
	UseTLS bool `yaml:"use_tls" json:"use_tls"`
}

// normalize applies documented defaults to an IMAPAccount for fields left at
// their zero value: port 993 (IMAPS), TLS enabled, folder INBOX.
func (a *IMAPAccount) normalize() {
	if a.Port == 0 {
		a.Port = 993
		a.UseTLS = true
	}
	if a.Folders == nil {
		a.Folders = []string{"INBOX"}
	}
}

// ---------------------------------------------------------------------------
// Notify section
// ---------------------------------------------------------------------------

// NotifyConfig holds settings for all notification channels.
type NotifyConfig struct {
	// Telegram configures the Telegram bot notification channel.
	Telegram TelegramConfig `yaml:"telegram" json:"telegram"`
}

// TelegramConfig configures the Telegram bot notification channel.
type TelegramConfig struct {
	// BotToken is the Telegram bot token from BotFather.
	BotToken string `yaml:"bot_token" json:"bot_token" sensitive:"true"`

	// ChatID is the target chat or channel ID for digests.
	ChatID int64 `yaml:"chat_id" json:"chat_id"`
}

// ---------------------------------------------------------------------------
// Storage section
// ---------------------------------------------------------------------------

// StorageConfig configures the state store (SQLite ledger).
type StorageConfig struct {
	// StatePath is the filesystem path to the SQLite database file.
	// Default is "./state/emailer.db".
	StatePath string `yaml:"state_path" json:"state_path"`

	// Stateless disables persistence entirely. When true, FetchUnreadOnly
	// must also be true.
	Stateless bool `yaml:"stateless" json:"stateless"`
}

// ---------------------------------------------------------------------------
// Digest section
// ---------------------------------------------------------------------------

// DigestConfig configures the digest rendering behaviour.
type DigestConfig struct {
	// MaxMessageExcerpt limits the number of characters per message
	// excerpt in the rendered digest.
	MaxMessageExcerpt int `yaml:"max_message_excerpt" json:"max_message_excerpt"`

	// IncludeReadStatus determines whether the read/unread badge is
	// shown next to each message in the digest.
	IncludeReadStatus bool `yaml:"include_read_status" json:"include_read_status"`
}

// ---------------------------------------------------------------------------
// Labels section
// ---------------------------------------------------------------------------

// LabelsConfig holds user-defined classification labels in addition to
// the built-in defaults (Useful, ToDelete, Ads).
type LabelsConfig struct {
	// Custom contains user-defined classification labels.
	Custom []string `yaml:"custom" json:"custom"`
}

// ---------------------------------------------------------------------------
// Prompts section
// ---------------------------------------------------------------------------

// PromptConfig holds prompt templates used for LLM classification.
type PromptConfig struct {
	// ClassificationPrompt is the main template used to classify each
	// email. Overrides the built-in default when set.
	ClassificationPrompt string `yaml:"classification_prompt" json:"classification_prompt"`

	// SystemPrompt is the system-level instruction for the LLM.
	SystemPrompt string `yaml:"system_prompt" json:"system_prompt"`
}

// ---------------------------------------------------------------------------
// Concurrency section
// ---------------------------------------------------------------------------

// ConcurrencyConfig controls parallelism limits throughout the pipeline.
type ConcurrencyConfig struct {
	// MaxAccounts limits the number of IMAP accounts fetched concurrently.
	MaxAccounts int `yaml:"max_accounts" json:"max_accounts"`

	// MaxLLMCalls limits the number of simultaneous LLM provider calls.
	MaxLLMCalls int `yaml:"max_llm_calls" json:"max_llm_calls"`

	// FetchBatchSize limits how many UIDs are fetched per IMAP UID FETCH
	// command. Larger batches use fewer round-trips; smaller batches bound
	// memory and per-command duration. 0 falls back to the default (10).
	FetchBatchSize int `yaml:"fetch_batch_size" json:"fetch_batch_size"`
}
