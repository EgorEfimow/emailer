package config

import "time"

// DefaultConfig returns a Config populated with sensible defaults.
//
// Default choices:
//   - FetchUnreadOnly: false (fetch all messages in the time window)
//   - MaxWindow: 72h (prevent overwhelming the LLM after host downtime)
//   - IMAP port: 993 (IMAPS), TLS enabled, folder: INBOX
//   - LLM timeout: 120s, retries: 3, max concurrency: 4
//   - Storage: ./state/emailer.db, stateful
//   - Digest excerpt: 500 chars, read status shown
//   - Classification labels: Useful, ToDelete, Ads
//   - Concurrency: 4 accounts, 4 LLM calls
func DefaultConfig() Config {
	return Config{
		FetchUnreadOnly: false,
		MaxWindow:       72 * time.Hour,

		LLM: LLMConfig{
			Provider:                "",
			APIKey:                  "",
			Model:                   "",
			Endpoint:                "",
			Timeout:                 120 * time.Second,
			MaxRetries:              3,
			MaxConcurrent:           4,
			AnalysisRepairMaxAttempts: 1,
		},

		IMAP: IMAPConfig{
			Accounts: nil,
			Timeout:  30 * time.Second,
		},

		Notify: NotifyConfig{
			Telegram: TelegramConfig{
				BotToken: "",
				ChatID:   0,
			},
		},

		Storage: StorageConfig{
			StatePath: "./state/emailer.db",
			Stateless: false,
		},

		Digest: DigestConfig{
			MaxMessageExcerpt:       500,
			IncludeReadStatus:       true,
			IncludeGlobalStats:      true,
			IncludeAccountStats:     true,
			IncludeSummaries:        true,
			IncludeKeyPoints:        true,
			IncludeActionItems:      true,
			IncludeRawExcerptFallback: true,
			MaxMessages:             100,
			MaxKeyPointsPerMessage:  5,
			MaxActionItemsPerMessage: 3,
			PriorityOnly:            false,
		},

		Labels: LabelsConfig{
			Custom: nil,
		},

		Prompts: PromptConfig{
			ClassificationPrompt: "",
			SystemPrompt:         "",
		},

		Concurrency: ConcurrencyConfig{
			MaxAccounts:    4,
			MaxLLMCalls:    4,
			FetchBatchSize: 10,
		},
	}
}
