# Email AI Agent Implementation Plan

This document lists **actionable tasks** for the email digest pipeline, grouped by feature area. Each task is a single, testable change. Checkboxes reflect **implementation status** (`[x]` = done, `[ ]` = not started).

---

## 1. Update LLM Response Schema and Domain Model

**Goal:** Extend the LLM response schema and domain model to include structured analysis fields (`summary`, `key_points`, `action_items`, `priority`).

- [x] In `internal/mail/types.go`, extend `Classification` with:
  ```go
  Summary     string   // concise summary of the email
  KeyPoints   []string // important facts or details from the email
  ActionItems []string // optional follow-up tasks requested by the email
  Priority    string   // priority indicator: high, medium, or low
  ```
- [x] In `internal/llm/prompt.go`, update `defaultPromptTemplate` so every email returns JSON fields:
  ```json
  {
    "summary": "...",
    "key_points": ["..."],
    "action_items": ["..."],
    "priority": "high|medium|low",
    "label": "...",
    "confidence": 0.0,
    "reason": "..."
  }
  ```
- [x] In `internal/llm/parse.go`, extend `classificationItem` and `ParseResponse` to parse and validate:
  - `summary` (non-empty string)
  - `key_points` (non-empty array of strings)
  - `action_items` (optional array of strings)
  - `priority` (one of: `high`, `medium`, `low`; invalid → item rejected)
- [x] Update tests in `internal/llm/*_test.go` and testdata under `testdata/gemini/` to include the new JSON shape.

---

## 2. Introduce Explicit Stats Structures in `internal/digest/digest.go`

**Goal:** Add structured statistics models (`DigestStats`, `AccountStats`) to track global and per-account metrics.

- [x] In `internal/digest/digest.go`, add:
  ```go
  type DigestStats struct {
    FetchedCount      int
    ClassifiedCount   int
    FailedCount       int
    ReadCount         int
    UnreadCount       int
    CountsByLabel     map[string]int
    AccountsChecked   int
    AccountsSucceeded int
    AccountsFailed    int
    HighPriorityCount int
  }
  
  type AccountStats struct {
    AccountLabel    string
    FetchedCount    int
    ClassifiedCount int
    FailedCount     int
    ReadCount       int
    UnreadCount     int
    CountsByLabel   map[string]int
    Status          string // "ok" or "error"
    Error           string // fetch error message
  }
  ```
- [x] Extend `DigestData` with:
  ```go
  GlobalStats  DigestStats
  AccountStats []AccountStats
  ```
- [x] In `internal/orchestrator/orchestrator.go`, update `buildDigestData` to aggregate stats from:
  - `messages` (fetched count, read/unread)
  - `classifications` (classified count, label counts, priority)
  - `fetchResults` (account status, errors)
- [x] In `internal/digest/markdown.go`, render:
  - Global summary block (`## Summary`) at the top
  - Account-level stats (`## Account Stats`) with warnings for failed accounts
  - Detailed message sections grouped by label
- [x] Add renderer tests in `internal/digest/markdown_test.go` for stats aggregation and error reporting.

---

## 3. Expose Account Fetch Failures to Digest Rendering

**Goal:** Make account-level fetch errors visible in the digest.

- [x] In `internal/orchestrator/orchestrator.go`, carry `accountErrors := mail.AccountErrors(fetchResults)` into `buildDigestData`.
- [x] Add account status/error fields to `digest.AccountStats`:
  ```go
  Status string // "ok" or "error"
  Error  string // fetch error message
  ```
- [x] In `internal/digest/markdown.go`, render a warning line for accounts with fetch errors:
  ```markdown
  ⚠️ **Fetch error:** connection refused
  ```
- [x] Add tests covering partial account failure in `internal/orchestrator/orchestrator_test.go` and `internal/digest/markdown_test.go`.

---

## 4. Extend LLM Analysis and Digest Sorting (Priority/Urgency)

**Goal:** Identify high-priority emails and make them prominent in the digest.

- [x] Add a `Priority` field to the parsed LLM result (`high`, `medium`, `low`).
- [x] In `internal/llm/prompt.go`, update the prompt to ask the model to identify urgent emails based on:
  - Deadlines
  - Payment/security risks
  - Direct requests
  - Calendar/time-sensitive content
  - Sender context
- [x] In `internal/llm/parse.go`, validate allowed priority values (invalid → item rejected).
- [x] In `internal/digest/markdown.go`, add a dedicated "Needs attention" section near the top for high-priority emails.
- [~] Include priority counts in global statistics (`DigestStats.HighPriorityCount` is populated; account-level counts not yet tracked).
- [x] In `internal/digest/markdown.go`, sort messages so high-priority items appear first within each label group.
- [x] Add unit tests for priority parsing and digest ordering in `internal/llm/parse_test.go` and `internal/digest/markdown_test.go`.

---

## 5. Render Summary, Key Points, and Action Items in Digest

**Goal:** Replace raw email excerpts with LLM-generated summaries and key points in the digest.

- [x] In `internal/digest/markdown.go`, update the Markdown template to render `Summary` and `KeyPoints` per email under the classification:
  ```markdown
  ### Summary
  > {{.Classification.Summary}}
  
  **Key points:**
  {{range .Classification.KeyPoints}}- {{.}}
  {{end}}
  ```
- [x] In `internal/digest/markdown.go`, render `ActionItems` under each email only when the list is non-empty:
  ```markdown
  **Action items:**
  {{range .Classification.ActionItems}}- {{.}}
  {{end}}
  ```
- [x] Use the raw excerpt only as fallback when summary generation fails.
- [x] Add renderer tests in `internal/digest/markdown_test.go` for summary/key points/action items.

---

