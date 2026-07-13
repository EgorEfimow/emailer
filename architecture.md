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
- Schema sections: `llm`, `imap`, `telegram`, `storage`, `digest`, `labels`, `prompts`.
- Setting: `FetchUnreadOnly` (boolean, default `false`).
- Setting: `MaxWindow` (duration, default `72h`). Caps the dynamic lookback period to prevent overwhelming the LLM after prolonged host downtime.

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
- **Fetch Mode**: Fetches ALL emails in the time window by default. Conditionally applies the `UNSEEN` flag to the IMAP search criteria if `FetchUnreadOnly` is `true`.
- **Metadata Capture**: Captures the `\Seen` flag during fetch to determine if an email was read or unread, persisting this to the `processed_messages` table.
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
- Built-in providers: Gemini, Ollama, OpenRouter.
- Composite key `(account_label, uid)` in every payload and response.
- Prompt builder wraps each email in unique delimiters, includes metadata (Date, Read/Unread status), and isolates instructions.
- Token budgeter computes per-message cost; batches split before provider call.
- Retry policy: 3 attempts, jittered exponential backoff (base 1s, factor 2, jitter ±25%), only on 429/5xx/network.
- Output validated against JSON schema before use.

### 5.5 Act Service (`internal/mail`)

- Custom IMAP keywords (no backslash prefix): `Useful`, `ToDelete`, `Ads`, plus user-defined.
- Flag writes batched per account in a single `UID STORE`.

### 5.6 Notify Service (`internal/notify`)

- Channel registry; currently supports Telegram (document and message).
- Renderers (`internal/digest`): Markdown (explicitly renders Date and Read/Unread status).
- Retry policy: 3 attempts, jittered backoff.
- If the run fails before producing a digest, an alert is sent to the configured Telegram chat.

### 5.7 Orchestrator (`internal/orchestrator`)

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
- **Docker (Optional):** A simple `Dockerfile` is provided for containerized environments, but native binary execution is the primary target.

## 12. Testing Strategy

- Unit tests per package with fakes from `internal/testutil`.
- Integration tests with a mock IMAP server.
- Contract tests per LLM provider using recorded HTTP fixtures.

## 13. Known Limitations

- No multi-tenant isolation (single user assumed).
- No interactive classification correction UI.
- No OAuth2 support; IMAP authentication relies strictly on app passwords.
- Single notification channel (Telegram) out of the box.
