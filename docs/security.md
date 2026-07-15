# Security

## Threat Model

The Email AI Agent processes sensitive email data and uses external API
services. This document describes the threat model and security measures
in place.

### Assets

| Asset | Sensitivity | Location |
|-------|-------------|----------|
| IMAP credentials | High | Config file, env vars |
| LLM API keys | High | Config file, env vars |
| Email content | Medium | Transiently in memory, passed to LLM |
| Telegram bot token | High | Config file, env vars |
| SQLite state file | Medium | Specified path (default `./state/emailer.db`) |
| Logs | Medium | stdout (captured by systemd journal) |

### Threat Scenarios

| Threat | Impact | Mitigation |
|--------|--------|------------|
| Config file leak | Credential exposure | File mode 0600, secrets in env vars |
| Log exposure | Credential leakage | All secrets redacted in logs |
| Transcript leak to LLM | Email content exposed | Data in transit is TLS-encrypted |
| SQLite file theft | Run history exposed | File mode 0600, no email bodies stored |
| Prompt injection | LLM misclassification | Email bodies wrapped in delimiters, isolated instructions |
| Network eavesdropping | Credential interception | TLS for IMAP, HTTPS for LLM/Telegram |
| Rogue system user | Full compromise | systemd sandboxing (NoNewPrivileges, ProtectSystem, etc.) |

## Security Measures

### 1. Secret Redaction

All fields tagged with `sensitive:"true"` in the config struct are
automatically redacted from structured logs. This includes:

- IMAP passwords
- LLM API keys
- Telegram bot tokens

Redaction uses regex patterns compiled from the config at startup. Matching
values are replaced with `[REDACTED]` in all log output.

### 2. API Key Transport

- **Gemini**: API key sent via `x-goog-api-key` header — never in URL.
- Ollama/OpenRouter providers are planned (see [`TODO.md`](../TODO.md)); each
  future provider will follow the same rule — keys in headers, never in URLs.
- Generic rule: secrets never appear in URLs or query strings.

### 3. IMAP Security

- TLS is enabled by default (`use_tls: true`).
- Port 993 (IMAPS) is the default.
- STARTTLS (port 143) is supported for servers that require it.
- Authentication uses app passwords — no OAuth2 flows.
- Credentials are never logged.

### 4. Prompt Injection Hardening

The classification prompt is designed to resist injection:

- Each email body is wrapped in unique delimiters.
- Classification instructions are isolated from email content.
- The LLM output is validated against a JSON schema before use.
- Invalid outputs trigger a repair attempt with a re-prompt.
- If repair fails, the message is classified as `Unknown` and flagged.

### 5. File System Security

- The SQLite state file should be created with mode 0600.
- The config file should be readable only by the running user.
- systemd service files include sandboxing directives:
  - `NoNewPrivileges=yes`
  - `ProtectSystem=strict`
  - `ProtectHome=yes`
  - `PrivateDevices=yes`
  - `PrivateTmp=yes`

### 6. Network Security

- All network calls have configurable timeouts (default 120s for LLM, 30s for IMAP).
- TLS is used for all external connections.
- Retry policy includes jittered backoff to avoid thundering-herd.

### 7. Data Retention

- Email bodies are processed in memory and are **not** stored in the SQLite database.
- The SQLite database stores only:
  - Run metadata (timestamps, status, message counts)
  - Message identifiers (account_label, uid, classification)
  - Short digest excerpts (configurable, default 500 chars)
  - Flag application records
  - Digest delivery status
- Logs may contain email subject lines (not bodies) at debug level.

## Operational Security

### Running as a Dedicated User

Create a dedicated system user for the agent:

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin emailer
```

### Config File Permissions

```bash
sudo chmod 600 /etc/emailer/config.yaml
sudo chown emailer:emailer /etc/emailer/config.yaml
```

### State File Permissions

The state directory is created with mode 0700 by systemd's `StateDirectory`.
If managing manually:

```bash
sudo mkdir -p /var/lib/emailer
sudo chmod 700 /var/lib/emailer
sudo chown emailer:emailer /var/lib/emailer
```

### Logging

Logs are sent to stdout and captured by the systemd journal. They contain
structured JSON with redacted secrets.

### Environment Variables

Use environment variables for secrets instead of the config file:

```bash
export EMAILER_LLM_API_KEY=your-key
export EMAILER_NOTIFY_TELEGRAM_BOT_TOKEN=your-token
```

This keeps secrets out of the config file, which may be checked into
version control.

## Reporting Vulnerabilities

See [SECURITY.md](../SECURITY.md) for the vulnerability reporting policy.