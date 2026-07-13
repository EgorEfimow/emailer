# Architecture

## 1. Overview

The Email AI Agent is a scheduled, stateful-by-default Go service that
ingests unread mail from multiple IMAP accounts, classifies each message
with a configurable LLM provider, persists run state for idempotency,
applies IMAP keyword flags back to source mailboxes, and delivers a
rendered digest to one or more notification channels.

The service can run as a one-shot CLI (cron-driven) or as a long-running
process exposing an HTTP control plane (manual trigger, health, metrics).

## 2. Architectural Principles

| Principle | Implementation |
| --- | --- |
| Stateless option, stateful default | SQLite run ledger by default; `--stateless` flag disables persistence for ephemeral runs. |
| Idempotent | Composite key `(account_label, uid)` deduplicates work across runs. |
| Fail-soft | Per-account and per-message failures are isolated; a partial digest is always produced. |
| Provider-agnostic | LLM providers behind a single interface; new providers added via registry. |
| Observable | Structured logs, Prometheus metrics, OpenTelemetry traces, explicit run IDs. |
| Secret-safe | All secrets redacted in logs; API keys never appear in URLs. |
| Testable | All I/O behind interfaces; fakes for mail, LLM, notifier, clock, and store. |
| Backpressure-aware | Bounded concurrency, token-budgeted batching, circuit breakers. |

## 3. High-Level Architecture

```text
                 ┌──────────────────────────────────────────┐
                 │              Control Plane               │
                 │  CLI (cron)  •  HTTP API  •  Webhook     │
                 └───────────────────────┬──────────────────┘
                                         │
                                         ▼
                 ┌──────────────────────────────────────────┐
                 │            Orchestrator                  │
                 │  run id • context • shutdown • metrics   │
                 └───────┬────────┬────────┬────────┬───────┘
                         │        │        │        │
              ┌──────────▼┐ ┌─────▼─────┐ ┌▼─────────┐ ┌▼────────┐
              │  Ingest   │ │  Reason   │ │  Act     │ │ Notify  │
              │  Service  │ │  Service  │ │ Service  │ │ Service │
              └─────┬─────┘ └─────┬─────┘ └─────┬────┘ └────┬────┘
                    │             │            │           │
              ┌─────▼─────┐ ┌─────▼─────┐ ┌────▼────┐ ┌────▼────┐
              │ IMAP      │ │ LLM       │ │ IMAP    │ │ Channel │
              │ Adapter   │ │ Adapters  │ │ Adapter │ │ Adapters│
              │ (go-imap) │ │ (registry)│ │         │ │         │
              └───────────┘ └───────────┘ └─────────┘ └─────────┘
                    │             │            │           │
                    └─────────────┴────────────┴───────────┘
                                  │
                          ┌───────▼────────┐
                          │  State Store   │
                          │  (SQLite)      │
                          │  run ledger    │
                          │  dedup index   │
                          └────────────────┘
```

## 4. Package Layout

```text
cmd/
  emailer/           # one-shot CLI entrypoint
  server/            # long-running HTTP entrypoint
internal/
  config/            # layered env + YAML + JSON loader
  log/               # slog setup, secret redaction, run-id injection
  metrics/           # Prometheus collectors
  trace/             # OpenTelemetry setup
  shutdown/          # signal handling, context cancellation
  store/             # SQLite run ledger and dedup index
  mail/              # IMAP adapter, models, sanitization
  llm/               # provider registry, prompt builder, parser, retries
  notify/            # channel registry, renderers, retry policies
  digest/            # markdown + html renderers
  orchestrator/      # pipeline composition, concurrency, partial failures
  httpapi/           # control-plane handlers
  webhooks/          # inbound webhook receivers
  actions/           # plugin interface for custom post-classification actions
  security/          # prompt-injection hardening, output sanitization
  testutil/          # fakes: clock, mail, llm, notifier, store
```

## 5. Core Components

### 5.1 Configuration (`internal/config`)

