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
	llmAnalysisRepairMaxAttempts := fs.Int("llm-analysis-repair-max-attempts", cfg.LLM.AnalysisRepairMaxAttempts, "")

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
	digestIncludeGlobalStats := fs.Bool("digest-include-global-stats", cfg.Digest.IncludeGlobalStats, "")
	digestIncludeAccountStats := fs.Bool("digest-include-account-stats", cfg.Digest.IncludeAccountStats, "")
	digestIncludeSummaries := fs.Bool("digest-include-summaries", cfg.Digest.IncludeSummaries, "")
	digestIncludeKeyPoints := fs.Bool("digest-include-key-points", cfg.Digest.IncludeKeyPoints, "")
	digestIncludeActionItems := fs.Bool("digest-include-action-items", cfg.Digest.IncludeActionItems, "")
	digestIncludeRawExcerptFallback := fs.Bool("digest-include-raw-excerpt-fallback", cfg.Digest.IncludeRawExcerptFallback, "")
	digestMaxMessages := fs.Int("digest-max-messages", cfg.Digest.MaxMessages, "")
	digestMaxKeyPointsPerMessage := fs.Int("digest-max-key-points-per-message", cfg.Digest.MaxKeyPointsPerMessage, "")
	digestMaxActionItemsPerMessage := fs.Int("digest-max-action-items-per-message", cfg.Digest.MaxActionItemsPerMessage, "")
	digestPriorityOnly := fs.Bool("digest-priority-only", cfg.Digest.PriorityOnly, "")

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
	cfg.LLM.AnalysisRepairMaxAttempts = *llmAnalysisRepairMaxAttempts

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
		acct.normalize()
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
	cfg.Digest.IncludeGlobalStats = *digestIncludeGlobalStats
	cfg.Digest.IncludeAccountStats = *digestIncludeAccountStats
	cfg.Digest.IncludeSummaries = *digestIncludeSummaries
	cfg.Digest.IncludeKeyPoints = *digestIncludeKeyPoints
	cfg.Digest.IncludeActionItems = *digestIncludeActionItems
	cfg.Digest.IncludeRawExcerptFallback = *digestIncludeRawExcerptFallback
	cfg.Digest.MaxMessages = *digestMaxMessages
	cfg.Digest.MaxKeyPointsPerMessage = *digestMaxKeyPointsPerMessage
	cfg.Digest.MaxActionItemsPerMessage = *digestMaxActionItemsPerMessage
	cfg.Digest.PriorityOnly = *digestPriorityOnly

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
