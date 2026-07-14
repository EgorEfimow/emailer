# Planning

This document is the canonical, step-by-step, smallest-possible-increment
plan for building the Email AI Agent from scratch. Every step is a single,
testable, mergeable change. No step depends on a later step. No step
combines two concerns.

Legend:
- `[ ]` not started
- `[x]` done (update as we go)

## Phase 0 — Repository Foundation

### Branch: `chore/repo-foundation`
- [x] 0.1 Initialize Go module `github.com/<org>/emailer` with Go 1.26.
- [x] 0.2 Create directory tree from `architecture.md` §4 (empty `.keep` files).
- [x] 0.3 Add `LICENSE` (MIT).
- [x] 0.4 Add `.gitignore` (Go, IDE, OS, `.env`, `state/`).
- [x] 0.5 Add `.editorconfig`.
- [x] 0.6 Add `Makefile` with `build`, `test`, `lint`, `fmt`, `tidy`, `clean`, `run` targets.
- [x] 0.7 Add `.golangci.yml` with `errcheck`, `govet`, `staticcheck`, `gocyclo`, `misspell`.
- [x] 0.8 Add `SECURITY.md` with vulnerability reporting policy.
- [x] 0.9 Add GitHub Actions workflow `ci.yml`: `lint`, `test`, `build` on push and PR.
- [x] 0.10 Add GitHub Actions workflow `release.yml` on tag.
- [x] 0.11 Add `.pre-commit-config.yaml` configuring gitleaks to scan for secrets before local commits.
- [x] 0.12 Add `gitleaks` scanning step to the `ci.yml` GitHub Actions workflow.
- [x] 0.13 Install pre-commit hooks locally (`pre-commit install`).

## Phase 1 — Logging Foundation

### Branch: `feat/logging`
- [x] 1.1 Create `internal/log` package and implement `NewLogger(w io.Writer, level string, opts ...Option) (*slog.Logger, error)`.
- [x] 1.2 Add unit tests for `NewLogger` level parsing.
- [x] 1.3 Add `WithRunID(logger, runID)` helper.
- [x] 1.4 Add unit tests for run-id injection.
- [x] 1.5 Add `WithSecretRedaction(logger, patterns []regexp.Regexp)` helper and `Sensitive` type wrapper.
- [x] 1.6 Add unit tests for secret redaction.

## Phase 2 — Configuration Core

### Branch: `feat/config-structs-and-defaults`
- [x] 2.1 Create `internal/config` package. Define `Config` struct with all sections from `architecture.md` §5.1 as nested structs. Tag every secret field with `sensitive:"true"`.
- [x] 2.2 Implement `defaults.go` with all default values (including `MaxWindow=72h`, `FetchUnreadOnly=false`).
- [x] 2.3 Add unit tests for defaults and struct loading.

### Branch: `feat/config-loaders`
- [x] 2.4 Implement env loader `loadEnv(cfg *Config) error` using `os.LookupEnv`.
- [x] 2.5 Add unit tests for env override.
- [x] 2.6 Implement YAML loader `loadYAML(path string, cfg *Config) error`.
- [x] 2.7 Add unit tests for YAML override.
- [x] 2.8 Implement JSON loader `loadJSON(path string, cfg *Config) error`.
- [x] 2.9 Add unit tests for JSON override.
- [x] 2.10 Implement CLI flag loader using `flag` package.
- [x] 2.11 Implement `Load(opts LoadOptions) (Config, error)` that applies sources in precedence order.
- [x] 2.12 Add unit tests for precedence ordering.

### Branch: `feat/config-validation`
- [x] 2.13 Implement `Validate(cfg Config) error` with full validation.
- [x] 2.14 Add `IMAPAccount` validation (label, host, port range, username, password).
- [x] 2.15 Add unit tests for `IMAPAccount` validation.
- [x] 2.16 Add `LLMConfig` and `NotifyConfig` validation.
- [x] 2.17 Add unit tests for `LLMConfig` and `NotifyConfig` validation.
- [x] 2.18 Add `StorageConfig`, `LabelsConfig`, `PromptConfig`, and `ConcurrencyConfig` validation.
- [x] 2.19 Add unit tests for `Storage`, `Labels`, `Prompt`, and `Concurrency` validation.
- [x] 2.20 Implement `SecretRedactionPatterns(cfg Config) []regexp.Regexp`.
- [x] 2.21 Add unit tests for missing required fields and malformed inputs.
- [x] 2.22 Add `.env.example` and `config.example.yaml`.

