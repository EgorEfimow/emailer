# Planning

This document is the canonical, step-by-step, smallest-possible-increment
plan for building the Email AI Agent from scratch. Every step is a single,
testable, mergeable change. No step depends on a later step. No step
combines two concerns.

Legend:
- `[ ]` not started
- `[x]` done (update as we go)

## Phase 0 — Repository Foundation

- [ ] 0.1 Initialize Go module `github.com/<org>/emailer` with Go 1.25.
- [ ] 0.2 Create directory tree from `architecture.md` §4 (empty `.keep` files).
- [ ] 0.3 Add `LICENSE` (MIT).
- [ ] 0.4 Add `.gitignore` (Go, IDE, OS, `.env`, `state/`).
- [ ] 0.5 Add `.editorconfig`.
- [ ] 0.6 Add `Makefile` with `build`, `test`, `lint`, `fmt`, `tidy`, `clean`, `docker`, `run`, `server` targets.
- [ ] 0.7 Add `.golangci.yml` with `errcheck`, `govet`, `staticcheck`, `gocyclo`, `misspell`.
- [ ] 0.8 Add `renovate.json` for dependency automation.
- [ ] 0.9 Add `SECURITY.md` with vulnerability reporting policy.
- [ ] 0.10 Add GitHub Actions workflow `ci.yml`: `lint`, `test`, `build` on push and PR.
- [ ] 0.11 Add GitHub Actions workflow `release.yml` on tag.

## Phase 1 — Logging Foundation

- [ ] 1.1 Create `internal/log` package.
- [ ] 1.2 Implement `NewLogger(w io.Writer, level string, opts ...Option) (*slog.Logger, error)`.
- [ ] 1.3 Add `WithRunID(logger, runID)` helper.
- [ ] 1.4 Add `WithSecretRedaction(logger, patterns []regexp.Regexp)` helper.
- [ ] 1.5 Define `Sensitive` type wrapper for `slog.LogValuer`.
- [ ] 1.6 Add unit tests for level parsing.
- [ ] 1.7 Add unit tests for secret redaction.
- [ ] 1.8 Add unit tests for run-id injection.

## Phase 2 — Configuration Core

- [ ] 2.1 Create `internal/config` package.
- [ ] 2.2 Define `Config` struct with all sections from `architecture.md` §5.1 as nested structs.
- [ ] 2.3 Tag every secret field with `sensitive:"true"`.
- [ ] 2.4 Implement `defaults.go` with all default values.
- [ ] 2.5 Implement env loader `loadEnv(cfg *Config) error` using `os.LookupEnv`.
- [ ] 2.6 Implement YAML loader `loadYAML(path string, cfg *Config) error` using `gopkg.in/yaml.v3`.
- [ ] 2.7 Implement JSON loader `loadJSON(path string, cfg *Config) error`.
- [ ] 2.8 Implement CLI flag loader using `flag` package.
- [ ] 2.9 Implement `Load(opts LoadOptions) (Config, error)` that applies sources in precedence order.
- [ ] 2.10 Implement `Validate(cfg Config) error` with full validation.
- [ ] 2.11 Add `IMAPAccount` validation: label, host, port range, username, password non-empty; folder list non-empty.
- [ ] 2.12 Add `LLMConfig` validation: provider in registry; model non-empty; API key required iff provider requires it.
- [ ] 2.13 Add `NotifyConfig` validation: at least one channel enabled.
- [ ] 2.14 Add `StorageConfig` validation: path writable if stateful.
- [ ] 2.15 Add `ScheduleConfig` validation: cron expression parseable.
- [ ] 2.16 Add `LabelsConfig` validation: at least one label; labels match `^[A-Za-z][A-Za-z0-9_-]{0,30}$`.
- [ ] 2.17 Add `PromptConfig` validation: template parses.
- [ ] 2.18 Add `ConcurrencyConfig` validation: positive integers.
- [ ] 2.19 Add `SecretRedactionPatterns(cfg Config) []regexp.Regexp` deriving patterns from sensitive fields.
- [ ] 2.20 Add `FetchUnreadOnly` boolean to Config (default: `false`).
- [ ] 2.21 Add unit tests: defaults.
- [ ] 2.22 Add unit tests: env override.
- [ ] 2.23 Add unit tests: YAML override.
- [ ] 2.24 Add unit tests: JSON override.
- [ ] 2.25 Add unit tests: precedence ordering.
- [ ] 2.26 Add unit tests: every validation rule.
- [ ] 2.27 Add unit tests: missing required fields.
- [ ] 2.28 Add unit tests: malformed YAML/JSON.
- [ ] 2.29 Add unit tests: sensitive field redaction.
- [ ] 2.30 Add unit tests for `FetchUnreadOnly` validation and defaulting.
- [ ] 2.31 Add `.env.example` covering every variable.
- [ ] 2.32 Add `config.example.yaml`.

