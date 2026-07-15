# Architecture

## 1. Overview

The Email AI Agent is a one-shot, stateful-by-default Go CLI binary. 
It ingests mail from multiple IMAP accounts, classifies each message 
with a configurable LLM provider, persists run state for idempotency, 
applies IMAP keyword flags back to source mailboxes, and delivers a 
rendered digest to Telegram.

It is designed to be executed by an OS-level scheduler (e.g., systemd 
timers, cron, Windows Task Scheduler) 4 times a day. 

By default, the agent fetches **ALL emails** within a dynamic time window. 
It relies on its internal SQLite state store to ensure idempotency and 
prevent duplicate processing across overlapping or missed runs. The 
generated reports explicitly display the email's timestamp and whether 
it was read or unread.

## 2. Architectural Principles

| Principle | Implementation |
| --- | --- |
| Stateful by default | SQLite run ledger by default; `--stateless` flag disables persistence (requires `FetchUnreadOnly=true`). |
| Idempotent | Composite key `(account_label, uid)` deduplicates work across overlapping runs. |
| Fail-soft | Per-account and per-message failures are isolated; a partial digest is always produced. |
| Provider-agnostic | LLM providers behind a single interface; new providers added via registry. |
| Observable | Structured JSON logs (`slog`) with run-id, account, stage, and duration fields. |
| Secret-safe | All secrets redacted in logs; API keys never appear in URLs. |
| Self-healing | Derives fetch window from the last successful run timestamp to survive host downtime. |

## 3. High-Level Architecture

```text
 ┌──────────────┐
 │ OS Scheduler │ (cron, systemd, Task Scheduler)
 └──────┬───────┘
        │ executes
        ▼
 ┌──────────────────────────────────────────┐
 │            Orchestrator                  │
 │  run id • context • shutdown • logging   │
 └───────┬────────┬────────┬────────┬───────┘
         │        │        │        │
  ┌──────▼┐ ┌─────▼─────┐ ┌▼────────┐ ┌▼────────┐
  │ Ingest│ │  Reason   │ │  Act    │ │ Notify  │
  │ Service│ │  Service  │ │ Service │ │ Service │
  └────┬──┘ └─────┬─────┘ └────┬────┘ └────┬────┘
       │         │            │           │
  ┌────▼────┐ ┌──▼─────┐ ┌────▼────┐ ┌────▼────┐
  │ IMAP    │ │ LLM    │ │ IMAP    │ │Telegram │
  │ Adapter │ │Adapters│ │ Adapter │ │ Adapter │
  └─────────┘ └────────┘ └─────────┘ └─────────┘
       │         │            │           │
       └─────────┴────────────┴───────────┘
                       │
               ┌───────▼────────┐
               │  State Store   │
               │  (SQLite)      │
               └────────────────┘
```

## 4. Package Layout

```text
cmd/
  emailer/           # one-shot CLI entrypoint
internal/
  config/            # layered env + YAML + JSON loader
  log/               # slog setup, secret redaction, run-id injection
  shutdown/          # signal handling, context cancellation
  store/             # SQLite run ledger and dedup index
  mail/              # IMAP adapter, models, sanitization
  llm/               # provider registry, prompt builder, parser, retries
  notify/            # telegram channel, retry policies
  digest/            # markdown renderers
  orchestrator/      # pipeline composition, concurrency, partial failures
  security/          # prompt-injection hardening, output sanitization
  testutil/          # fakes: clock, mail, llm, notifier, store
```

## 5. Core Components

### 5.1 Configuration (`internal/config`)

- Layered sources, later overrides earlier: defaults → YAML file → env vars → CLI flags.
- Typed schema with validation.
- Secrets flagged `sensitive:"true"` are redacted in logs.
- Schema sections: `llm`, `imap`, `notify` (wraps `telegram`), `storage`, `digest`, `labels`, `prompts`, `concurrency`.
- Setting: `FetchUnreadOnly` (boolean, default `false`).
- Setting: `MaxWindow` (duration, default `72h`). Caps the dynamic lookback period to prevent overwhelming the LLM after prolonged host downtime.
- Setting: `imap.timeout` (duration, default `30s`). Bounds each IMAP command (dial, login, select, fetch, store); `0` disables the timeout.
- Setting: `concurrency.fetch_batch_size` (int, default `10`). UIDs fetched per IMAP UID FETCH command; `0` falls back to the default.
- **Digest settings** (`digest`):
  - `max_message_excerpt` (int, default `500`): max characters per message excerpt.
  - `include_read_status` (bool, default `true`): show read/unread badge per message.
  - `include_global_stats` (bool, default `true`): render global summary block (`## Summary`).
  - `include_account_stats` (bool, default `true`): render per-account stats (`## Account Stats`).
  - `include_summaries` (bool, default `true`): render LLM summaries per message.
  - `include_key_points` (bool, default `true`): render key points per message.
  - `include_action_items` (bool, default `true`): render action items per message.
  - `include_raw_excerpt_fallback` (bool, default `true`): show raw excerpt when analysis fails.
  - `max_messages` (int, default `100`): cap total messages in digest (`0` = unlimited).
  - `max_key_points_per_message` (int, default `5`): cap key points per message (`0` = unlimited).
  - `max_action_items_per_message` (int, default `3`): cap action items per message (`0` = unlimited).
  - `priority_only` (bool, default `false`): show only high-priority messages.