## Phase 3 — Shutdown and Context

### Branch: `feat/shutdown-context`
- [x] 3.1 Create `internal/shutdown` package. Implement `ContextWithSignal(parent context.Context, signals ...os.Signal) (context.Context, func())`.
- [x] 3.2 Add unit tests for `ContextWithSignal` with fake signals.
- [x] 3.3 Implement `WaitForDrain(ctx, timeout, wg)` helper.
- [x] 3.4 Add unit tests for `WaitForDrain`.

## Phase 4 — State Store

### Branch: `feat/sqlite-schema`
- [x] 4.1 Create `internal/store` package. Define `Store` interface and domain structs (`Run`, `ProcessedMessage` with `IsRead`, `FlagRecord`, `DigestRecord`).
- [x] 4.2 Implement `SQLiteStore` using `modernc.org/sqlite`. Add migrations directory.
- [x] 4.3 Migration 0001: `runs` table.
- [x] 4.4 Migration 0002: `processed_messages` table (include `is_read` column) with composite index on `(account_label, uid)`.
- [x] 4.5 Migration 0003: `flags_applied` table.
- [x] 4.6 Migration 0004: `digests` table.
- [x] 4.7 Implement migration runner via `golang-migrate`.
- [x] 4.8 Add unit tests verifying migrations apply cleanly to an in-memory DB.

### Branch: `feat/sqlite-implementation`
- [x] 4.9 Implement `NewSQLiteStore`, `Close()`, `RecordRun`, and `FinishRun`.
- [x] 4.10 Add unit tests for `RecordRun` and `FinishRun`.
- [x] 4.11 Implement `RecordMessage` and `AlreadyProcessed` (batch lookup).
- [x] 4.12 Add unit tests for `RecordMessage` and `AlreadyProcessed` (including concurrent runs).
- [x] 4.13 Implement `RecordFlag` and `RecordDigest`.
- [x] 4.14 Add unit tests for `RecordFlag` and `RecordDigest`.
- [x] 4.15 Implement `GetRun`, `ListRuns`, and `GetLastSuccessfulRunTime`.
- [x] 4.16 Add unit tests for `GetRun`, `ListRuns`, and `GetLastSuccessfulRunTime`.
- [x] 4.17 Implement in-memory `NoopStore` for stateless mode.

## Phase 5 — Mail Models

### Branch: `feat/mail-models`
- [x] 5.1 Create `internal/mail` package. Define `Message`, `AttachmentMeta`, `Classification`, `MessageKey`, and `Flag` types.
- [x] 5.2 Add unit tests for `MessageKey` equality and map behavior.
- [x] 5.3 Define `ClassificationToFlag(c Classification, cfg LabelsConfig) Flag` mapping.
- [x] 5.4 Add unit tests for mapping including custom labels.

## Phase 6 — Mail Sanitization

### Branch: `feat/mail-sanitization`
- [x] 6.1 Create `internal/mail/sanitize.go`. Implement `StripHTML(s string) string` with state machine.
- [x] 6.2 Add unit tests for HTML stripping edge cases.
- [x] 6.3 Implement `StripControlChars` and `DecodeEntities`.
- [x] 6.4 Add unit tests for entity decoding and control chars.
- [x] 6.5 Implement `ConvertCharset(r io.Reader, contentType string) (string, error)`.
- [x] 6.6 Add unit tests for charset conversion (ISO-8859-1, KOI8-R, Shift-JIS).
- [x] 6.7 Implement `Truncate(s string, limit int) string` rune-aware.
- [x] 6.8 Add unit tests for truncation.

## Phase 7 — Mail IMAP Adapter