## Phase 3 — Shutdown and Context

- [ ] 3.1 Create `internal/shutdown` package.
- [ ] 3.2 Implement `ContextWithSignal(parent context.Context, signals ...os.Signal) (context.Context, func())`.
- [ ] 3.3 Implement `WaitForDrain(ctx, timeout, wg)` helper.
- [ ] 3.4 Add unit tests with fake signals.

## Phase 4 — Metrics and Tracing

- [ ] 4.1 Create `internal/metrics` package.
- [ ] 4.2 Define Prometheus collectors: `runs_total`, `run_duration_seconds`, `messages_fetched_total`, `llm_calls_total`, `llm_latency_seconds`, `flags_applied_total`, `notify_send_total`, `errors_total`.
- [ ] 4.3 Implement `Register(prom *prometheus.Registry)`.
- [ ] 4.4 Create `internal/trace` package.
- [ ] 4.5 Implement OTLP exporter setup.
- [ ] 4.6 Implement `StartSpan(ctx, name) (context.Context, func())`.
- [ ] 4.7 Add unit tests for metric registration.
- [ ] 4.8 Add unit tests for span lifecycle.

## Phase 5 — State Store

- [ ] 5.1 Create `internal/store` package.
- [ ] 5.2 Define `Store` interface: `RecordRun`, `FinishRun`, `RecordMessage`, `AlreadyProcessed`, `RecordFlag`, `RecordDigest`, `GetRun`, `ListRuns`.
- [ ] 5.3 Define `Run`, `ProcessedMessage`, `FlagRecord`, `DigestRecord` structs.
- [ ] 5.4 Implement `SQLiteStore` using `modernc.org/sqlite` (pure-Go, no CGO).
- [ ] 5.5 Add migrations directory `internal/store/migrations`.
- [ ] 5.6 Migration 0001: `runs` table.
- [ ] 5.7 Migration 0002: `processed_messages` table with composite index on `(account_label, uid)`.
- [ ] 5.8 Migration 0003: `flags_applied` table.
- [ ] 5.9 Migration 0004: `digests` table.
- [ ] 5.10 Migration 0005: `provider_disagreements` table (for ensemble mode).
- [ ] 5.11 Implement migration runner via `golang-migrate`.
- [ ] 5.12 Implement `NewSQLiteStore(path string, logger) (*SQLiteStore, error)`.
- [ ] 5.13 Implement `Close()`.
- [ ] 5.14 Implement `RecordRun`.
- [ ] 5.15 Implement `FinishRun`.
- [ ] 5.16 Implement `RecordMessage`.
- [ ] 5.17 Implement `AlreadyProcessed` (batch lookup).
- [ ] 5.18 Implement `RecordFlag`.
- [ ] 5.19 Implement `RecordDigest`.
- [ ] 5.20 Implement `GetRun`.
- [ ] 5.21 Implement `ListRuns`.
- [ ] 5.22 Implement in-memory `NoopStore` for stateless mode.
- [ ] 5.23 Add unit tests per method using in-memory DB.
- [ ] 5.24 Add integration tests with `testcontainers` SQLite.
- [ ] 5.25 Add tests for `AlreadyProcessed` correctness under concurrent runs.
- [ ] 5.26 Add documentation in `README.md` warning that `FetchUnreadOnly=false` requires a persistent state store.

## Phase 6 — Mail Models