- **Digest settings** (`digest` section):
  - `max_message_excerpt` (int, default `500`): max characters per message excerpt.
  - `include_read_status` (bool, default `true`): show read/unread badge per message.
  - `include_global_stats` (bool, default `true`): render global summary block.
  - `include_account_stats` (bool, default `true`): render per-account statistics.
  - `include_summaries` (bool, default `true`): render LLM summaries per message.
  - `include_key_points` (bool, default `true`): render key points per message.
  - `include_action_items` (bool, default `true`): render action items per message.
  - `include_raw_excerpt_fallback` (bool, default `true`): show raw excerpt when analysis fails.
  - `max_messages` (int, default `100`): cap total messages in digest (`0` = unlimited).
  - `max_key_points_per_message` (int, default `5`): cap key points per message (`0` = unlimited).
  - `max_action_items_per_message` (int, default `3`): cap action items per message (`0` = unlimited).
  - `priority_only` (bool, default `false`): only show high-priority messages.

### 5.2 State Store (`internal/store`)

- SQLite database, single file at `--state-path` (default `./state/emailer.db`).
- Tables:
  - `runs(id, started_at, finished_at, status, message_count, error)`
  - `processed_messages(run_id, account_label, uid, is_read, classification, digest_excerpt, processed_at)`
  - `flags_applied(account_label, uid, flag, applied_at)`
  - `digests(run_id, channel, status, payload_hash)`
- Index on `(account_label, uid)` for dedup lookups.
- When `FetchUnreadOnly` is `false`, the SQLite store is strictly required to prevent duplicate digests.

### 5.3 Ingest Service (`internal/mail`)

- Interface:
  ```text
  type Ingester interface {
      Fetch(ctx, account, opts) ([]Message, error)
      ApplyFlags(ctx, account, flags) error
  }
  ```
- One persistent IMAP client per account per run (single dial for fetch + flag).
- Authentication via app passwords exclusively (no OAuth2 flows).
- Configurable folder list per account (default `INBOX`).
- **Command timeouts**: every IMAP command is bounded by `imap.timeout` (default `30s`) via the client's `Timeout` field.
- **Reconnection**: a dropped connection triggers a single reconnect-and-retry (re-Dial + re-Login) on the next operation, bound to the request context. Connection-level errors (EOF, closed, timeout, broken pipe) are detected and retried once.
- **Fetch Mode**: Fetches ALL emails in the time window by default. Conditionally applies the `UNSEEN` flag to the IMAP search criteria if `FetchUnreadOnly` is `true`.
- **Batched fetch**: the UIDs returned by the search are retrieved in batches of `concurrency.fetch_batch_size` (default 10) per UID FETCH command, bounding per-command memory and duration.
- **Metadata Capture**: Captures the `\Seen` flag during fetch to determine if an email was read or unread, persisting this to the `processed_messages` table.
- **Header sanitization**: `Subject`, `From`, and `To` header fields are sanitized before classification/digest rendering — HTML markup is stripped, C0/C1 control characters removed, and HTML entities decoded — so attacker-controlled header values cannot inject markup into prompts or digests.
- MIME-aware body extraction with charset conversion to UTF-8.
- Concurrency: bounded worker pool, default `min(len(accounts), 4)`.

### 5.4 Reason Service (`internal/llm`)

- Provider registry; each provider implements:
  ```text
  type Provider interface {
      Name() string
      Classify(ctx, Request) (Response, error)
  }
  ```