- Layered sources, later overrides earlier: defaults → YAML file → env vars → CLI flags.
- Typed schema with validation.
- Secrets flagged `sensitive:"true"` are redacted in logs.
- Hot-reloadable subset (log level, concurrency) via SIGHUP.
- Schema sections: `llm`, `imap`, `telegram`, `slack`, `webhook`, `storage`, `schedule`, `digest`, `labels`, `prompts`.

### 5.2 State Store (`internal/store`)

- SQLite database, single file at `--state-path` (default `./state/emailer.db`).
- Tables:
  - `runs(id, started_at, finished_at, status, message_count, error)`
  - `processed_messages(run_id, account_label, uid, classification, digest_excerpt, processed_at)`
  - `flags_applied(account_label, uid, flag, applied_at)`
  - `digests(run_id, channel, status, payload_hash)`
- Index on `(account_label, uid)` for dedup lookups.
- Migrations via `golang-migrate`.
- `--stateless` flag uses an in-memory no-op store.

### 5.3 Ingest Service (`internal/mail`)

- Interface:
  ```text
  type Ingester interface {
      Fetch(ctx, account, opts) ([]Message, error)
      ApplyFlags(ctx, account, flags) error
  }
  ```
- One persistent IMAP client per account per run (single dial for fetch + flag).
- STARTTLS upgrade path when only plaintext port is available.
- OAuth2 bearer token support for Gmail and Microsoft.
- Configurable folder list per account (default `INBOX`).
- Configurable time window (default 24h).
- MIME-aware body extraction with charset conversion to UTF-8.
- Attachment metadata captured (filename, mime, size); bodies not loaded.
- Concurrency: bounded worker pool, default `min(len(accounts), 4)`.
- UID ordering: fetch via `SORT` if server supports `SORT=ARRIVAL`, else sort client-side by internal date.

### 5.4 Reason Service (`internal/llm`)

- Provider registry; each provider implements:
  ```text
  type Provider interface {
      Name() string
      Classify(ctx, Request) (Response, error)
      Stream(ctx, Request, chan<- Token) error  // optional
  }
  ```
- Built-in providers: Gemini, OpenAI, Anthropic, Ollama, OpenRouter, Mistral.
- Composite key `(account_label, uid)` in every payload and response.
- Prompt builder wraps each email in unique delimiters and isolates instructions above and below the data block.
- Token budgeter computes per-message cost; batches split before provider call.
- Streaming supported for providers that expose it.
- Ensemble mode: call N providers, vote by majority, persist disagreement.
- Retry policy: 3 attempts, jittered exponential backoff (base 1s, factor 2, jitter ±25%), only on 429/5xx/network.
- Circuit breaker per provider: opens after 5 consecutive failures, half-open after 30s.
- Output validated against JSON schema before use; fallback to repair prompt on parse failure.

### 5.5 Act Service (`internal/actions` + `internal/mail`)

- Custom IMAP keywords (no backslash prefix): `Useful`, `ToDelete`, `Ads`, plus user-defined.
- Flag writes batched per account in a single `UID STORE`.
- Plugin interface:
  ```text
  type Action interface {
      Execute(ctx, Message, Classification) error
  }
  ```
- Built-in plugins: flag-writer, telegram-alert, archive, move-to-folder.

### 5.6 Notify Service (`internal/notify`)

- Channel registry; each channel implements:
  ```text
  type Channel interface {
      Name() string
      Send(ctx, Digest) error
  }
  ```
- Built-in channels: Telegram (document), Telegram (message), Slack, Email (SMTP), Webhook, File.
- Renderers (`internal/digest`): Markdown, HTML, plain text.
- Retry policy: 3 attempts, jittered backoff, same shape as LLM.
- Failure of one channel does not block others.
- Alert channel is a special configuration: if the run fails before producing a digest, an alert is sent to the configured alert channel.

### 5.7 Orchestrator (`internal/orchestrator`)