- [ ] 6.1 Create `internal/mail` package.
- [ ] 6.2 Define `Message` struct: `AccountLabel`, `UID`, `Folder`, `MessageID`, `From`, `To`, `Subject`, `Date`, `Body`, `IsRead`, `AttachmentMetas`.
- [ ] 6.3 Define `AttachmentMeta` struct: `Filename`, `MIME`, `Size`.
- [ ] 6.4 Define `Classification` type as string.
- [ ] 6.5 Define `MessageKey` type as `(AccountLabel, UID)` composite.
- [ ] 6.6 Define `Flag` type as string (no backslash).
- [ ] 6.7 Define `ClassificationToFlag(c Classification, cfg LabelsConfig) Flag` mapping.
- [ ] 6.8 Add unit tests for mapping including custom labels.
- [ ] 6.9 Add unit tests for `MessageKey` equality and map behavior.

## Phase 7 — Mail Sanitization

- [ ] 7.1 Create `internal/mail/sanitize.go`.
- [ ] 7.2 Implement `StripHTML(s string) string` with state machine, not regex.
- [ ] 7.3 Implement `StripControlChars(s string) string`.
- [ ] 7.4 Implement `DecodeEntities(s string) string`.
- [ ] 7.5 Implement `ConvertCharset(r io.Reader, contentType string) (string, error)` using `golang.org/x/text/encoding`.
- [ ] 7.6 Implement `Truncate(s string, limit int) string` rune-aware.
- [ ] 7.7 Add unit tests for HTML stripping edge cases.
- [ ] 7.8 Add unit tests for charset conversion (ISO-8859-1, KOI8-R, Shift-JIS).
- [ ] 7.9 Add unit tests for entity decoding.
- [ ] 7.10 Add fuzz test for `StripHTML`.

## Phase 8 — Mail IMAP Adapter

- [ ] 8.1 Add dependency `github.com/emersion/go-imap/v15` and `github.com/emersion/go-message`.
- [ ] 8.2 Define `Ingester` interface: `Fetch`, `ApplyFlags`, `Close`.
- [ ] 8.3 Define `FetchOptions`: `Since`, `Folders`, `Limit`, `IncludeAttachments`, `UnreadOnly`.
- [ ] 8.4 Implement `IMAPClient` struct wrapping `*client.Client`.
- [ ] 8.5 Implement `dial(ctx, account)` with TLS, STARTTLS, plaintext options.
- [ ] 8.6 Implement `login(ctx, account)` using app passwords (no OAuth2).
- [ ] 8.7 Implement `selectFolder(ctx, folder)`.
- [ ] 8.8 Implement `searchByWindow(ctx, since, unreadOnly)` returning UIDs.
- [ ] 8.9 Implement `sortByDate(uids)` client-side fallback.
- [ ] 8.10 Implement `fetchHeaders(ctx, uidset)` returning envelopes and `\Seen` flag status.
- [ ] 8.11 Implement `fetchBody(ctx, uidset)` returning body sections.
- [ ] 8.12 Implement `readBody(part io.Reader, contentType string) (string, []AttachmentMeta, error)`.
- [ ] 8.13 Implement `applyFlags(ctx, uidset, flags)` using `UID STORE` with plain keywords.
- [ ] 8.14 Implement `checkPermanentFlags(ctx)` to verify custom keywords allowed.
- [ ] 8.15 Implement `Close()` with logout.
- [ ] 8.16 Add unit tests for `readBody` with multipart fixtures.
- [ ] 8.17 Add unit tests for `sortByDate`.
- [ ] 8.18 Add unit tests for `applyFlags` flag mapping.
- [ ] 8.19 Add unit tests for IMAP search criteria generation (verify `UNSEEN` flag is conditionally added).
- [ ] 8.20 Add integration test with `go-imap` mock server verifying "all emails" fetch returns read and unread messages.

## Phase 9 — Mail Concurrency

- [ ] 9.1 Create `internal/mail/pool.go`.
- [ ] 9.2 Implement `FetchAll(ctx, accounts []IMAPAccount, opts FetchOptions, concurrency int) (map[string][]Message, map[string]error)`.
- [ ] 9.3 Use `errgroup` with bounded semaphore.
- [ ] 9.4 Add unit tests with fake `Ingester` implementations.
- [ ] 9.5 Add test: one account fails, others succeed.
- [ ] 9.6 Add test: context cancelled mid-fetch.

