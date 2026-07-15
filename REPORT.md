# Emailer Product Tasks

This document describes the next product tasks for the email digest pipeline. Each task is written so another model or engineer can implement it without needing prior context from chat.

## User-requested tasks

### Task 1: Add a global statistics block for all mailboxes

**Goal:** Show one high-level statistics block at the top of every digest, aggregated across all configured IMAP accounts.

**Current behavior:** The app fetches emails and renders a flat digest grouped by classification label. The digest only shows basic run-level counters such as fetched, classified, and failed counts.

**Required behavior:** The digest must start with an "Overall statistics" section that summarizes the entire run across all mailboxes.

**Implementation details:**

- Add a structured global statistics model to the digest data layer.
- Calculate global totals after fetching and classification are complete.
- Include at least these metrics:
  - number of configured accounts checked;
  - number of accounts successfully fetched;
  - number of accounts that failed;
  - total messages fetched;
  - total messages classified;
  - total messages that failed classification;
  - count by classification label, for example `Useful`, `Ads`, `ToDelete`, `Unknown`, and custom labels;
  - unread count and read count;
  - high-priority count if priority detection is implemented.
- Render this block before account-level sections and before individual email details.
- Keep the output concise enough for Telegram.

**Acceptance criteria:**

- A digest with messages from multiple accounts shows one aggregated statistics block.
- Label counts in the global block match the sum of all individual message classifications.
- If one account fails but others succeed, the global block shows both successful and failed account counts.
- Existing digest rendering tests are updated or new tests are added.

---

### Task 2: Add a statistics block for each mailbox

**Goal:** Show a separate statistics block for every configured mailbox/account in the digest.

**Current behavior:** Messages have an `AccountLabel`, but the digest does not present account-level statistics as first-class output.

**Required behavior:** After the global statistics block, the digest must show a compact section for each account.

**Implementation details:**

- Add an account statistics model to the digest data layer.
- Calculate account-level totals from fetched messages, classifications, and fetch errors.
- Include at least these metrics per account:
  - account label;
  - fetch status: success, partial, or failed;
  - number of messages fetched;
  - number of messages classified;
  - number of messages that failed classification;
  - count by classification label;
  - read count and unread count;
  - optional top sender/domain if sender aggregation is implemented.
- If an account fetch fails, render a visible warning under that account instead of silently omitting it.
- Make sure accounts with zero new messages can still appear if that is useful for clarity.

**Acceptance criteria:**

- A digest generated from two or more accounts includes a separate account stats section for each account.
- If an account has no new messages, its section clearly says so.
- If an account fails to fetch, the digest shows the account label and failure status.
- Account-level counts sum correctly into global counts.

---

### Task 3: Replace raw email text with summary and key points for each email

**Goal:** For each email in the digest, show an LLM-generated summary and key points instead of dumping or truncating raw body text.

**Current behavior:** The digest renders an excerpt from the raw email body. The LLM currently returns classification fields such as label, confidence, and reason, but not a dedicated summary or key points.

**Required behavior:** Each email entry in the digest must include:

- a short summary;
- a list of key points;
- the existing classification label, confidence, and reason;
- optionally the raw excerpt only as a fallback when summary generation fails.

**Implementation details:**

- Extend the LLM prompt so the model returns structured fields for `summary` and `key_points` for every email.
- Extend the LLM parser to parse and validate these fields.
- Store the parsed summary and key points in the message entry used by the digest renderer.
- Update the Markdown template to render summary and key points under each email.
- Keep summaries short, for example one to three sentences.
- Limit key points to a configurable small number, for example three to five bullet points.
- If the LLM fails or omits fields, use safe fallback text instead of breaking the whole digest.

**Acceptance criteria:**

- Every successfully analyzed email shows a summary and key points in the digest.
- Raw email body text is not shown as the primary content for analyzed emails.
- Malformed or missing summary fields are handled gracefully.
- Parser, prompt, and renderer tests cover the new fields.

---

## Additional recommended tasks

### Task 4: Introduce a structured email analysis model

**Goal:** Avoid overloading the existing classification model by introducing a structured model that represents the full LLM analysis of an email.

**Why this is needed:** Classification is only one part of the desired output. Summary, key points, action items, urgency, and other fields should have explicit places in the domain model.