## 6. Add Sender and Domain Aggregation

**Goal:** Show which senders/domains are producing the most email in the current run.

- [ ] In `internal/orchestrator/orchestrator.go`, parse sender addresses/domains from message metadata:
  - Normalize domains to lowercase
  - Handle malformed/missing sender fields safely
- [ ] Compute top senders and top domains globally and per account (bounded to 5 entries).
- [ ] Add top-sender/domain fields to `DigestStats` and `AccountStats`:
  ```go
  TopSenders  []string // e.g., ["sender@example.com", ...]
  TopDomains  []string // e.g., ["example.com", ...]
  ```
- [ ] In `internal/digest/markdown.go`, render a compact "Top senders" / "Noisiest domains" block in the statistics section.
- [ ] Add tests for sender/domain parsing and rendering in `internal/orchestrator/orchestrator_test.go` and `internal/digest/markdown_test.go`.

---

## Implementation Notes
- **Design divergence:** The structured analysis model was implemented by extending `mail.Classification` (not as a separate `EmailAnalysis` type). `llm.Response.Classifications` carries `[]mail.Classification`.
- **Stats models:** Global stats are named `DigestStats`; per-account stats are `AccountStats`. Both are fully implemented.
- **Rendering gaps:** Summary/key points and action items are not yet rendered in the Markdown template. Sender/domain aggregation is not implemented.

## Phase 8 — "What Changed" Highlights

### Branch: `feat/digest-highlights`
- [ ] 8.1 Add a `Highlights []string` field to `DigestData`.
- [ ] 8.2 Use stored run history (`internal/store`) plus current run to generate deterministic highlights (e.g., high-priority count, failed account, ad increase, same-sender burst).
- [ ] 8.3 Render highlights near the top; omit or show a neutral message when nothing notable.
- [ ] 8.4 Add unit tests for normal, no-new-mail, partial-failure, and high-priority scenarios.

## Phase 9 — LLM Response Schema Versioning

### Branch: `feat/llm-schema-version`
- [ ] 9.1 Add a top-level `schema_version` to the LLM JSON response and require it in the prompt.
- [ ] 9.2 Validate `schema_version` in the parser; decide backward-compatible fallback for the old classification-only schema.
- [ ] 9.3 Update the repair prompt to request the current schema version.
- [ ] 9.4 Add tests for valid current schema, missing version, unsupported version, and old-schema fallback if supported.

## Phase 10 — Robust Partial LLM Failure Fallback

### Branch: `feat/llm-partial-fallback`
- [ ] 10.1 Define a policy: accept valid analyses, mark invalid as failed, retry repair once, fallback only failed items to raw excerpt, fallback the whole digest only when no valid items remain.
- [ ] 10.2 Track per-message analysis warnings/errors on `EmailAnalysis`.
- [ ] 10.3 Count failed analyses in global and account stats; render a clear fallback block for failed emails.
- [ ] 10.4 Mark run status `degraded` when partial analysis is recoverable but degraded.
- [ ] 10.5 Add tests: one bad item among many good ones keeps the good analyses; failed items counted and visible.

## Phase 11 — Telegram-Safe Digest Length Controls

### Branch: `feat/notify-length-controls`
- [ ] 11.1 Add configurable renderer limits: max detailed emails, max summary length, max key points/action items per email, max rendered digest length.
- [ ] 11.2 In the Telegram channel (`internal/notify/telegram`), split oversized payloads or fall back to a document when over limits.
- [ ] 11.3 Add truncation indicators (e.g., "and N more emails not shown") and keep MarkdownV2 valid after splitting/truncation.
- [ ] 11.4 Add tests covering truncation and oversized-digest delivery behavior.

## Phase 12 — Digest Configuration Options

### Branch: `feat/digest-config`
- [ ] 12.1 Add a `digest` config section: `include_global_stats`, `include_account_stats`, `include_summaries`, `include_key_points`, `include_action_items`, `include_raw_excerpt_fallback`, `max_messages`, `max_key_points_per_message`, `max_action_items_per_message`, `priority_only`.
- [ ] 12.2 Provide safe defaults preserving current useful behavior.
- [ ] 12.3 Validate new options in `Validate()`; update `.env.example` and `config.example.yaml`.
- [ ] 12.4 Update `architecture.md` §5.1/§5.6 and docs (`docs/configuration.md`).
- [ ] 12.5 Wire config into renderer/channel construction.
- [ ] 12.6 Add tests for defaults, toggles, and invalid values.

## Phase 13 — Docker (Optional)

### Branch: `chore/docker`
- [ ] 13.1 Add `Dockerfile` multi-stage: `golang:1.25-alpine` build, `gcr.io/distroless/static-debian12:nonroot` runtime.
- [ ] 13.2 Add `.dockerignore`.

## Phase 14 — Hardening and Final Audit

### Branch: `chore/hardening-audit`
- [ ] 14.1 Run `golangci-lint` and resolve all findings.
- [ ] 14.2 Run `govulncheck` and resolve all findings.
- [ ] 14.3 Run `go test -race ./...`.
- [ ] 14.4 Audit all error paths for log coverage.
- [ ] 14.5 Audit all secrets for redaction coverage.
- [ ] 14.6 Audit all network calls for timeout and retry.
- [ ] 14.7 Verify `architecture.md`, `AGENTS.md`, `planning.md` reflect final state.

## Phase 15 — Release

### Branch: `release/v0.1.0`
- [ ] 15.1 Tag `v0.1.0-rc.1`.
- [ ] 15.2 Cut release candidate.
- [ ] 15.3 Run end-to-end on staging for 7 consecutive days.
- [ ] 15.4 Tag `v0.1.0`.
- [ ] 15.5 Publish release notes.