## Phase 10 — LLM Models

- [ ] 10.1 Create `internal/llm` package.
- [ ] 10.2 Define `Request` struct: `Messages []InputMessage`, `Labels []Classification`, `PromptTemplate string`, `BudgetTokens int`.
- [ ] 10.3 Define `InputMessage` struct: `Key MessageKey`, `From`, `Subject`, `Date`, `IsRead`, `Body`.
- [ ] 10.4 Define `Response` struct: `Digest string`, `Classifications map[MessageKey]Classification`, `TokensUsed int`, `ProviderMeta`.
- [ ] 10.5 Define `Provider` interface: `Name()`, `Classify(ctx, Request) (Response, error)`, `SupportsStreaming() bool`, `Stream(ctx, Request, chan<- Token) error`.
- [ ] 10.6 Define `Token` struct for streaming.
- [ ] 10.7 Define `ProviderRegistry` type.
- [ ] 10.8 Implement `Register(name, factory)`.
- [ ] 10.9 Implement `Lookup(name) (Factory, error)`.
- [ ] 10.10 Add unit tests for registry behavior.

## Phase 11 — Prompt Engineering

- [ ] 11.1 Create `internal/llm/prompt.go`.
- [ ] 11.2 Define `BuildPrompt(req Request) (string, error)`.
- [ ] 11.3 Wrap each email in delimiters: `<email uid="..." account="..."> ... </email>`.
- [ ] 11.4 Place instructions above and below the data block, including email metadata (Date, Read/Unread status).
- [ ] 11.5 Include explicit JSON schema in the prompt.
- [ ] 11.6 Include label definitions from `LabelsConfig`.
- [ ] 11.7 Make template configurable via `PromptConfig.Template`.
- [ ] 11.8 Use `text/template` for rendering.
- [ ] 11.9 Add unit tests for prompt structure.
- [ ] 11.10 Add unit tests for template injection (template values are escaped).
- [ ] 11.11 Add fuzz test for `BuildPrompt`.

## Phase 12 — Response Parsing

- [ ] 12.1 Create `internal/llm/parse.go`.
- [ ] 12.2 Implement `ParseResponse(raw string, labels []Classification) (Response, error)`.
- [ ] 12.3 Strip markdown code fences robustly (multiple fences, partial fences).
- [ ] 12.4 Validate JSON against schema using `github.com/xeipuuv/gojsonschema`.
- [ ] 12.5 Parse `classifications` as `map[MessageKey]Classification`.
- [ ] 12.6 Reject unknown classifications.
- [ ] 12.7 Reject duplicate keys.
- [ ] 12.8 Reject missing keys for provided input messages.
- [ ] 12.9 Implement `RepairWithPrompt(raw, err) (string, error)` that asks the model to fix its output.
- [ ] 12.10 Add unit tests for valid response.
- [ ] 12.11 Add unit tests for each rejection case.
- [ ] 12.12 Add unit tests for fence stripping variants.
- [ ] 12.13 Add fuzz test for `ParseResponse`.

## Phase 13 — Token Budgeting

- [ ] 13.1 Create `internal/llm/budget.go`.
- [ ] 13.2 Implement `EstimateTokens(s string) int` using a heuristic (chars/4 for English, chars/2 for CJK).
- [ ] 13.3 Implement `SplitBatch(msgs []InputMessage, budget int) ([][]InputMessage, error)`.
- [ ] 13.4 Add unit tests for split correctness.
- [ ] 13.5 Add unit tests for budget overflow.
- [ ] 13.6 Add unit tests for empty input.

## Phase 14 — Retry and Circuit Breaker

- [ ] 14.1 Create `internal/llm/retry.go`.
- [ ] 14.2 Implement `RetryPolicy` struct: `MaxAttempts`, `BaseDelay`, `MaxDelay`, `Jitter`, `RetryableStatuses`.
- [ ] 14.3 Implement `Do(ctx, fn) error` with jittered exponential backoff.
- [ ] 14.4 Implement `IsRetryable(err) bool`.
- [ ] 14.5 Create `internal/llm/breaker.go`.
- [ ] 14.6 Implement `CircuitBreaker` with closed/open/half-open states.
- [ ] 14.7 Add unit tests for retry success on second attempt.
- [ ] 14.8 Add unit tests for retry exhaustion.
- [ ] 14.9 Add unit tests for non-retryable errors.
- [ ] 14.10 Add unit tests for breaker state transitions.
- [ ] 14.11 Add unit tests for context cancellation during backoff.

