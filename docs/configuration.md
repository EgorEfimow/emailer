# Configuration

The Email AI Agent loads configuration from four sources, in order of
increasing precedence (later overrides earlier):

1. **Built-in defaults** (see `defaults.go`)
2. **YAML / JSON file** — specified by `--config <path>`
3. **Environment variables** — prefixed with `EMAILER_`
4. **CLI flags** — passed directly to the binary

## Quick Reference

| Setting | Env var | CLI flag | Default | Sensitive |
|---------|---------|----------|---------|-----------|
| Provider | `EMAILER_LLM_PROVIDER` | `--llm-provider` | `gemini` | |
| API key | `EMAILER_LLM_API_KEY` | `--llm-api-key` | — | ✅ |
| Model | `EMAILER_LLM_MODEL` | `--llm-model` | `gemini-2.0-flash` | |
| Endpoint | `EMAILER_LLM_ENDPOINT` | `--llm-endpoint` | — | |
| LLM timeout | `EMAILER_LLM_TIMEOUT` | `--llm-timeout` | `120s` | |
| LLM max retries | `EMAILER_LLM_MAX_RETRIES` | `--llm-max-retries` | `3` | |
| LLM max concurrent | `EMAILER_LLM_MAX_CONCURRENT` | `--llm-max-concurrent` | `4` | |
| IMAP accounts | `EMAILER_IMAP_ACCOUNTS` | `--imap-*` | — | |
| IMAP command timeout | `EMAILER_IMAP_TIMEOUT` | — | `30s` | |
| Telegram bot token | `EMAILER_NOTIFY_TELEGRAM_BOT_TOKEN` | `--telegram-bot-token` | — | ✅ |
| Telegram chat ID | `EMAILER_NOTIFY_TELEGRAM_CHAT_ID` | `--telegram-chat-id` | — | |
| State path | `EMAILER_STORAGE_STATE_PATH` | `--state-path` | `./state/emailer.db` | |
| Stateless mode | `EMAILER_STORAGE_STATELESS` | `--stateless` | `false` | |
| Fetch unread only | `EMAILER_FETCH_UNREAD_ONLY` | `--fetch-unread-only` | `false` | |
| Max window | `EMAILER_MAX_WINDOW` | `--max-window` | `72h` | |
| Max message excerpt | `EMAILER_DIGEST_MAX_MESSAGE_EXCERPT` | `--digest-max-message-excerpt` | `500` | |
| Include read status | `EMAILER_DIGEST_INCLUDE_READ_STATUS` | `--digest-include-read-status` | `true` | |
| Include global stats | `EMAILER_DIGEST_INCLUDE_GLOBAL_STATS` | `--digest-include-global-stats` | `true` | |
| Include account stats | `EMAILER_DIGEST_INCLUDE_ACCOUNT_STATS` | `--digest-include-account-stats` | `true` | |
| Include summaries | `EMAILER_DIGEST_INCLUDE_SUMMARIES` | `--digest-include-summaries` | `true` | |
| Include key points | `EMAILER_DIGEST_INCLUDE_KEY_POINTS` | `--digest-include-key-points` | `true` | |
| Include action items | `EMAILER_DIGEST_INCLUDE_ACTION_ITEMS` | `--digest-include-action-items` | `true` | |
| Include raw excerpt fallback | `EMAILER_DIGEST_INCLUDE_RAW_EXCERPT_FALLBACK` | `--digest-include-raw-excerpt-fallback` | `true` | |
| Max messages | `EMAILER_DIGEST_MAX_MESSAGES` | `--digest-max-messages` | `100` | |
| Max key points per message | `EMAILER_DIGEST_MAX_KEY_POINTS_PER_MESSAGE` | `--digest-max-key-points-per-message` | `5` | |
| Max action items per message | `EMAILER_DIGEST_MAX_ACTION_ITEMS_PER_MESSAGE` | `--digest-max-action-items-per-message` | `3` | |
| Priority only | `EMAILER_DIGEST_PRIORITY_ONLY` | `--digest-priority-only` | `false` | |
| Custom labels | `EMAILER_LABELS_CUSTOM` | `--labels-custom` | `[]` | |
| Concurrency accounts | `EMAILER_CONCURRENCY_MAX_ACCOUNTS` | `--concurrency-max-accounts` | `4` | |
| Concurrency LLM calls | `EMAILER_CONCURRENCY_MAX_LLM_CALLS` | `--concurrency-max-llm-calls` | `4` | |
| Fetch batch size | `EMAILER_CONCURRENCY_FETCH_BATCH_SIZE` | — | `10` | |

