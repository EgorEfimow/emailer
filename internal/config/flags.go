package config

import (
	"flag"
	"fmt"
	"strings"
)

// loadFlags overrides cfg fields from CLI flags parsed from args.
// If args is nil, os.Args[1:] is used.
func loadFlags(args []string, cfg *Config) error {
	fs := flag.NewFlagSet("emailer", flag.ContinueOnError)

	// ── Top-level ────────────────────────────────────────────────────
	fetchUnreadOnly := fs.Bool("fetch-unread-only", cfg.FetchUnreadOnly, "")
	maxWindow := fs.Duration("max-window", cfg.MaxWindow, "")

	// ── LLM ──────────────────────────────────────────────────────────
	llmProvider := fs.String("llm-provider", cfg.LLM.Provider, "")
	llmAPIKey := fs.String("llm-api-key", cfg.LLM.APIKey, "")
	llmModel := fs.String("llm-model", cfg.LLM.Model, "")
	llmEndpoint := fs.String("llm-endpoint", cfg.LLM.Endpoint, "")
	llmTimeout := fs.Duration("llm-timeout", cfg.LLM.Timeout, "")
	llmMaxRetries := fs.Int("llm-max-retries", cfg.LLM.MaxRetries, "")
	llmMaxConcurrent := fs.Int("llm-max-concurrent", cfg.LLM.MaxConcurrent, "")

	// ── IMAP single account ─────────────────────────────────────────
	imapHost := fs.String("imap-host", "", "")
	imapLabel := fs.String("imap-label", "", "")
	imapPort := fs.Int("imap-port", 0, "")
	imapUsername := fs.String("imap-username", "", "")
	imapPassword := fs.String("imap-password", "", "")
	imapFolders := fs.String("imap-folders", "", "")
	imapUseTLS := fs.Bool("imap-use-tls", false, "")

	// ── Notify / Telegram ───────────────────────────────────────────
	telegramBotToken := fs.String("telegram-bot-token", cfg.Notify.Telegram.BotToken, "")
	telegramChatID := fs.Int64("telegram-chat-id", cfg.Notify.Telegram.ChatID, "")

	// ── Storage ─────────────────────────────────────────────────────
	statePath := fs.String("state-path", cfg.Storage.StatePath, "")
	stateless := fs.Bool("stateless", cfg.Storage.Stateless, "")

	// ── Digest ──────────────────────────────────────────────────────
	digestMaxExcerpt := fs.Int("digest-max-message-excerpt", cfg.Digest.MaxMessageExcerpt, "")
	digestIncludeRead := fs.Bool("digest-include-read-status", cfg.Digest.IncludeReadStatus, "")

	// ── Labels ──────────────────────────────────────────────────────
	labelsCustom := fs.String("labels-custom", "", "")

	// ── Concurrency ─────────────────────────────────────────────────
	concurrencyMaxAccounts := fs.Int("concurrency-max-accounts", cfg.Concurrency.MaxAccounts, "")
	concurrencyMaxLLMCalls := fs.Int("concurrency-max-llm-calls", cfg.Concurrency.MaxLLMCalls, "")

	// ── Parse ───────────────────────────────────────────────────────
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("config.loadFlags: %w", err)
	}

	// ── Apply ───────────────────────────────────────────────────────

	// Top-level
	cfg.FetchUnreadOnly = *fetchUnreadOnly
	cfg.MaxWindow = *maxWindow

	// LLM
	cfg.LLM.Provider = *llmProvider
	cfg.LLM.APIKey = *llmAPIKey
	cfg.LLM.Model = *llmModel
	cfg.LLM.Endpoint = *llmEndpoint
	cfg.LLM.Timeout = *llmTimeout
	cfg.LLM.MaxRetries = *llmMaxRetries
	cfg.LLM.MaxConcurrent = *llmMaxConcurrent

	// IMAP single account (only if host is set)
	if *imapHost != "" {
		acct := IMAPAccount{
			Host:     *imapHost,
			Label:    *imapLabel,
			Port:     *imapPort,
			Username: *imapUsername,
			Password: *imapPassword,
			UseTLS:   *imapUseTLS,
		}
		if *imapFolders != "" {
			acct.Folders = splitComma(*imapFolders)
		}
		cfg.IMAP.Accounts = append(cfg.IMAP.Accounts, acct)
	}

	// Notify / Telegram
	cfg.Notify.Telegram.BotToken = *telegramBotToken
	cfg.Notify.Telegram.ChatID = *telegramChatID

	// Storage
	cfg.Storage.StatePath = *statePath
	cfg.Storage.Stateless = *stateless

	// Digest
	cfg.Digest.MaxMessageExcerpt = *digestMaxExcerpt
	cfg.Digest.IncludeReadStatus = *digestIncludeRead

	// Labels
	if *labelsCustom != "" {
		cfg.Labels.Custom = splitComma(*labelsCustom)
	}

	// Concurrency
	cfg.Concurrency.MaxAccounts = *concurrencyMaxAccounts
	cfg.Concurrency.MaxLLMCalls = *concurrencyMaxLLMCalls

	return nil
}

// splitComma splits a comma-separated string, trimming whitespace and
// skipping empty entries.
func splitComma(s string) []string {
	parts := strings.Split(s, ",")
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	return cleaned
}
