# Email AI Agent

A one-shot CLI tool that ingests email from multiple IMAP accounts, classifies
each message using an LLM, applies IMAP keyword flags, and delivers a digest
to Telegram.

Designed to run 4× per day via systemd timers (or cron) — no long-running
server, no HTTP API, no webhooks.

## Quickstart

### 1. Download or build

```bash
# Build from source
git clone https://github.com/egorefimow/emailer.git
cd emailer
make build

# The binary is at bin/emailer
```

Requirements: Go 1.25+.

### 2. Configure

Copy the example config and fill in your values:

```bash
cp config.example.yaml config.yaml
# Edit config.yaml — set your IMAP accounts, LLM API key, and Telegram chat
```

### 3. Run

```bash
./bin/emailer --config config.yaml
```

On first run, the agent:

1. Connects to each IMAP account and fetches emails from the last 24 hours.
2. Sends each message to the LLM for classification (`Useful`, `ToDelete`, `Ads`).
3. Applies IMAP keyword flags back to the source mailbox.
4. Sends a Markdown digest to your Telegram chat.

### 4. Schedule (optional)

```bash
# Install systemd timer (4×/day)
sudo cp deploy/systemd/emailer.service /etc/systemd/system/
sudo cp deploy/systemd/emailer.timer  /etc/systemd/system/
sudo systemctl enable --now emailer.timer
```

See [deploy/README.md](deploy/README.md) for full deployment instructions.

## Features

- **Multi-account IMAP**: Fetch from as many IMAP accounts as needed.
- **LLM classification**: Classifies emails via Gemini, Ollama, or OpenRouter.
- **IMAP keyword flags**: Tags messages with `Useful`, `ToDelete`, `Ads` (and custom labels).
- **Telegram digest**: Renders a grouped Markdown digest and sends it to Telegram.
- **Idempotent**: Tracks processed messages by `(account_label, uid)` — no duplicates.
- **Dynamic window**: Derives fetch window from the last successful run, capped at 72h.
- **Fail-soft**: Per-account and per-message failures are isolated; a partial digest is always produced.
- **Secret-safe**: All secrets redacted in logs; API keys never appear in URLs.
- **Stateful by default**: SQLite ledger for deduplication; optional `--stateless` mode.

## Documentation

| Document | Description |
|----------|-------------|
| [Configuration](docs/configuration.md) | All configuration options — env vars, YAML, CLI flags |
| [Providers](docs/providers.md) | LLM provider setup (Gemini, Ollama, OpenRouter) |
| [Security](docs/security.md) | Threat model and security practices |
| [Troubleshooting](docs/troubleshooting.md) | Common issues and solutions |
| [Architecture](architecture.md) | Architectural overview and design decisions |
| [Deployment](deploy/README.md) | systemd timer and cron setup |

## CLI Usage

```
Usage: emailer [options]

Main flags:
  --config <path>         Path to YAML or JSON config file
  --log-level <level>     Log level: debug, info, warn, error (default: info)
  --dry-run               Skip side effects (flag writes, notifications)
  --force-reprocess       Reprocess all messages, ignoring prior runs
  --window <duration>     Explicit fetch window (e.g. 24h, 72h)

Exit codes:
  0   Success (completed, degraded, partial)
  1   Fatal error (ingest failed, config error, store error)
  2   CLI flag / config validation error
  130 Cancelled (SIGINT/SIGTERM)
```

## Development

```bash
make build    # Build the binary
make test     # Run all tests with race detection
make lint     # Run golangci-lint
make fmt      # Format Go code
make tidy     # Tidy Go modules
make clean    # Clean build artifacts
```

## License

MIT — see [LICENSE](LICENSE).