// Package main is the CLI entrypoint for the email AI agent. It wires together
// all packages from Phases 1–19 into a runnable binary.
//
// Per AGENTS.md §9: no business logic lives here — only dependency wiring.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/egorefimow/emailer/internal/config"
	"github.com/egorefimow/emailer/internal/digest"
	"github.com/egorefimow/emailer/internal/llm"
	"github.com/egorefimow/emailer/internal/llm/gemini"
	"github.com/egorefimow/emailer/internal/llm/mistral"
	"github.com/egorefimow/emailer/internal/llm/ollama"
	"github.com/egorefimow/emailer/internal/llm/openrouter"
	"github.com/egorefimow/emailer/internal/log"
	"github.com/egorefimow/emailer/internal/mail"
	"github.com/egorefimow/emailer/internal/notify"
	"github.com/egorefimow/emailer/internal/notify/telegram"
	"github.com/egorefimow/emailer/internal/orchestrator"
	"github.com/egorefimow/emailer/internal/shutdown"
	"github.com/egorefimow/emailer/internal/store"
)

// main is the real entrypoint. It returns an exit code so deferred cleanup
// runs before os.Exit.
func main() {
	os.Exit(run())
}

// run implements the full CLI lifecycle: parse flags → load config → set up
// dependencies → run orchestrator → map result to exit code.
func run() int { //nolint:gocyclo
	// -----------------------------------------------------------------------
	// Step 1: Parse main.go-only flags manually
	// -----------------------------------------------------------------------
	// These flags are consumed here and not passed to config.Load, avoiding
	// duplication with the flags defined in config.loadFlags.
	cfgPath, logLevel, dryRun, forceReprocess, window, filteredArgs, showHelp := parseMainFlags()
	if showHelp {
		printUsage()
		return 2
	}

	// Default log level
	if logLevel == "" {
		logLevel = "info"
	}

	// -----------------------------------------------------------------------
	// Step 2: Load configuration from file, env, and remaining CLI flags
	// -----------------------------------------------------------------------
	cfg, err := config.Load(config.LoadOptions{
		ConfigPath: cfgPath,
		Args:       filteredArgs,
	})
	if err != nil {
		// If the error is from flag parsing (e.g. unknown flag), print usage.
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}

	// -----------------------------------------------------------------------
	// Step 3: Validate configuration
	// -----------------------------------------------------------------------
	if err := config.Validate(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		return 2
	}

	// -----------------------------------------------------------------------
	// Step 4: Create structured logger with secret redaction
	// -----------------------------------------------------------------------
	logger, err := log.NewLogger(os.Stdout, logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger setup error: %v\n", err)
		return 2
	}

	secretPatterns := config.SecretRedactionPatterns(cfg)
	logger = log.WithSecretRedaction(logger, secretPatterns)
	logger.Info("logger initialized",
		slog.String("log_level", logLevel),
		slog.Bool("dry_run", dryRun),
		slog.Bool("force_reprocess", forceReprocess),
	)

	// -----------------------------------------------------------------------
	// Step 5: Create signal-aware context
	// -----------------------------------------------------------------------
	ctx, cancel := shutdown.ContextWithSignal(context.Background())
	defer cancel()

	// -----------------------------------------------------------------------
	// Step 6: Open the state store
	// -----------------------------------------------------------------------
	var st store.Store
	if cfg.Storage.Stateless {
		st = store.NewNoopStore()
		logger.Info("store: noop (stateless mode)")
	} else {
		s, err := store.NewSQLiteStore(ctx, cfg.Storage.StatePath)
		if err != nil {
			logger.Error("store: failed to open SQLite store", slog.Any("error", err))
			fmt.Fprintf(os.Stderr, "store error: %v\n", err)
			return 1
		}
		st = s
		logger.Info("store: SQLite opened", slog.String("path", cfg.Storage.StatePath))
	}
	defer func() {
		if err := st.Close(); err != nil {
			logger.Error("store: close error", slog.Any("error", err))
		}
	}()

	// -----------------------------------------------------------------------
	// Step 7: Dial IMAP ingesters
	// -----------------------------------------------------------------------
	ingesters := make(map[string]mail.Ingester, len(cfg.IMAP.Accounts))
	var imapClients []*mail.IMAPClient

	for _, acct := range cfg.IMAP.Accounts {
		cli, err := mail.NewIMAPClient(logger, cfg.IMAP.Timeout)
		if err != nil {
			logger.Error("imap: failed to create client", slog.Any("error", err))
			fmt.Fprintf(os.Stderr, "imap client error: %v\n", err)
			return 1
		}

		if err := cli.Dial(ctx, acct); err != nil {
			logger.Error("imap: dial failed",
				slog.String("account", acct.Label),
				slog.String("host", acct.Host),
				slog.Any("error", err),
			)
			// Close any clients that were already dialled.
			for _, c := range imapClients {
				if err := c.Close(); err != nil {
					logger.Error("imap: close error during dial failure cleanup", slog.Any("error", err))
				}
			}
			fmt.Fprintf(os.Stderr, "imap dial error for %s (%s): %v\n", acct.Label, acct.Host, err)
			return 1
		}

		if err := cli.Login(ctx, acct); err != nil {
			logger.Error("imap: login failed",
				slog.String("account", acct.Label),
				slog.String("host", acct.Host),
				slog.Any("error", err),
			)
			if err := cli.Close(); err != nil {
				logger.Error("imap: close error after login failure", slog.Any("error", err))
			}
			for _, c := range imapClients {
				if err := c.Close(); err != nil {
					logger.Error("imap: close error during login failure cleanup", slog.Any("error", err))
				}
			}
			fmt.Fprintf(os.Stderr, "imap login error for %s (%s): %v\n", acct.Label, acct.Host, err)
			return 1
		}

		imapClients = append(imapClients, cli)
		ingesters[acct.Label] = cli
		logger.Info("imap: connected",
			slog.String("account", acct.Label),
			slog.String("host", acct.Host),
		)
	}

	// Defer close all IMAP connections (LIFO order).
	defer func() {
		for _, cli := range imapClients {
			if err := cli.Close(); err != nil {
				logger.Error("imap: close error", slog.Any("error", err))
			}
		}
	}()

	// -----------------------------------------------------------------------
	// Step 8: Create LLM provider via registry
	// -----------------------------------------------------------------------
	providerRegistry := llm.NewProviderRegistry()
	providerRegistry.Register("gemini", gemini.Factory)
	providerRegistry.Register("mistral", mistral.Factory)
	providerRegistry.Register("ollama", ollama.Factory)
	providerRegistry.Register("openrouter", openrouter.Factory)

	providerFactory := providerRegistry.Lookup(cfg.LLM.Provider)
	if providerFactory == nil {
		logger.Error("llm: provider not found",
			slog.String("provider", cfg.LLM.Provider),
			slog.String("registered", strings.Join(providerRegistry.Registered(), ", ")),
		)
		fmt.Fprintf(os.Stderr, "llm provider %q not found (registered: %s)\n",
			cfg.LLM.Provider, strings.Join(providerRegistry.Registered(), ", "))
		return 1
	}

	provider, err := providerFactory(ctx, cfg.LLM.APIKey, cfg.LLM.Endpoint, cfg.LLM.Model)
	if err != nil {
		logger.Error("llm: provider creation failed",
			slog.String("provider", cfg.LLM.Provider),
			slog.Any("error", err),
		)
		fmt.Fprintf(os.Stderr, "llm provider error: %v\n", err)
		return 1
	}
	logger.Info("llm: provider created",
		slog.String("provider", provider.Name()),
		slog.String("model", cfg.LLM.Model),
	)

	// -----------------------------------------------------------------------
	// Step 9: Create digest renderers
	// -----------------------------------------------------------------------
	renderer := digest.NewMarkdownRenderer(cfg.Digest)
	fallbackRenderer := digest.NewFallbackRenderer(cfg.Digest)
	logger.Info("renderers created",
		slog.String("primary", renderer.Name()),
		slog.String("fallback", fallbackRenderer.Name()),
	)

	// -----------------------------------------------------------------------
	// Step 10: Create notification channel via registry
	// -----------------------------------------------------------------------
	channelRegistry := notify.NewChannelRegistry()
	channelRegistry.Register("telegram", telegram.Factory)

	channelFactory := channelRegistry.Lookup("telegram")
	if channelFactory == nil {
		logger.Error("notify: telegram channel factory not found")
		fmt.Fprintf(os.Stderr, "notify: telegram channel factory not found\n")
		return 1
	}

	channel, err := channelFactory(ctx, map[string]any{
		"bot_token": cfg.Notify.Telegram.BotToken,
		"chat_id":   cfg.Notify.Telegram.ChatID,
	})
	if err != nil {
		logger.Error("notify: channel creation failed", slog.Any("error", err))
		fmt.Fprintf(os.Stderr, "notify channel error: %v\n", err)
		return 1
	}
	logger.Info("notify: channel created", slog.String("channel", channel.Name()))

	// -----------------------------------------------------------------------
	// Step 11: Create orchestrator and run
	// -----------------------------------------------------------------------
	pipeline := orchestrator.New(st, ingesters, provider, renderer, fallbackRenderer, channel, logger, cfg)

	opts := orchestrator.RunOptions{
		ForceReprocess: forceReprocess,
		DryRun:         dryRun,
		Stateless:      cfg.Storage.Stateless,
	}
	if window != nil {
		opts.Window = window
	}

	result := pipeline.Run(ctx, opts)

	// -----------------------------------------------------------------------
	// Step 12: Log result and map to exit code
	// -----------------------------------------------------------------------
	logger.Info("run complete",
		slog.String("run_id", result.RunID),
		slog.String("status", string(result.Status)),
		slog.Int("total_fetched", result.TotalFetched),
		slog.Int("total_classified", result.TotalClassified),
		slog.Int("failed", result.FailedCount),
	)

	if result.Err != nil {
		logger.Error("run error", slog.Any("error", result.Err))
	}

	switch result.Status {
	case store.RunStatusCompleted, store.RunStatusDegraded, store.RunStatusPartial, store.RunStatusPartiallyClassified:
		return 0

	case store.RunStatusIngestFailed:
		return 1

	case store.RunStatusCancelled:
		return 130

	default:
		// Unknown status — treat as fatal.
		return 1
	}
}