## Phase 15 — LLM Gemini Adapter

- [ ] 15.1 Create `internal/llm/gemini` package.
- [ ] 15.2 Implement `Factory(cfg LLMConfig) (Provider, error)`.
- [ ] 15.3 Implement `Classify` using `POST /v1beta/models/{model}:generateContent` with `x-goog-api-key` header (not query string).
- [ ] 15.4 Implement response unmarshaling.
- [ ] 15.5 Implement streaming via `streamGenerateContent`.
- [ ] 15.6 Implement token usage extraction.
- [ ] 15.7 Add HTTP fixture under `testdata/gemini/`.
- [ ] 15.8 Add contract test using `httptest.Server`.
- [ ] 15.9 Add test for API key in header, not URL.
- [ ] 15.10 Add test for retryable status codes.
- [ ] 15.11 Add test for streaming token delivery.

## Phase 16 — LLM OpenAI Adapter (optional)

- [ ] 16.1 Create `internal/llm/openai` package.
- [ ] 16.2 Implement `Factory`.
- [ ] 16.3 Implement `Classify` using `POST /v1/chat/completions` with `Authorization: Bearer`.
- [ ] 16.4 Use `response_format: {"type": "json_object"}`.
- [ ] 16.5 Implement streaming via SSE.
- [ ] 16.6 Add fixtures and contract tests as in Phase 15.

## Phase 17 — LLM Anthropic Adapter (optional)

- [ ] 17.1 Create `internal/llm/anthropic` package.
- [ ] 17.2 Implement `Factory`.
- [ ] 17.3 Implement `Classify` using `POST /v1/messages` with `x-api-key` and `anthropic-version` headers.
- [ ] 17.4 Use `system` prompt for instructions, `user` for data block.
- [ ] 17.5 Implement streaming.
- [ ] 17.6 Add fixtures and contract tests.

## Phase 18 — LLM Ollama Adapter

- [ ] 18.1 Create `internal/llm/ollama` package.
- [ ] 18.2 Implement `Factory`.
- [ ] 18.3 Implement `Classify` using `POST /api/chat` with `format: "json"`.
- [ ] 18.4 Implement streaming.
- [ ] 18.5 Add fixtures and contract tests.

## Phase 19 — LLM OpenRouter Adapter

- [ ] 19.1 Create `internal/llm/openrouter` package.
- [ ] 19.2 Implement `Factory` (OpenAI-compatible with extra headers).
- [ ] 19.3 Add fixtures and contract tests.

## Phase 20 — LLM Mistral Adapter (optional)

- [ ] 20.1 Create `internal/llm/mistral` package.
- [ ] 20.2 Implement `Factory`.
- [ ] 20.3 Add fixtures and contract tests.

## Phase 21 — LLM Ensemble Mode (optional)

- [ ] 21.1 Create `internal/llm/ensemble.go`.
- [ ] 21.2 Implement `EnsembleProvider` that calls N providers concurrently.
- [ ] 21.3 Implement majority-vote classification.
- [ ] 21.4 Persist disagreements to store.
- [ ] 21.5 Add unit tests for voting.
- [ ] 21.6 Add unit tests for tie-breaking.
- [ ] 21.7 Add unit tests for partial provider failure.

## Phase 22 — Security Hardening

- [ ] 22.1 Create `internal/security` package.
- [ ] 22.2 Implement `WrapEmailContent(content string, key MessageKey) string` with delimiters.
- [ ] 22.3 Implement `IsolateInstructions(prompt string) string`.
- [ ] 22.4 Implement `SanitizeOutput(raw string) string` removing embedded instructions.
- [ ] 22.5 Add unit tests for delimiter injection resistance.
- [ ] 22.6 Add unit tests for output sanitization.
- [ ] 22.7 Add fuzz test against injection attempts.

## Phase 23 — Digest Renderers

