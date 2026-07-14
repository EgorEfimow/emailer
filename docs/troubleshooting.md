# Troubleshooting

Common issues and their solutions.

## Configuration Errors

### "configuration error: ..."

The config validation failed. Common causes:

- **Missing required fields**: Ensure all required fields are set (IMAP host,
  username, password, LLM API key, Telegram bot token and chat ID).
- **Invalid port**: Port must be between 1 and 65535, or 0 for default.
- **Invalid duration**: Duration strings like `72h`, `30m`, `3600s` are
  expected. Use Go duration format.
- **Invalid JSON**: If using `EMAILER_IMAP_ACCOUNTS` env var, ensure it's valid
  JSON.

### "llm provider not found"

The provider specified in the config is not registered. See
[providers.md](providers.md) for supported providers.

Ensure the provider name exactly matches one of: `gemini`, `ollama`,
`openrouter`.

## IMAP Connection Issues

### "dial failed" or "login failed"

- **Network connectivity**: Ensure the IMAP server is reachable from your
  network. Test with `openssl s_client -connect imap.example.com:993`.
- **App password**: Many providers (Gmail, Outlook) require an app-specific
  password, not your regular account password.
- **TLS settings**: If the server uses STARTTLS on port 143, set `use_tls: true`
  and port 143. If the server uses plain IMAP (not recommended), set
  `use_tls: false`.
- **Firewall**: Ensure outbound access to the IMAP port is allowed.
- **2FA/MFA**: If 2FA is enabled on the account, you must use an app password.

### "no messages fetched"

- **Check the time window**: Use `--window 168h` to fetch a full week. If using
  dynamic window, the first run defaults to 24h.
- **Check the folder name**: Ensure the folder exists (default is `INBOX`,
  case-sensitive for some servers).
- **Check `fetch_unread_only`**: If set to `true`, only unread messages are
  fetched.
- **First run**: On first run, only the last 24 hours of messages are fetched.

## LLM Issues

### "LLM returned invalid JSON"

- The LLM response did not match the expected JSON schema.
- The agent will attempt a repair with a re-prompt.
- If repair fails, the message is classified as `Unknown`.
- **Model choice**: Some smaller models struggle with JSON formatting. Try a
  larger model (e.g., `gemini-2.0-flash` instead of `gemini-1.5-flash`).

### "LLM provider creation failed"

- **API key**: Ensure the API key is correct and has not expired.
- **Endpoint**: If using a custom endpoint, verify it's reachable.
- **Model**: Verify the model name is valid for the provider.

### "LLM request timeout"

- The default timeout is 120s. For large batches of emails, the LLM call may
  time out.
- **Solutions**:
  - Reduce the fetch window with `--window 12h`.
  - Set `fetch_unread_only: true` to process fewer messages.
  - Increase the timeout: `llm.timeout: 240s`.

## Telegram Notification Issues

### "Telegram send failed"

- **Bot token**: Ensure the bot token is correct.
- **Chat ID**: Ensure the chat ID is correct. For group chats, the ID is
  negative (e.g., `-1001234567890`).
- **Bot permissions**: Ensure the bot has permission to send messages in the
  target chat.
- **File size**: The digest file must not exceed 45 MB.
- **Network**: Ensure outbound access to `api.telegram.org` is allowed.

### "No digest received"

- Check the logs for the run status.
- If the run status is `ingest_failed`, all IMAP accounts failed.
- If the run status is `degraded`, the LLM failed but a fallback digest was
  sent.
- Check the Telegram bot hasn't been blocked by the user.

## State Store Issues

### "failed to open SQLite store"

- **Permission denied**: Ensure the state directory is writable by the running
  user.
- **Disk full**: Check available disk space.
- **Locked**: If another instance is running, the database may be locked.
  Ensure only one instance runs at a time.

### "stateless mode requires fetch_unread_only"

When `stateless: true` is set, the config must also have
`fetch_unread_only: true`. This prevents processing the same messages
multiple times without deduplication.

## Systemd Timer Issues

### "timer doesn't run"

```bash
# Check timer status
sudo systemctl status emailer.timer

# Check if the timer is enabled
sudo systemctl is-enabled emailer.timer

# List all timers
sudo systemctl list-timers --all
```

### "service fails when triggered by timer"

```bash
# Check the service logs
sudo journalctl -u emailer.service -n 100 --no-pager

# Run manually to see output
sudo systemctl start emailer
```

### "Permission denied" in journal

The systemd service runs as the `emailer` user. Ensure:

- The binary is executable by the `emailer` user.
- The config file is readable by the `emailer` user.
- The state directory is writable by the `emailer` user.

## General Debugging

### Enable debug logging

```bash
./emailer --config config.yaml --log-level debug
```

Debug logs include:
- IMAP search criteria and results
- LLM request and response details (redacted)
- Flag application attempts
- Digest content

### Force a full re-process

```bash
./emailer --config config.yaml --force-reprocess
```

This ignores the deduplication index and processes all messages in the
time window. Useful for testing after changing prompts or labels.

### Dry run

```bash
./emailer --config config.yaml --dry-run
```

This skips all side effects:
- No IMAP flags are applied.
- No Telegram message is sent.
- State is still recorded (unless `--stateless` is also set).

### Check binary version

```bash
./emailer --help
```

The help output shows the build version and available flags.

## Exit Codes Reference

| Code | Meaning | Action |
|------|---------|--------|
| 0 | Success | Everything worked. |
| 1 | Fatal error | Check logs for the error. |
| 2 | Config/flag error | Fix the config or flags. |
| 130 | Cancelled | Process was interrupted (SIGINT/SIGTERM). |