// ---------------------------------------------------------------------------
// Flag parsing helpers
// ---------------------------------------------------------------------------

// parseMainFlags manually extracts the 5 main.go-only flags from os.Args,
// returning the filtered args slice for config.Load. It also detects --help.
func parseMainFlags() (cfgPath, logLevel string, dryRun, forceReprocess bool, window *time.Duration, filteredArgs []string, showHelp bool) { //nolint:gocyclo
	args := os.Args[1:]
	filtered := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]

		// --help
		if arg == "--help" || arg == "-h" {
			return "", "", false, false, nil, nil, true
		}

		// --config <path> or --config=<path>
		if arg == "--config" || arg == "-config" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				cfgPath = args[i+1]
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			cfgPath = strings.TrimPrefix(arg, "--config=")
			continue
		}

		// --log-level <level> or --log-level=<level>
		if arg == "--log-level" || arg == "-log-level" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				logLevel = args[i+1]
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--log-level=") {
			logLevel = strings.TrimPrefix(arg, "--log-level=")
			continue
		}

		// --dry-run
		if arg == "--dry-run" || arg == "-dry-run" {
			dryRun = true
			continue
		}

		// --force-reprocess
		if arg == "--force-reprocess" || arg == "-force-reprocess" {
			forceReprocess = true
			continue
		}

		// --window <duration> or --window=<duration>
		if arg == "--window" || arg == "-window" {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				d, err := time.ParseDuration(args[i+1])
				if err == nil {
					window = &d
				}
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--window=") {
			d, err := time.ParseDuration(strings.TrimPrefix(arg, "--window="))
			if err == nil {
				window = &d
			}
			continue
		}

		// Not a main.go flag — pass through to config.Load.
		filtered = append(filtered, arg)
	}

	return cfgPath, logLevel, dryRun, forceReprocess, window, filtered, false
}

// printUsage writes the usage text to stderr.
func printUsage() {
	_, _ = fmt.Fprintf(os.Stderr, `Usage: emailer [options]

A one-shot email AI agent that ingests IMAP mail, classifies it via an LLM,
applies keyword flags, and delivers a digest to Telegram.

Main flags (parsed before config loading):
  --config <path>         Path to YAML or JSON config file
  --log-level <level>     Log level: debug, info, warn, error (default: info)
  --dry-run               Skip side effects (flag writes, notifications)
  --force-reprocess       Reprocess all messages, ignoring prior runs
  --window <duration>     Explicit fetch window (e.g. 24h, 72h)

Config flags (passed to config.Load, see --help on those):
  --llm-provider, --llm-api-key, --llm-model, --llm-endpoint, ...
  --imap-host, --imap-label, --imap-port, --imap-username, ...
  --telegram-bot-token, --telegram-chat-id, ...
  --state-path, --stateless, --fetch-unread-only, --max-window, ...

Exit codes:
  0   Success (completed, degraded, partial)
  1   Fatal error (ingest failed, config error, store error)
  2   CLI flag / config validation error
  130 Cancelled (SIGINT/SIGTERM)
`)
	_ = os.Stderr.Sync() //nolint:errcheck
}
