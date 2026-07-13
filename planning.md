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
- [ ] 0.6 Add `Makefile` with `build`, `test`, `lint`, `fmt`, `tidy`, `clean`, `run` targets.
- [ ] 0.7 Add `.golangci.yml` with `errcheck`, `govet`, `staticcheck`, `gocyclo`, `misspell`.
- [ ] 0.8 Add `SECURITY.md` with vulnerability reporting policy.
- [ ] 0.9 Add GitHub Actions workflow `ci.yml`: `lint`, `test`, `build` on push and PR.
- [ ] 0.10 Add GitHub Actions workflow `release.yml` on tag.

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
- [ ] 2.4 Implement `defaults.go` with all default values (including `MaxWindow=72h`, `FetchUnreadOnly=false`).
- [ ] 2.5 Implement env loader `loadEnv(cfg *Config) error` using `os.LookupEnv`.
- [ ] 2.6 Implement YAML loader `loadYAML(path string, cfg *Config) error` using `gopkg.in/yaml.v3`.
- [ ] 2.7 Implement JSON loader `loadJSON(path string, cfg *Config) error`.
- [ ] 2.8 Implement CLI flag loader using `flag` package.
- [ ] 2.9 Implement `Load(opts LoadOptions) (Config, error)` that applies sources in precedence order.
- [ ] 2.10 Implement `Validate(cfg Config) error` with full validation.
- [ ] 2.11 Add `IMAPAccount` validation: label, host, port range, username, password non-empty.
- [ ] 2.12 Add `LLMConfig` validation: provider in registry; model non-empty.
- [ ] 2.13 Add `NotifyConfig` validation: Telegram chat ID and token present.
- [ ] 2.14 Add `StorageConfig` validation: path writable if stateful.
- [ ] 2.15 Add `LabelsConfig` validation: at least one label; labels match `^[A-Za-z][A-Za-z0-9_-]{0,30}$`.
- [ ] 2.16 Add `PromptConfig` validation: template parses.
- [ ] 2.17 Add `ConcurrencyConfig` validation: positive integers.
- [ ] 2.18 Add `SecretRedactionPatterns(cfg Config) []regexp.Regexp` deriving patterns from sensitive fields.
- [ ] 2.19 Add unit tests: defaults.
- [ ] 2.20 Add unit tests: env override.
- [ ] 2.21 Add unit tests: YAML/JSON override and precedence.
- [ ] 2.22 Add unit tests: every validation rule.
- [ ] 2.23 Add unit tests: missing required fields.
- [ ] 2.24 Add unit tests: malformed YAML/JSON.
- [ ] 2.25 Add unit tests: sensitive field redaction.
- [ ] 2.26 Add `.env.example` covering every variable.
- [ ] 2.27 Add `config.example.yaml`.

## Phase 3 — Shutdown and Context

- [ ] 3.1 Create `internal/shutdown` package.
- [ ] 3.2 Implement `ContextWithSignal(parent context.Context, signals ...os.Signal) (context.Context, func())`.
- [ ] 3.3 Implement `WaitForDrain(ctx, timeout, wg)` helper.
- [ ] 3.4 Add unit tests with fake signals.

## Phase 4 — State Store

- [ ] 4.1 Create `internal/store` package.
- [ ] 4.2 Define `Store` interface: `RecordRun`, `FinishRun`, `RecordMessage`, `AlreadyProcessed`, `RecordFlag`, `RecordDigest`, `GetRun`, `ListRuns`, `GetLastSuccessfulRunTime`.
- [ ] 4.3 Define `Run`, `ProcessedMessage` (include `IsRead` bool), `FlagRecord`, `DigestRecord` structs.
- [ ] 4.4 Implement `SQLiteStore` using `modernc.org/sqlite` (pure-Go, no CGO).
- [ ] 4.5 Add migrations directory `internal/store/migrations`.
- [ ] 4.6 Migration 0001: `runs` table.
- [ ] 4.7 Migration 0002: `processed_messages` table (include `is_read` column) with composite index on `(account_label, uid)`.
- [ ] 4.8 Migration 0003: `flags_applied` table.
- [ ] 4.9 Migration 0004: `digests` table.
- [ ] 4.10 Implement migration runner via `golang-migrate`.
- [ ] 4.11 Implement `NewSQLiteStore(path string, logger) (*SQLiteStore, error)`.
- [ ] 4.12 Implement `Close()`.
- [ ] 4.13 Implement all interface methods.
- [ ] 4.14 Implement in-memory `NoopStore` for stateless mode.
- [ ] 4.15 Add unit tests per method using in-memory DB.
- [ ] 4.16 Add tests for `AlreadyProcessed` correctness under concurrent runs.
- [ ] 4.17 Add tests for `GetLastSuccessfulRunTime` returning correct timestamp or zero value.