- Built-in providers: **Gemini**, **Ollama**, **OpenRouter**, **Mistral**. New providers are added via the same provider registry (see AGENTS.md §7).
- Composite key `(account_label, uid)` in every payload and response.
- Prompt builder wraps each email in unique delimiters, includes metadata (Date, Read/Unread status), and isolates instructions.
- **Schema versioning:** LLM responses include a top-level `schema_version` integer (currently `1`). The parser validates this field:
  - Missing or `0` → accepted as version 1 (backward compatible).
  - `1` → accepted.
  - `>1` → rejected (unsupported future version).
  - `<0` → rejected (malformed).
  Repair prompts request the current schema version.
- Token budgeter computes per-message cost; batches split before provider call.
- Retry policy: 3 attempts, jittered exponential backoff (base 1s, factor 2, jitter ±25%), only on 429/5xx/network.
- Output validated against JSON schema before use.
- **Partial analysis fallback:** When the LLM response contains a mix of valid and invalid per-message analyses, the pipeline applies a repair-once policy per `llm.analysis_repair_max_attempts` (default 1). Valid analyses are accepted; invalid items are retried via a repair prompt. Items still invalid after repair are classified as `Unknown` with a raw body excerpt and an `AnalysisError` recorded. The digest renders these with a fallback block. If zero valid analyses remain after repair, the whole digest falls back to `FallbackRenderer`. Run status becomes `partially_classified` when analysis failures exist but valid items remain; `degraded` when all items fail.

### 5.5 Act Service (`internal/mail`)

- Custom IMAP keywords (no backslash prefix): `Useful`, `ToDelete`, `Ads`, plus user-defined.
- **PERMANENTFLAGS check**: before storing a custom keyword, `ApplyFlags` consults the selected folder's `PERMANENTFLAGS`. Keywords not permitted by the server (and without the `\*` wildcard) are skipped and logged as a warning. System flags (leading backslash) are always permitted.
- Flag writes batched per account in a single `UID STORE`.
- A dropped connection during flag application triggers a single reconnect-and-retry (see §5.3).

### 5.6 Digest Renderers (`internal/digest`)

- `Renderer` interface with `Render(ctx, DigestData) (string, error)` and `Name() string`.
- `DigestData` struct: run metadata, per-message entries with subject, from, date, read/unread status, classification label/confidence/reason, summary, key points, action items, priority, and excerpt. Also carries `GlobalStats`, `AccountStats`, `Highlights`, and `AnalysisFailedCount`.
- `MarkdownRenderer`: `text/template`-based, groups messages by classification label in alphabetical order, renders Date and Read/Unread status explicitly. Respects `DigestConfig` for all rendering toggles:
  - `IncludeGlobalStats`: suppress global summary block (`## Summary`).
  - `IncludeAccountStats`: suppress per-account stats block (`## Account Stats`).
  - `IncludeSummaries`: omit per-message summaries, key points, and action items.
  - `IncludeKeyPoints`: omit key points section when summaries are shown.
  - `IncludeActionItems`: omit action items section when summaries are shown.
  - `IncludeRawExcerptFallback`: when summaries are enabled but analysis failed, show placeholder instead of raw excerpt.
  - `MaxMessageExcerpt`: truncate message excerpts at this length.
  - `MaxKeyPointsPerMessage`: truncate key points list per message.
  - `MaxActionItemsPerMessage`: truncate action items list per message.
  - `MaxMessages`: cap total messages rendered (preferring high-priority then most recent); `0` = no limit.
  - `PriorityOnly`: if true, filter to only high-priority messages before applying `MaxMessages`.
  - `IncludeReadStatus`: show/hide read/unread badge.
- Includes a "Needs Attention" section for high-priority messages. For messages where LLM analysis failed, renders a fallback block showing the raw excerpt with an error indicator.
- `FallbackRenderer`: simplified digest for LLM failure, lists all fetched messages without classification labels. Also respects `MaxMessages`, `PriorityOnly`, `IncludeGlobalStats`, `IncludeAccountStats`, `MaxMessageExcerpt`, and `IncludeReadStatus`.

### 5.7 Notify Service (`internal/notify`)

- `Channel` interface with `Send(ctx, payload, opts)` and `Name() string`.
- `ChannelRegistry` for factory registration and lookup by name.
- `SendOptions`: filename hint, caption (max 1024 chars).
- Telegram implementation (`internal/notify/telegram`): sends digest via `sendDocument` as multipart/form-data with MarkdownV2 parse mode.
- Caption support: auto-truncated to 1024 characters.
- Size guard: payloads exceeding 45 MB are rejected.
- Retry policy: 3 attempts, jittered exponential backoff (base 1s, factor 2, jitter ±25%), only on 429/5xx/network errors.
- If the run fails before producing a digest, an alert is sent to the configured Telegram chat.