**Implementation details:**

- Create a new domain type such as `EmailAnalysis` or extend the current classification result carefully.
- The model should include:
  - message key/account/UID;
  - label;
  - confidence;
  - reason;
  - summary;
  - key points;
  - action items;
  - urgency or priority;
  - optional parser warnings.
- Update LLM response parsing to produce this structured model.
- Update orchestrator code to pass the structured analysis to digest rendering and flag application.
- Keep IMAP flag application based on the classification label.

**Acceptance criteria:**

- The code has a clear type that represents full email analysis, not just classification.
- Digest rendering no longer depends on raw body excerpts for successfully analyzed emails.
- Existing classification behavior and IMAP flag behavior continue to work.

---

### Task 5: Add account-level error reporting to the digest

**Goal:** Make partial failures visible to the user inside the digest, not only in logs.

**Why this is needed:** If one mailbox fails to fetch, the current digest may look like fewer emails arrived. The user needs to know that a mailbox was unavailable or failed.

**Implementation details:**

- Preserve account fetch errors from the fetch stage until digest rendering.
- Add account status and error message fields to account statistics.
- Render a warning for every failed account.
- Use safe, non-sensitive error messages. Do not expose passwords, tokens, or full connection strings.
- Include failed accounts in global account counts.

**Acceptance criteria:**

- If one account fails and another succeeds, the digest is still sent and clearly marks the failed account.
- Error output is useful but does not leak secrets.
- Tests cover a partial fetch failure scenario.

---

### Task 6: Add priority or urgency detection

**Goal:** Identify which emails need immediate attention and make them prominent in the digest.

**Why this is needed:** A summary-only digest can still require too much scanning. Priority detection helps the user focus on urgent or important emails first.

**Implementation details:**

- Extend the LLM prompt and response schema with a priority field.
- Use a small controlled vocabulary, for example:
  - `high`;
  - `medium`;
  - `low`;
  - `unknown`.
- Tell the LLM to mark emails as high priority when they include deadlines, direct requests, security issues, payment issues, legal/account access issues, or time-sensitive work.
- Validate priority values in the parser.
- Render high-priority emails in a dedicated "Needs attention" section near the top of the digest.
- Include priority counts in global and account statistics.

**Acceptance criteria:**

- Parsed analyses include a validated priority value.
- High-priority emails are easy to see near the top of the digest.
- Invalid priority values do not break the full run and are handled as `unknown` or parser errors according to the chosen policy.

---

### Task 7: Add action item extraction

**Goal:** Extract concrete next steps from emails and show them separately from general key points.

**Why this is needed:** Key points summarize content, but action items tell the user what to do next.

**Implementation details:**

- Extend the LLM response schema with `action_items`, an array of short strings.
- Prompt the LLM to return an empty array when no action is needed.
- Render action items under each email only when the list is non-empty.
- Keep action items concise and imperative, for example "Reply with availability" or "Review invoice attachment".
- Add action item counts to stats if useful.

**Acceptance criteria:**

- Emails that require a response or task show action items.
- Informational emails can have an empty action item list.
- The digest template does not show empty action sections.

---

### Task 8: Add Telegram-safe digest length controls

**Goal:** Prevent long digests from failing delivery or becoming unreadable in Telegram.

**Why this is needed:** Adding statistics, summaries, key points, and action items can greatly increase message length.

**Implementation details:**

- Add configurable renderer limits, such as:
  - maximum number of detailed emails;
  - maximum summary length;
  - maximum number of key points per email;
  - maximum number of action items per email;
  - maximum rendered digest length.
- In the Telegram notification layer, handle oversized payloads by splitting messages or sending a document fallback.
- Add clear truncation indicators such as "and 12 more emails not shown".
- Ensure Markdown formatting remains valid after splitting or truncation.

**Acceptance criteria:**

- Very large digests do not fail silently.
- Telegram delivery succeeds for oversized runs through splitting or fallback behavior.
- Tests cover truncation and oversized digest delivery behavior.

---

### Task 9: Add digest configuration options

**Goal:** Allow users to control which digest sections are shown and how verbose the output is.

**Why this is needed:** Different users may want a short digest, a full digest, a stats-only digest, or only high-priority messages.

**Implementation details:**