## Phase 5 — Mail Models

- [ ] 5.1 Create `internal/mail` package.
- [ ] 5.2 Define `Message` struct: `AccountLabel`, `UID`, `Folder`, `MessageID`, `From`, `To`, `Subject`, `Date`, `Body`, `IsRead`, `AttachmentMetas`.
- [ ] 5.3 Define `AttachmentMeta` struct: `Filename`, `MIME`, `Size`.
- [ ] 5.4 Define `Classification` type as string.
- [ ] 5.5 Define `MessageKey` type as `(AccountLabel, UID)` composite.
- [ ] 5.6 Define `Flag` type as string (no backslash).
- [ ] 5.7 Define `ClassificationToFlag(c Classification, cfg LabelsConfig) Flag` mapping.
- [ ] 5.8 Add unit tests for mapping including custom labels.
- [ ] 5.9 Add unit tests for `MessageKey` equality and map behavior.

## Phase 6 — Mail Sanitization

- [ ] 6.1 Create `internal/mail/sanitize.go`.
- [ ] 6.2 Implement `StripHTML(s string) string` with state machine, not regex.
- [ ] 6.3 Implement `StripControlChars(s string) string`.
- [ ] 6.4 Implement `DecodeEntities(s string) string`.
- [ ] 6.5 Implement `ConvertCharset(r io.Reader, contentType string) (string, error)` using `golang.org/x/text/encoding`.
- [ ] 6.6 Implement `Truncate(s string, limit int) string` rune-aware.
- [ ] 6.7 Add unit tests for HTML stripping edge cases.
- [ ] 6.8 Add unit tests for charset conversion (ISO-8859-1, KOI8-R, Shift-JIS).
- [ ] 6.9 Add unit tests for entity decoding.

## Phase 7 — Mail IMAP Adapter

- [ ] 7.1 Add dependency `github.com/emersion/go-imap/v15` and `github.com/emersion/go-message`.
- [ ] 7.2 Define `Ingester` interface: `Fetch`, `ApplyFlags`, `Close`.
- [ ] 7.3 Define `FetchOptions`: `Since`, `Folders`, `Limit`, `IncludeAttachments`, `UnreadOnly`.
- [ ] 7.4 Implement `IMAPClient` struct wrapping `*client.Client`.
- [ ] 7.5 Implement `dial(ctx, account)` with TLS, STARTTLS, plaintext options.
- [ ] 7.6 Implement `login(ctx, account)` using app passwords.
- [ ] 7.7 Implement `selectFolder(ctx, folder)`.
- [ ] 7.8 Implement `searchByWindow(ctx, since, unreadOnly)` returning UIDs.
- [ ] 7.9 Implement `fetchHeaders(ctx, uidset)` returning envelopes and `\Seen` flag status.
- [ ] 7.10 Implement `fetchBody(ctx, uidset)` returning body sections.
- [ ] 7.11 Implement `readBody(part io.Reader, contentType string) (string, []AttachmentMeta, error)`.
- [ ] 7.12 Implement `applyFlags(ctx, uidset, flags)` using `UID STORE` with plain keywords.
- [ ] 7.13 Implement `Close()` with logout.
- [ ] 7.14 Add unit tests for `readBody` with multipart fixtures.
- [ ] 7.15 Add unit tests for `applyFlags` flag mapping.
- [ ] 7.16 Add unit tests for IMAP search criteria generation (verify `UNSEEN` flag is conditionally added).
- [ ] 7.17 Add integration test with `go-imap` mock server verifying "all emails" fetch returns read and unread messages.

## Phase 8 — Mail Concurrency

- [ ] 8.1 Create `internal/mail/pool.go`.
- [ ] 8.2 Implement `FetchAll(ctx, accounts []IMAPAccount, opts FetchOptions, concurrency int) (map[string][]Message, map[string]error)`.
- [ ] 8.3 Use `errgroup` with bounded semaphore.
- [ ] 8.4 Add unit tests with fake `Ingester` implementations.
- [ ] 8.5 Add test: one account fails, others succeed.
- [ ] 8.6 Add test: context cancelled mid-fetch.

## Phase 9 — LLM Models