### Branch: `feat/imap-core`
- [x] 7.1 Add dependency `github.com/emersion/go-imap/v2` and `github.com/emersion/go-message`. Define `Ingester` interface and `FetchOptions`.
- [x] 7.2 Implement `IMAPClient` struct, `dial(ctx, account)` with TLS/STARTTLS/plaintext, and `login(ctx, account)` using app passwords.
- [x] 7.3 Implement `selectFolder` and `searchByWindow(ctx, since, unreadOnly)`.
- [x] 7.4 Add unit tests for IMAP search criteria generation (verify `UNSEEN` flag is conditionally added).
- [x] 7.5 Implement `fetchHeaders(ctx, uidset)` returning envelopes and `\Seen` flag status.
- [x] 7.6 Add integration test with `go-imap` mock server verifying "all emails" fetch returns read and unread messages.

### Branch: `feat/imap-fetch-and-flag`
- [x] 7.7 Implement `fetchBody(ctx, uidset)` and `readBody(part io.Reader, contentType string) (string, []AttachmentMeta, error)`.
- [x] 7.8 Add unit tests for `readBody` with multipart fixtures.
- [x] 7.9 Implement `applyFlags(ctx, uidset, flags)` using `UID STORE` with plain keywords. Implement `Close()`.
- [x] 7.10 Add unit tests for `applyFlags` flag mapping.

## Phase 8 — Mail Concurrency

### Branch: `feat/mail-concurrency`
- [x] 8.1 Create `internal/mail/pool.go`. Implement `FetchAll` using `errgroup` with bounded semaphore.
- [x] 8.2 Add unit tests with fake `Ingester` implementations (one account fails, others succeed).
- [x] 8.3 Add unit test: context cancelled mid-fetch.

## Phase 9 — LLM Models

### Branch: `feat/llm-models`
- [x] 9.1 Create `internal/llm` package. Define `Request`, `InputMessage`, `Response`, and `Provider` interface.
- [x] 9.2 Define `ProviderRegistry` type. Implement `Register` and `Lookup`.
- [x] 9.3 Add unit tests for registry behavior.

## Phase 10 — Prompt Engineering

### Branch: `feat/prompt-engineering`
- [x] 10.1 Create `internal/llm/prompt.go`. Define `BuildPrompt(req Request) (string, error)` using `text/template`.
- [x] 10.2 Add unit tests for prompt structure (delimiters, metadata, schema).
- [x] 10.3 Add unit tests for template injection (template values are escaped).

## Phase 11 — Response Parsing

### Branch: `feat/response-parsing`
- [x] 11.1 Create `internal/llm/parse.go`. Implement `ParseResponse` stripping fences and validating JSON against schema.
- [x] 11.2 Add unit tests for valid response parsing.
- [x] 11.3 Add unit tests for each rejection case (unknown classifications, duplicate keys, missing keys).
- [x] 11.4 Implement `RepairWithPrompt(raw, err) (string, error)`.
- [x] 11.5 Add unit tests for `RepairWithPrompt`.

## Phase 13 — LLM Gemini Adapter

### Branch: `feat/llm-gemini`
- [x] 13.1 Create `internal/llm/gemini` package. Implement `Factory` and `Classify` using `x-goog-api-key` header.
- [x] 13.2 Implement response unmarshaling and token usage extraction.
- [x] 13.3 Add HTTP fixture under `testdata/gemini/` and contract tests using `httptest.Server`.
- [x] 13.4 Add test for API key in header (not URL) and retryable status codes.

## Phase 17 — Digest Renderers

### Branch: `feat/digest-renderers`
- [x] 17.1 Create `internal/digest` package. Define `Renderer` interface and `DigestData` struct.
- [x] 17.2 Implement `MarkdownRenderer` using `text/template` (explicitly render Date and Read/Unread status).
- [x] 17.3 Add unit tests for `MarkdownRenderer`.
- [x] 17.4 Implement `FallbackRenderer` for LLM failure.
- [x] 17.5 Add unit tests for `FallbackRenderer`.

## Phase 18 — Notify Telegram Channel