- [ ] 23.1 Create `internal/digest` package.
- [ ] 23.2 Define `Renderer` interface: `Render(data DigestData) ([]byte, string, error)` returning bytes, MIME type, error.
- [ ] 23.3 Define `DigestData` struct: `RunID`, `StartedAt`, `FinishedAt`, `Messages []ClassifiedMessage`, `Summary string`, `Status`.
- [ ] 23.4 Implement `MarkdownRenderer` using `text/template` (must explicitly render Date and Read/Unread status).
- [ ] 23.5 Implement `HTMLRenderer` using `html/template` (XSS-safe, must explicitly render Date and Read/Unread status).
- [ ] 23.6 Implement `PlainTextRenderer`.
- [ ] 23.7 Implement `FallbackRenderer` for LLM failure (lists messages without classifications).
- [ ] 23.8 Add unit tests per renderer.
- [ ] 23.9 Add unit tests for XSS safety in HTML renderer.

## Phase 24 — Notify Models

- [ ] 24.1 Create `internal/notify` package.
- [ ] 24.2 Define `Channel` interface: `Name()`, `Send(ctx, Payload) error`.
- [ ] 24.3 Define `Payload` struct: `Filename`, `ContentType`, `Bytes`, `Caption`, `Metadata`.
- [ ] 24.4 Define `ChannelRegistry`.
- [ ] 24.5 Implement `Register` and `Lookup`.
- [ ] 24.6 Add unit tests.

## Phase 25 — Notify Telegram Channel

- [ ] 25.1 Create `internal/notify/telegram` package.
- [ ] 25.2 Implement `Factory(cfg TelegramConfig) (Channel, error)`.
- [ ] 25.3 Implement `Send` using `sendDocument` for payloads over 4096 bytes.
- [ ] 25.4 Implement `Send` using `sendMessage` for short payloads.
- [ ] 25.5 Use `Authorization: Bearer` style header where possible; otherwise redact token in logs.
- [ ] 25.6 Implement caption support (1024 char limit).
- [ ] 25.7 Implement file size guard at 45 MB.
- [ ] 25.8 Add HTTP fixture tests.
- [ ] 25.9 Add test for size guard.
- [ ] 25.10 Add test for retryable status codes.

## Phase 26 — Notify Slack Channel

- [ ] 26.1 Create `internal/notify/slack` package.
- [ ] 26.2 Implement `Factory`.
- [ ] 26.3 Implement `Send` using `chat.postMessage` with file attachment via `files.upload`.
- [ ] 26.4 Add fixture tests.

## Phase 27 — Notify Email Channel

- [ ] 27.1 Create `internal/notify/email` package.
- [ ] 27.2 Implement `Factory`.
- [ ] 27.3 Implement `Send` using `net/smtp` with TLS.
- [ ] 27.4 Attach digest as `.md` MIME part.
- [ ] 27.5 Add unit tests with in-memory SMTP server.

## Phase 28 — Notify Webhook Channel

- [ ] 28.1 Create `internal/notify/webhook` package.
- [ ] 28.2 Implement `Factory`.
- [ ] 28.3 Implement `Send` using `POST` with HMAC-SHA256 signature header.
- [ ] 28.4 Add unit tests with `httptest.Server`.

## Phase 29 — Notify File Channel

- [ ] 29.1 Create `internal/notify/file` package.
- [ ] 29.2 Implement `Factory`.
- [ ] 29.3 Implement `Send` writing to configurable directory with timestamped filename.
- [ ] 29.4 Add unit tests with temp directory.

## Phase 30 — Notify Retry

- [ ] 30.1 Create `internal/notify/retry.go`.
- [ ] 30.2 Implement `RetrySender` wrapping `Channel`.
- [ ] 30.3 Use the same `RetryPolicy` shape as LLM.
- [ ] 30.4 Add unit tests.

## Phase 31 — Notify Fan-out

- [ ] 31.1 Create `internal/notify/fanout.go`.
- [ ] 31.2 Implement `FanoutSender` that sends to all configured channels concurrently.
- [ ] 31.3 Collect per-channel errors; never block one channel on another.
- [ ] 31.4 Add unit tests with fake channels.

## Phase 32 — Actions Plugin System