- [ ] 9.1 Create `internal/llm` package.
- [ ] 9.2 Define `Request` struct: `Messages []InputMessage`, `Labels []Classification`, `PromptTemplate string`, `BudgetTokens int`.
- [ ] 9.3 Define `InputMessage` struct: `Key MessageKey`, `From`, `Subject`, `Date`, `IsRead`, `Body`.
- [ ] 9.4 Define `Response` struct: `Digest string`, `Classifications map[MessageKey]Classification`, `TokensUsed int`, `ProviderMeta`.
- [ ] 9.5 Define `Provider` interface: `Name()`, `Classify(ctx, Request) (Response, error)`.
- [ ] 9.6 Define `ProviderRegistry` type.
- [ ] 9.7 Implement `Register(name, factory)` and `Lookup(name)`.
- [ ] 9.8 Add unit tests for registry behavior.

## Phase 10 — Prompt Engineering

- [ ] 10.1 Create `internal/llm/prompt.go`.
- [ ] 10.2 Define `BuildPrompt(req Request) (string, error)`.
- [ ] 10.3 Wrap each email in delimiters: `<email uid="..." account="..."> ... </email>`.
- [ ] 10.4 Place instructions above and below the data block, including email metadata (Date, Read/Unread status).
- [ ] 10.5 Include explicit JSON schema in the prompt.
- [ ] 10.6 Include label definitions from `LabelsConfig`.
- [ ] 10.7 Make template configurable via `PromptConfig.Template`.
- [ ] 10.8 Use `text/template` for rendering.
- [ ] 10.9 Add unit tests for prompt structure.
- [ ] 10.10 Add unit tests for template injection (template values are escaped).

## Phase 11 — Response Parsing

- [ ] 11.1 Create `internal/llm/parse.go`.
- [ ] 11.2 Implement `ParseResponse(raw string, labels []Classification) (Response, error)`.
- [ ] 11.3 Strip markdown code fences robustly.
- [ ] 11.4 Validate JSON against schema using `github.com/xeipuuv/gojsonschema`.
- [ ] 11.5 Parse `classifications` as `map[MessageKey]Classification`.
- [ ] 11.6 Reject unknown classifications, duplicate keys, or missing keys.
- [ ] 11.7 Implement `RepairWithPrompt(raw, err) (string, error)` that asks the model to fix its output.
- [ ] 11.8 Add unit tests for valid response and each rejection case.

## Phase 12 — Token Budgeting & Retries

- [ ] 12.1 Create `internal/llm/budget.go`.
- [ ] 12.2 Implement `EstimateTokens(s string) int` and `SplitBatch(msgs []InputMessage, budget int) ([][]InputMessage, error)`.
- [ ] 12.3 Create `internal/llm/retry.go`.
- [ ] 12.4 Implement `RetryPolicy` struct and `Do(ctx, fn) error` with jittered exponential backoff.
- [ ] 12.5 Add unit tests for split correctness and retry exhaustion.

## Phase 13 — LLM Gemini Adapter

- [ ] 13.1 Create `internal/llm/gemini` package.
- [ ] 13.2 Implement `Factory(cfg LLMConfig) (Provider, error)`.
- [ ] 13.3 Implement `Classify` using `POST /v1beta/models/{model}:generateContent` with `x-goog-api-key` header.
- [ ] 13.4 Implement response unmarshaling and token usage extraction.
- [ ] 13.5 Add HTTP fixture under `testdata/gemini/` and contract tests using `httptest.Server`.

## Phase 14 — LLM Ollama Adapter

- [ ] 14.1 Create `internal/llm/ollama` package.
- [ ] 14.2 Implement `Factory`.
- [ ] 14.3 Implement `Classify` using `POST /api/chat` with `format: "json"`.
- [ ] 14.4 Add fixtures and contract tests.

## Phase 15 — LLM OpenRouter Adapter

- [ ] 15.1 Create `internal/llm/openrouter` package.
- [ ] 15.2 Implement `Factory` (OpenAI-compatible with extra headers).
- [ ] 15.3 Add fixtures and contract tests.

## Phase 16 — Security Hardening

- [ ] 16.1 Create `internal/security` package.
- [ ] 16.2 Implement `WrapEmailContent(content string, key MessageKey) string` with delimiters.
- [ ] 16.3 Implement `IsolateInstructions(prompt string) string`.
- [ ] 16.4 Implement `SanitizeOutput(raw string) string` removing embedded instructions.
- [ ] 16.5 Add unit tests for delimiter injection resistance and output sanitization.

## Phase 17 — Digest Renderers

- [ ] 17.1 Create `internal/digest` package.
- [ ] 17.2 Define `Renderer` interface: `Render(data DigestData) ([]byte, string, error)`.
- [ ] 17.3 Define `DigestData` struct: `RunID`, `StartedAt`, `FinishedAt`, `Messages []ClassifiedMessage`, `Summary string`, `Status`.
- [ ] 17.4 Implement `MarkdownRenderer` using `text/template` (must explicitly render Date and Read/Unread status).
- [ ] 17.5 Implement `FallbackRenderer` for LLM failure (lists messages without classifications).
- [ ] 17.6 Add unit tests per renderer.