### Branch: `feat/notify-telegram`
- [x] 18.1 Create `internal/notify` package. Define `Channel` interface.
- [x] 18.2 Create `internal/notify/telegram` package. Implement `Send` using `sendDocument` and `sendMessage`.
- [x] 18.3 Implement caption support (1024 char limit) and retry logic.
- [x] 18.4 Add HTTP fixture tests (size guard, retryable status codes).

## Phase 19 — Orchestrator

### Branch: `feat/orchestrator-pipeline`
- [x] 19.1 Create `internal/orchestrator` package. Define `Pipeline` and `RunOptions`.
- [x] 19.2 Implement `Run` steps 1-4 (open store, fetch concurrently, filter, build LLM request).
- [x] 19.3 Implement `Run` steps 5-10 (call provider, parse, render, execute flags, send digest, record finish).
- [x] 19.4 Add unit test: all stages succeed.
- [x] 19.5 Add unit test: ingest partial failure (continues with remaining accounts).

### Branch: `feat/orchestrator-window-logic`
- [x] 19.6 Implement partial-failure semantics and fallback digest on LLM failure.
- [x] 19.7 Add unit test: LLM failure triggers fallback digest.
- [x] 19.8 Implement Dynamic Window logic (`GetLastSuccessfulRunTime`, `MaxWindow` cap, 24h fallback).
- [x] 19.9 Add unit test: orchestrator uses `lastRun` time when `Window` is unset.
- [x] 19.10 Add unit test: explicit `--window` overrides dynamic logic completely.

## Phase 20 — CLI Entrypoint

### Branch: `feat/cli-entrypoint`
- [x] 20.1 Create `cmd/emailer/main.go` — flag parsing, config loading, logger, store. Main flags: `--config`, `--log-level`, `--dry-run`, `--force-reprocess`, `--window`. Other flags delegated to `config.Load`.
- [x] 20.2 Build dependencies: IMAP ingesters, LLM provider (via registry), renderers, notification channel (via registry). Run orchestrator.
- [x] 20.3 Exit code mapping: 0=success, 1=fatal error, 2=config/flag error, 130=cancelled.
- [x] 20.4 Smoke test: `--help` prints usage and exits 2; no config exits 2 with validation errors.

## Phase 21 — Docker (Optional)

### Branch: `chore/docker`
- [ ] 21.1 Add `Dockerfile` multi-stage: `golang:1.25-alpine` build, `gcr.io/distroless/static-debian12:nonroot` runtime.
- [ ] 21.2 Add `.dockerignore`.

## Phase 22 — Deployment Manifests

### Branch: `docs/deployment-manifests`
- [x] 22.1 Add `deploy/systemd/emailer.service` and `.timer`.
- [x] 22.2 Add `deploy/README.md` explaining how to set up OS-level scheduling.

## Phase 23 — Documentation

### Branch: `docs/user-documentation`
- [x] 23.1 Write `README.md` with quickstart.
- [x] 23.2 Write `docs/configuration.md` with every option.
- [x] 23.3 Write `docs/providers.md` with provider setup.
- [x] 23.4 Write `docs/security.md` with threat model.
- [x] 23.5 Write `docs/troubleshooting.md`.

## Phase 24 — Hardening and Final Audit

### Branch: `chore/hardening-audit`
- [ ] 24.1 Run `golangci-lint` and resolve all findings.
- [ ] 24.2 Run `govulncheck` and resolve all findings.
- [ ] 24.3 Run `go test -race ./...`.
- [ ] 24.4 Audit all error paths for log coverage.
- [ ] 24.5 Audit all secrets for redaction coverage.
- [ ] 24.6 Audit all network calls for timeout and retry.
- [ ] 24.7 Verify `architecture.md`, `CLAUDE.md`, `planning.md` reflect final state.

## Phase 25 — Release

### Branch: `release/v0.1.0`
- [ ] 25.1 Tag `v0.1.0-rc.1`.
- [ ] 25.2 Cut release candidate.
- [ ] 25.3 Run end-to-end on staging for 7 consecutive days.
- [ ] 25.4 Tag `v0.1.0`.
- [ ] 25.5 Publish release notes.
