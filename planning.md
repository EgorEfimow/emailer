# Planning

This document is the canonical, step-by-step, smallest-possible-increment
plan for building the Email AI Agent. Every step is a single, testable,
mergeable change. No step depends on a later step. No step combines two
concerns.

Legend:
- `[ ]` not started
- `[x]` done (update as we go)

## Phase 1 — Structured Email Analysis Model

### Branch: `feat/email-analysis-model`
- [ ] 1.1 Create an `EmailAnalysis` domain type in `internal/llm` with fields: `MessageKey`, `Label`, `Confidence`, `Reason`, `Summary`, `KeyPoints []string`, `ActionItems []string`, `Priority`, `Warnings []string`.
- [ ] 1.2 Update the LLM `Response` type to carry `[]EmailAnalysis`.
- [ ] 1.3 Update `ParseResponse` (`internal/llm/parse.go`) to build `EmailAnalysis`, mapping classification plus new fields.
- [ ] 1.4 Add unit tests for `EmailAnalysis` construction and per-field mapping.
- [ ] 1.5 Update the orchestrator to pass `EmailAnalysis` (not raw classification) to the digest renderer and flag application.
- [ ] 1.6 Add unit tests for orchestrator wiring with the new model.
- [ ] 1.7 Keep IMAP flag application based solely on the `Label` field.

## Phase 2 — Global Statistics Block

### Branch: `feat/digest-global-stats`
- [ ] 2.1 Add a `GlobalStats` model to `internal/digest` (`DigestData`): accounts checked/succeeded/failed, totals fetched/classified/failed, counts by label, read/unread counts, high-priority count.
- [ ] 2.2 Compute `GlobalStats` in the orchestrator after fetch and classification complete.
- [ ] 2.3 Render an "Overall statistics" block at the very top of the Markdown digest, before account sections and email details.
- [ ] 2.4 Add unit tests asserting label counts equal the sum of all per-message classifications.
- [ ] 2.5 Add a unit test for a partial-failure run showing both successful and failed account counts.

## Phase 3 — Per-Account Statistics and Error Reporting

### Branch: `feat/digest-account-stats`
- [ ] 3.1 Add an `AccountStats` model to `internal/digest`: label, fetch status (success/partial/failed), fetched/classified/failed counts, counts by label, read/unread counts, error message, optional top sender/domain.
- [ ] 3.2 Preserve account fetch errors from the ingest stage through to rendering (Task 5).
- [ ] 3.3 Compute `AccountStats` per account in the orchestrator.
- [ ] 3.4 Render a compact per-account section after the global block; render a visible warning for failed accounts (no secrets leaked).
- [ ] 3.5 Ensure zero-message accounts can still render a clear "no new messages" line.
- [ ] 3.6 Add unit tests: per-account section per account, failed account marked, zero-message account labeled.

## Phase 4 — Summary and Key Points per Email

### Branch: `feat/llm-summary-keypoints`
- [ ] 4.1 Extend the LLM prompt (`internal/llm/prompt.go`) to request `summary` (1–3 sentences) and `key_points` (3–5 bullets) per email.
- [ ] 4.2 Extend `EmailAnalysis` with `Summary` and `KeyPoints`, and update parser validation.
- [ ] 4.3 Store summary/key points in message entries consumed by the renderer.
- [ ] 4.4 Update the Markdown template to render summary and key points per email under the classification.
- [ ] 4.5 Use the raw excerpt only as fallback when summary generation fails.
- [ ] 4.6 Add parser, prompt, and renderer tests for the new fields and graceful fallback.

## Phase 5 — Priority / Urgency Detection

### Branch: `feat/llm-priority`
- [ ] 5.1 Add a `Priority` field (controlled vocabulary: `high`, `medium`, `low`, `unknown`) to `EmailAnalysis` and the LLM schema.
- [ ] 5.2 Extend the prompt to mark `high` for deadlines, direct requests, security/payment/legal/account-access/time-sensitive issues.
- [ ] 5.3 Validate priority values in the parser; invalid → `unknown` per policy.
- [ ] 5.4 Render a dedicated "Needs attention" section near the top for high-priority emails.
- [ ] 5.5 Include priority counts in global and account statistics.
- [ ] 5.6 Add parser and renderer tests for the priority vocabulary and fallback.

## Phase 6 — Action Item Extraction

### Branch: `feat/llm-action-items`
- [ ] 6.1 Add `ActionItems []string` to `EmailAnalysis` and the LLM schema (empty array when none).
- [ ] 6.2 Extend the prompt to return concise imperative action items.
- [ ] 6.3 Update parser validation for `action_items`.
- [ ] 6.4 Render action items under each email only when the list is non-empty.
- [ ] 6.5 Optionally add action item counts to stats.
- [ ] 6.6 Add tests covering empty vs. populated action items and template suppression of empty sections.

## Phase 7 — Sender and Domain Aggregation

### Branch: `feat/digest-sender-aggregation`
- [ ] 7.1 Parse sender addresses/domains from message metadata; normalize domains to lowercase; handle malformed/missing safely.
- [ ] 7.2 Compute top senders and top domains globally and per account (bounded to a small number of entries).
- [ ] 7.3 Add top-sender/domain fields to `GlobalStats`/`AccountStats`.
- [ ] 7.4 Render a compact "Top senders" / "Noisiest domains" block in the statistics section.
- [ ] 7.5 Add tests for normal addresses, display names, empty, and malformed sender values.

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