## Phase 18 — Notify Telegram Channel

- [ ] 18.1 Create `internal/notify` package.
- [ ] 18.2 Define `Channel` interface: `Name()`, `Send(ctx, Payload) error`.
- [ ] 18.3 Create `internal/notify/telegram` package.
- [ ] 18.4 Implement `Send` using `sendDocument` for payloads over 4096 bytes and `sendMessage` for short payloads.
- [ ] 18.5 Implement caption support (1024 char limit) and retry logic.
- [ ] 18.6 Add HTTP fixture tests.

## Phase 19 — Orchestrator

- [ ] 19.1 Create `internal/orchestrator` package.
- [ ] 19.2 Define `Pipeline` struct holding all dependencies.
- [ ] 19.3 Define `RunOptions`: `DryRun`, `ForceReprocess`, `Window time.Duration`.
- [ ] 19.4 Implement `Run(ctx, opts) (RunResult, error)`.
- [ ] 19.5 Step 1: open store, record run start.
- [ ] 19.6 Step 2: fetch all accounts concurrently.
- [ ] 19.7 Step 3: filter already-processed via store.
- [ ] 19.8 Step 4: build LLM request with token budgeting.
- [ ] 19.9 Step 5: call provider with retry.
- [ ] 19.10 Step 6: parse and validate response.
- [ ] 19.11 Step 7: render digest.
- [ ] 19.12 Step 8: execute flag writes.
- [ ] 19.13 Step 9: send digest via notify.
- [ ] 19.14 Step 10: record run finish.
- [ ] 19.15 Implement partial-failure semantics per `architecture.md` §7.
- [ ] 19.16 Implement Dynamic Window logic: If `RunOptions.Window` is zero, query store for `GetLastSuccessfulRunTime()`.
- [ ] 19.17 Apply `MaxWindow` cap: If `time.Since(lastRun) > MaxWindow`, use `MaxWindow`.
- [ ] 19.18 Apply Fallback: If no previous run exists, use default 24h window.
- [ ] 19.19 Add unit tests with all fakes for the above scenarios.

## Phase 20 — CLI Entrypoint

- [ ] 20.1 Create `cmd/emailer/main.go`.
- [ ] 20.2 Parse CLI flags: `--config`, `--stateless`, `--dry-run`, `--force-reprocess`, `--window`, `--max-window`, `--log-level`.
- [ ] 20.3 Load config with precedence.
- [ ] 20.4 Set up logger with secret redaction.
- [ ] 20.5 Set up signal context.
- [ ] 20.6 Open store (or noop).
- [ ] 20.7 Build all dependencies and run orchestrator.
- [ ] 20.8 Map `RunResult` to exit code per `architecture.md` §7.
- [ ] 20.9 Ensure deferred cleanup runs on all paths.
- [ ] 20.10 Add smoke test invoking the binary with `--help`.

## Phase 21 — Docker (Optional)

- [ ] 21.1 Add `Dockerfile` multi-stage: `golang:1.25-alpine` build, `gcr.io/distroless/static-debian12:nonroot` runtime.
- [ ] 21.2 Add `.dockerignore`.

## Phase 22 — Deployment Manifests

- [ ] 22.1 Add `deploy/systemd/emailer.service` and `.timer`.
- [ ] 22.2 Add `deploy/README.md` explaining how to set up OS-level scheduling.

## Phase 23 — Documentation

- [ ] 23.1 Write `README.md` with quickstart.
- [ ] 23.2 Write `docs/configuration.md` with every option.
- [ ] 23.3 Write `docs/providers.md` with provider setup.
- [ ] 23.4 Write `docs/security.md` with threat model.
- [ ] 23.5 Write `docs/troubleshooting.md`.

## Phase 24 — Hardening and Final Audit

- [ ] 24.1 Run `golangci-lint` and resolve all findings.
- [ ] 24.2 Run `govulncheck` and resolve all findings.
- [ ] 24.3 Run `go test -race ./...`.
- [ ] 24.4 Audit all error paths for log coverage.
- [ ] 24.5 Audit all secrets for redaction coverage.
- [ ] 24.6 Audit all network calls for timeout and retry.
- [ ] 24.7 Verify `architecture.md`, `CLAUDE.md`, `planning.md` reflect final state.

## Phase 25 — Release

- [ ] 25.1 Tag `v0.1.0-rc.1`.
- [ ] 25.2 Cut release candidate.
- [ ] 25.3 Run end-to-end on staging for 7 consecutive days.
- [ ] 25.4 Tag `v0.1.0`.
- [ ] 25.5 Publish release notes.