- Add a digest configuration section to YAML/JSON config.
- Suggested settings:
  - `include_global_stats`;
  - `include_account_stats`;
  - `include_summaries`;
  - `include_key_points`;
  - `include_action_items`;
  - `include_raw_excerpt_fallback`;
  - `max_messages`;
  - `max_key_points_per_message`;
  - `max_action_items_per_message`;
  - `priority_only`.
- Add defaults that preserve useful behavior without requiring new config.
- Update config validation, example config, and documentation.
- Wire configuration into renderer construction.

**Acceptance criteria:**

- Users can enable or disable major digest sections through config.
- Missing config uses safe defaults.
- Invalid config values produce clear validation errors.
- Documentation and example config describe the new options.

---

### Task 10: Add a "what changed since last digest" highlights section

**Goal:** Show concise run-level highlights that explain what is new or unusual compared with previous runs.

**Why this is needed:** The app is intended to run periodically. Users benefit from knowing the delta, not only the current state.

**Implementation details:**

- Use stored run history and current run data to generate highlights.
- Examples of highlights:
  - "3 high-priority emails require attention";
  - "work account failed to fetch";
  - "No useful emails found";
  - "Ads increased compared with the previous run";
  - "5 emails from the same sender".
- Add a `Highlights` field to digest data.
- Render highlights near the top of the digest, after global stats or before them.
- Keep highlight generation deterministic and testable.

**Acceptance criteria:**

- The digest includes a short highlights section when there is useful information to show.
- Highlights are omitted or replaced with a neutral message when there is nothing notable.
- Tests cover at least normal, no-new-mail, partial-failure, and high-priority scenarios.

---

### Task 11: Add sender and domain aggregation

**Goal:** Show which senders or domains are producing the most email in the current run.

**Why this is needed:** Sender/domain aggregation helps identify noisy newsletters, automated systems, spammy domains, and important frequent contacts.

**Implementation details:**

- Parse sender email addresses and domains from message metadata.
- Normalize domains to lower case.
- Handle malformed or missing sender fields safely.
- Calculate top senders and top domains globally and per account.
- Render a compact "Top senders" or "Noisiest domains" block in the statistics section.
- Do not let this section dominate the digest; limit it to a small number of entries.

**Acceptance criteria:**

- Stats include top senders or domains when enough data exists.
- Malformed sender values do not crash digest generation.
- Tests cover normal addresses, display names, empty sender values, and malformed sender values.

---

### Task 12: Version the LLM response schema

**Goal:** Make future changes to the LLM JSON response safer and easier to migrate.

**Why this is needed:** The response schema will grow from classification-only to full analysis. Without schema versioning, old responses, malformed responses, and future migrations become harder to debug.

**Implementation details:**

- Add a top-level `schema_version` field to the LLM JSON response.
- Update the prompt to require the current schema version.
- Update the parser to validate the schema version.
- Decide whether to support the old classification-only schema as a backward-compatible fallback.
- Update the repair prompt so it asks for the current schema version.
- Add tests for valid current schema, missing schema version, unsupported schema version, and old schema fallback if supported.

**Acceptance criteria:**

- Valid LLM responses include the expected schema version.
- Unsupported schema versions produce clear parser errors or controlled fallback behavior.
- Parser and repair prompt tests cover schema version handling.

---

### Task 13: Add robust fallback behavior for partial LLM analysis failures

**Goal:** Keep the digest useful even when the LLM returns incomplete, malformed, or partially invalid analysis for some emails.

**Why this is needed:** With a larger JSON schema, the probability of partial parse failures increases. One bad item should not discard all successful analyses if safe partial handling is possible.

**Implementation details:**

- Decide a policy for partial LLM failures:
  - accept valid items and mark invalid items as failed;
  - retry repair once;
  - fallback only failed items to raw excerpt;
  - fallback the entire digest only when no valid items remain.
- Track parser warnings or per-message analysis errors.
- Include failed analysis counts in global and account stats.
- Render a clear fallback block for emails that could not be summarized.
- Make sure the run status reflects degraded behavior when appropriate.

**Acceptance criteria:**

- If the LLM returns valid analysis for most emails and invalid data for one email, the digest still uses valid analyses.
- Failed items are counted and visible in stats.
- The user receives a digest rather than losing the whole run when partial analysis is recoverable.