## Configuration File (YAML)

```yaml
# LLM Provider
llm:
  provider: gemini                # gemini (see TODO.md for ollama/openrouter)
  api_key: "your-api-key"         # 🔒 secret
  model: gemini-2.0-flash
  endpoint: ""                    # optional provider URL override
  timeout: 120s
  max_retries: 3
  max_concurrent: 4

# IMAP Accounts
imap:
  accounts:
    - label: work                 # unique identifier for dedup
      host: imap.example.com
      port: 993                   # 0 = default 993 (IMAPS)
      username: user@example.com
      password: "app-password"    # 🔒 secret
      folders:
        - INBOX
      use_tls: true

# Telegram Notification
notify:
  telegram:
    bot_token: "123456:ABC-DEF"   # 🔒 secret
    chat_id: -1001234567890

# Storage
storage:
  state_path: ./state/emailer.db
  # stateless: false

# Fetch Behaviour
fetch_unread_only: false
max_window: 72h

# Digest Rendering
digest:
  max_message_excerpt: 500        # characters per message in digest
  include_read_status: true       # show read/unread badge per message
  include_global_stats: true      # show global summary block (## Summary)
  include_account_stats: true     # show per-account stats (## Account Stats)
  include_summaries: true         # show LLM summaries per message
  include_key_points: true        # show key points per message
  include_action_items: true      # show action items per message
  include_raw_excerpt_fallback: true  # show raw excerpt when analysis fails
  max_messages: 100               # cap total messages in digest (0 = unlimited)
  max_key_points_per_message: 5   # cap key points per message (0 = unlimited)
  max_action_items_per_message: 3 # cap action items per message (0 = unlimited)
  priority_only: false            # show only high-priority messages

# Classification Labels
labels:
  custom: []
  # custom:
  #   - Urgent
  #   - Reference

# Prompt Templates
prompts:
  classification_prompt: ""       # overrides default prompt
  system_prompt: ""               # overrides default system prompt

# Concurrency
concurrency:
  max_accounts: 4
  max_llm_calls: 4
```

## Environment Variables

Every configuration option is also available as an environment variable with
the `EMAILER_` prefix. Nested keys use `_` separators:

```bash
export EMAILER_LLM_PROVIDER=gemini
export EMAILER_LLM_API_KEY=your-api-key
export EMAILER_IMAP_ACCOUNTS='[{"label":"work","host":"imap.example.com","port":993,"username":"user@example.com","password":"secret","folders":["INBOX"],"use_tls":true}]'
export EMAILER_NOTIFY_TELEGRAM_BOT_TOKEN=123456:ABC-DEF
export EMAILER_NOTIFY_TELEGRAM_CHAT_ID=-1001234567890
export EMAILER_STORAGE_STATE_PATH=./state/emailer.db
export EMAILER_FETCH_UNREAD_ONLY=false
export EMAILER_MAX_WINDOW=72h
```

## CLI Flags

All configuration options are also exposed as CLI flags. See `--help` for the
full list:

```bash
./emailer --config config.yaml \
  --llm-model gemini-2.0-flash \
  --max-window 48h \
  --log-level debug
```

## Configuration Sections