- [ ] 32.1 Create `internal/actions` package.
- [ ] 32.2 Define `Action` interface: `Name()`, `Execute(ctx, ActionContext) error`.
- [ ] 32.3 Define `ActionContext` with message, classification, run ID, logger.
- [ ] 32.4 Implement `Registry`.
- [ ] 32.5 Implement `FlagWriterAction` using `mail.Ingester`.
- [ ] 32.6 Implement `TelegramAlertAction`.
- [ ] 32.7 Implement `ArchiveAction` (move to folder).
- [ ] 32.8 Implement `MoveToFolderAction`.
- [ ] 32.9 Add unit tests per action with fakes.

## Phase 33 — Orchestrator

- [ ] 33.1 Create `internal/orchestrator` package.
- [ ] 33.2 Define `Pipeline` struct holding all dependencies.
- [ ] 33.3 Define `RunOptions`: `DryRun`, `ForceReprocess`, `Window time.Duration`.
- [ ] 33.4 Implement `Run(ctx, opts) (RunResult, error)`.
- [ ] 33.5 Step 1: open store, record run start.
- [ ] 33.6 Step 2: fetch all accounts concurrently.
- [ ] 33.7 Step 3: filter already-processed via store.
- [ ] 33.8 Step 4: build LLM request with token budgeting.
- [ ] 33.9 Step 5: call provider with retry and breaker.
- [ ] 33.10 Step 6: parse and validate response.
- [ ] 33.11 Step 7: render digest.
- [ ] 33.12 Step 8: execute actions (flags, archive, etc.).
- [ ] 33.13 Step 9: send digest via notify fan-out.
- [ ] 33.14 Step 10: record run finish.
- [ ] 33.15 Implement partial-failure semantics per `architecture.md` §7.
- [ ] 33.16 Implement dry-run mode (skip actions, skip notify).
- [ ] 33.17 Implement force-reprocess (ignore dedup index).
- [ ] 33.18 Add unit tests with all fakes.
- [ ] 33.19 Add test: all stages succeed.
- [ ] 33.20 Add test: ingest partial failure.
- [ ] 33.21 Add test: LLM failure triggers fallback digest.
- [ ] 33.22 Add test: notify partial failure.
- [ ] 33.23 Add test: context cancellation.
- [ ] 33.24 Add test: dry-run skips actions and notify.
- [ ] 33.25 Add test: force-reprocess ignores dedup.
- [ ] 33.26 Add test: fetching "all emails" with overlapping windows does not produce duplicate digests (proves SQLite dedup works).

## Phase 34 — CLI Entrypoint

- [ ] 34.1 Create `cmd/emailer/main.go`.
- [ ] 34.2 Parse CLI flags: `--config`, `--stateless`, `--dry-run`, `--force-reprocess`, `--window`, `--log-level`.
- [ ] 34.3 Load config with precedence.
- [ ] 34.4 Set up logger with secret redaction.
- [ ] 34.5 Set up signal context.
- [ ] 34.6 Set up metrics and tracing.
- [ ] 34.7 Open store (or noop).
- [ ] 34.8 Build all dependencies.
- [ ] 34.9 Build orchestrator.
- [ ] 34.10 Run orchestrator.
- [ ] 34.11 Map `RunResult` to exit code per `architecture.md` §7.
- [ ] 34.12 Ensure deferred cleanup runs on all paths.
- [ ] 34.13 Add smoke test invoking the binary with `--help`.
- [ ] 34.14 Add smoke test with fake dependencies via build tag `smoke`.

## Phase 35 — HTTP Server Entrypoint

- [ ] 35.1 Create `cmd/server/main.go`.
- [ ] 35.2 Create `internal/httpapi` package.
- [ ] 35.3 Implement `POST /run` handler.
- [ ] 35.4 Implement `GET /healthz`.
- [ ] 35.5 Implement `GET /readyz`.
- [ ] 35.6 Implement `GET /metrics`.
- [ ] 35.7 Implement `POST /webhook/imap`.
- [ ] 35.8 Implement bearer token auth middleware.
- [ ] 35.9 Implement request logging middleware.
- [ ] 35.10 Implement graceful shutdown on signal.
- [ ] 35.11 Add integration tests with `httptest.Server`.
- [ ] 35.12 Add test: auth rejection.
- [ ] 35.13 Add test: concurrent run rejection (single in-flight run).