- Composes ingest → reason → act → notify.
- Emits a run ID at start; propagates via context and logs.
- Partial failure: any per-account ingest failure is logged, alert channel notified, and the run continues with remaining accounts.
- LLM failure: digest is rendered from a fallback template ("classification unavailable, listing messages"), run status marked `degraded`.
- Notify failure: run status marked `notify_failed` but exits 0 if actions already applied.
- Concurrency bounded by `--concurrency` flag.
- Graceful shutdown: cancels context on SIGINT/SIGTERM, drains in-flight work, persists run status.

### 5.8 Control Plane (`internal/httpapi`)

- `POST /run` — trigger an ad-hoc run.
- `GET /healthz` — liveness.
- `GET /readyz` — readiness (checks store reachable).
- `GET /metrics` — Prometheus.
- `POST /webhook/imap` — inbound webhook from provider push services.
- Auth via bearer token or mTLS, configurable.

## 6. Concurrency Model

- Orchestrator-level: bounded worker pool for account ingestion.
- Per-account: single IMAP connection, sequential within account (IMAP servers handle pipelining poorly in general).
- LLM calls: bounded semaphore, default 4 concurrent provider calls.
- Notifier calls: fan-out across channels, join with `errgroup`.
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
| Notify channel fails | Retry 3×, log, continue with other channels. |
| All notify channels fail | Status `notify_failed`, exit 0 if actions applied. |
| Context cancelled | Drain in-flight work, persist run status, exit 130. |

## 8. Resilience Patterns

- Retry with jittered exponential backoff.
- Circuit breaker per external dependency.
- Timeout per stage (ingest 60s, reason 120s, act 30s, notify 30s).
- Bulkhead via bounded semaphores per stage.
- Idempotency via `(account_label, uid)` dedup index.
- Fallback digest template on LLM failure.
- Health checks before run; skip run if store unreachable.

## 9. Security Considerations

- Secrets redacted in logs (`sensitive:"true"` tag).
- API keys sent in `Authorization` header, never in query string.
- IMAP credentials never logged.
- Prompt injection: email bodies wrapped in unique delimiters, instructions isolated, output schema-validated, control characters stripped, model output never executed.
- HTTP control plane authenticated.
- SQLite file mode 0600.
- Docker image runs as non-root UID.

## 10. Observability

- `slog` JSON handler with run-id, account, stage, duration fields.
- Prometheus metrics: `emailer_runs_total`, `emailer_run_duration_seconds`, `emailer_messages_fetched_total`, `emailer_llm_calls_total`, `emailer_llm_latency_seconds`, `emailer_flags_applied_total`, `emailer_notify_send_total`, `emailer_errors_total{stage}`.
- OpenTelemetry traces per run, per stage, per external call.
- Structured run summary logged at end of every run.

## 11. Deployment

- Docker image: `gcr.io/distroless/static-debian12:nonroot`, under 15 MB.
- Two deployment modes:
  - **Cron mode**: `cmd/emailer` one-shot, scheduled by Render Cron Jobs or systemd timer.
  - **Service mode**: `cmd/server` long-running, scheduled internally via `robfig/cron`, exposes HTTP control plane.
- Secrets injected via environment or mounted secret files.
- SQLite volume mounted for persistence.

## 12. Testing Strategy

- Unit tests per package with fakes from `internal/testutil`.
- Integration tests with `testcontainers` for SQLite and a mock IMAP server.
- Contract tests per LLM provider using recorded HTTP fixtures.
- End-to-end test with fake mail server, fake LLM, and fake notifier.
- Coverage gate at 80%.
- Fuzz tests for prompt builder, response parser, and MIME parser.

## 13. Known Limitations

- No full-text search of digests (future: SQLite FTS5).
- No multi-tenant isolation (single user assumed).
- No mobile push channel (future: NTFY, Pushover).
- No interactive classification correction UI (future: web dashboard).
- OAuth2 token refresh is best-effort; expired tokens surface as ingest failures.