### LLM (`llm`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `provider` | string | `gemini` | LLM provider: `gemini` (others tracked in `TODO.md`) |
| `api_key` | string | — | Provider API key (sensitive) |
| `model` | string | `gemini-2.0-flash` | Model identifier |
| `endpoint` | string | — | Custom API endpoint URL |
| `timeout` | duration | `120s` | Per-request timeout |
| `max_retries` | int | `3` | Retry attempts on transient failure |
| `max_concurrent` | int | `4` | Max concurrent LLM calls |

### IMAP (`imap`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `accounts` | array | `[]` | List of IMAP accounts |
| `timeout` | duration | `30s` | Per-command timeout (dial, login, select, fetch, store); `0` = no timeout |
| — `label` | string | — | Unique account identifier (used in dedup key) |
| — `host` | string | — | IMAP server hostname |
| — `port` | int | `993` | IMAP server port (993=IMAPS, 143=STARTTLS) |
| — `username` | string | — | IMAP login username |
| — `password` | string | — | IMAP app password (sensitive) |
| — `folders` | array | `["INBOX"]` | Mailboxes to fetch from |
| — `use_tls` | bool | `true` | Use TLS for connection |

### Telegram (`notify.telegram`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `bot_token` | string | — | Telegram bot token (sensitive) |
| `chat_id` | int | — | Target chat ID for digest delivery |

### Storage (`storage`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `state_path` | string | `./state/emailer.db` | SQLite database path |
| `stateless` | bool | `false` | Disable persistence (requires `fetch_unread_only=true`) |

### Fetch Behaviour

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `fetch_unread_only` | bool | `false` | Only fetch unread messages |
| `max_window` | duration | `72h` | Cap on dynamic lookback window |

### Digest (`digest`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_message_excerpt` | int | `500` | Max characters per message in digest |
| `include_read_status` | bool | `true` | Show read/unread badge per message |
| `include_global_stats` | bool | `true` | Render global summary block (## Summary) |
| `include_account_stats` | bool | `true` | Render per-account stats (## Account Stats) |
| `include_summaries` | bool | `true` | Render LLM summaries per message |
| `include_key_points` | bool | `true` | Render key points per message |
| `include_action_items` | bool | `true` | Render action items per message |
| `include_raw_excerpt_fallback` | bool | `true` | Show raw excerpt when analysis fails |
| `max_messages` | int | `100` | Cap total messages in digest (`0` = unlimited) |
| `max_key_points_per_message` | int | `5` | Cap key points per message (`0` = unlimited) |
| `max_action_items_per_message` | int | `3` | Cap action items per message (`0` = unlimited) |
| `priority_only` | bool | `false` | Show only high-priority messages |

### Labels (`labels`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `custom` | array | `[]` | Additional classification labels beyond built-in `Useful`, `ToDelete`, `Ads` |

### Prompts (`prompts`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `classification_prompt` | string | — | Overrides default classification prompt |
| `system_prompt` | string | — | Overrides default system prompt |

### Concurrency (`concurrency`)

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `max_accounts` | int | `4` | Accounts fetched concurrently |
| `max_llm_calls` | int | `4` | Simultaneous LLM provider calls |

## Fetch Window Logic

The time window for fetching emails is determined as follows:

1. **Explicit `--window` flag**: If set, uses this value exactly. No dynamic
   logic applied.
2. **Dynamic window (default)**: Reads the `finished_at` timestamp of the last
   successful run from the SQLite store and uses it as the `Since` parameter.
3. **Fallback**: If no previous successful run exists, defaults to a 24-hour
   window.
4. **Cap**: The `max_window` setting (default 72h) caps the dynamic lookback
   period, preventing the agent from overwhelming the LLM after prolonged
   host downtime.

## Stateless Mode

When `stateless: true` is set, the agent skips SQLite persistence entirely.
No deduplication is performed, and no run history is recorded. This mode
**requires** `fetch_unread_only: true` to prevent processing the same
messages across multiple runs.

Use stateless mode for ephemeral environments or initial testing. For
production use, always run with the default stateful mode.