## Phase 36 — Internal Scheduler

- [ ] 36.1 Create `internal/scheduler` package.
- [ ] 36.2 Use `github.com/robfig/cron/v3`.
- [ ] 36.3 Implement `Start(ctx, schedule, fn)`.
- [ ] 36.4 Implement `Stop()` with drain.
- [ ] 36.5 Add unit tests with fake clock.

## Phase 37 — Webhook Inbound

- [ ] 37.1 Create `internal/webhooks` package.
- [ ] 37.2 Define `Receiver` interface.
- [ ] 37.3 Implement `GmailPushReceiver` parsing Pub/Sub push.
- [ ] 37.4 Implement `GenericIMAPIDReceiver`.
- [ ] 37.5 Add unit tests per receiver.

## Phase 38 — Cost Tracking

- [ ] 38.1 Create `internal/cost` package.
- [ ] 38.2 Define `PriceTable` per provider/model.
- [ ] 38.3 Implement `Estimate(provider, model, tokensIn, tokensOut) (Cost, error)`.
- [ ] 38.4 Persist cost per run in `runs` table (migration 0006).
- [ ] 38.5 Add unit tests.
- [ ] 38.6 Add alert hook when run cost exceeds `CostConfig.AlertThreshold`.

## Phase 39 — Docker

- [ ] 39.1 Add `Dockerfile` multi-stage: `golang:1.25-alpine` build, `gcr.io/distroless/static-debian12:nonroot` runtime.
- [ ] 39.2 Add `.dockerignore`.
- [ ] 39.3 Add `docker-bake.hcl` for matrix builds (CLI, server).
- [ ] 39.4 Verify final image under 15 MB.
- [ ] 39.5 Add `docker-compose.yaml` for local development (server + sqlite volume).

## Phase 40 — Deployment Manifests

- [ ] 40.1 Add `deploy/render.yaml` for Render Cron Job.
- [ ] 40.2 Add `deploy/render-service.yaml` for Render Web Service.
- [ ] 40.3 Add `deploy/systemd/emailer.service` and `.timer`.
- [ ] 40.4 Add `deploy/k8s/` manifests: Deployment, Service, ConfigMap, Secret template, CronJob alternative.
- [ ] 40.5 Add `deploy/README.md`.

## Phase 41 — Documentation

- [ ] 41.1 Write `README.md` with quickstart.
- [ ] 41.2 Write `docs/configuration.md` with every option.
- [ ] 41.3 Write `docs/providers.md` with provider setup.
- [ ] 41.4 Write `docs/labels.md` with custom label guide.
- [ ] 41.5 Write `docs/prompts.md` with template guide.
- [ ] 41.6 Write `docs/deployment.md` summarizing Render, systemd, k8s.
- [ ] 41.7 Write `docs/security.md` with threat model.
- [ ] 41.8 Write `docs/troubleshooting.md`.
- [ ] 41.9 Write `docs/development.md` with local setup.
- [ ] 41.10 Generate API docs from OpenAPI for `httpapi`.

## Phase 42 — Hardening and Final Audit

- [ ] 42.1 Run `golangci-lint` and resolve all findings.
- [ ] 42.2 Run `govulncheck` and resolve all findings.
- [ ] 42.3 Run `go test -race ./...`.
- [ ] 42.4 Run `go test -cover ./...` and confirm ≥80%.
- [ ] 42.5 Run fuzz tests for 60 seconds each.
- [ ] 42.6 Audit all error paths for log coverage.
- [ ] 42.7 Audit all secrets for redaction coverage.
- [ ] 42.8 Audit all network calls for timeout and retry.
- [ ] 42.9 Audit all goroutines for context cancellation.
- [ ] 42.10 Verify `architecture.md`, `claude.md`, `planning.md` reflect final state.

## Phase 43 — Release

- [ ] 43.1 Tag `v0.1.0-rc.1`.
- [ ] 43.2 Cut release candidate.
- [ ] 43.3 Run end-to-end on staging for 7 consecutive days.
- [ ] 43.4 Tag `v0.1.0`.
- [ ] 43.5 Publish release notes.