### 5.8 Orchestrator (`internal/orchestrator`)

- Composes ingest → reason → act → notify.
- Emits a run ID at start; propagates via context and logs.
- **Dynamic Windowing (Watermark):** By default, the `Since` parameter for mail ingestion is derived from the `finished_at` timestamp of the last successful run. 
- **Fallback & Caps:** If no previous successful run exists, it defaults to a 24h window. The `--max-window` flag (default 72h) caps the lookback period.
- Explicit `--window` flags override this dynamic behavior entirely.
- Partial failure: any per-account ingest failure is logged, alert sent, and the run continues with remaining accounts.
- LLM failure: digest is rendered from a fallback template, run status marked `degraded`.
- Graceful shutdown: cancels context on SIGINT/SIGTERM, drains in-flight work, persists run status.

## 6. Concurrency Model

- Orchestrator-level: bounded worker pool for account ingestion.
- Per-account: single IMAP connection, sequential within account.
- LLM calls: bounded semaphore, default 4 concurrent provider calls.
- All goroutines tied to a single root context with cancellation.

## 7. Error Handling Strategy

| Failure | Behavior |
| --- | --- |
| Config invalid | Exit non-zero before any network call. |
| Single account ingest fails | Log, alert, continue with other accounts. |
| All accounts fail | Run status `ingest_failed`, alert sent, exit non-zero. |
| LLM transient failure | Retry 3× with backoff. |
| LLM permanent failure | Render fallback digest, status `degraded`, alert sent. |
| LLM partial analysis failure | Accept valid analyses, retry repair once, fallback failed items to raw excerpt, status `partially_classified` if any valid items remain. |
| Flag write fails | Log per-UID error, continue, status `partial`. |
| Telegram send fails | Retry 3×, log, exit non-zero. |
| Context cancelled | Drain in-flight work, persist run status, exit 130. |

## 8. Resilience Patterns

- Retry with jittered exponential backoff.
- Timeout per stage (ingest 60s, reason 120s, act 30s, notify 30s).
- Idempotency via `(account_label, uid)` dedup index.
- Fallback digest template on LLM failure.
- Dynamic watermarks for uninterrupted ingestion despite host downtime.

## 9. Security Considerations

- Secrets redacted in logs (`sensitive:"true"` tag).
- API keys sent in `Authorization` header, never in query string.
- IMAP credentials never logged.
- Prompt injection: email bodies wrapped in unique delimiters, instructions isolated, output schema-validated.
- SQLite file mode 0600.

## 10. Observability

- `slog` JSON handler outputting to `stdout` (captured by systemd/cron logs).
- Structured run summary logged at end of every run with stage durations and counts.

## 11. Deployment

- **Execution:** `cmd/emailer` one-shot binary.
- **Scheduling:** Managed entirely by the host OS (e.g., `systemd.timer`, `cron`). 
  - systemd service and timer units are provided in `deploy/systemd/`.
  - See `deploy/README.md` for setup instructions.
- **Docker (Optional):** A `Dockerfile` is planned but not yet in the tree — see [`TODO.md`](TODO.md). Native binary execution is the primary target.

## 12. Documentation

Project documentation is located in the repository root and `docs/` directory:

- `README.md` — Quickstart and overview.
- `docs/configuration.md` — Full configuration reference with all options.
- `docs/providers.md` — LLM provider setup (initially Gemini; additional providers tracked in TODO.md).
- `docs/security.md` — Threat model and security practices.
- `docs/troubleshooting.md` — Common issues and solutions.
- `deploy/README.md` — systemd timer and cron deployment guide.
- `architecture.md` — This document: architectural overview.
- `AGENTS.md` — Project operating manual for coding agents.
- `planning.md` — Implementation plan with step-by-step progress.
- `SIMVER.md` — Semantic versioning and git tagging policy.

## 13. Testing Strategy

- Unit tests per package with fakes from `internal/testutil`.
- Integration tests with a mock IMAP server.
- Contract tests per LLM provider using recorded HTTP fixtures.

## 14. Known Limitations

- No multi-tenant isolation (single user assumed).
- No interactive classification correction UI.
- No OAuth2 support; IMAP authentication relies strictly on app passwords.
- Single notification channel (Telegram) out of the box.
