# Deployment

The Email AI Agent is a one-shot CLI binary designed to be executed by an
OS-level scheduler. This directory contains configuration for systemd,
the recommended scheduler on Linux.

## Prerequisites

- A compiled binary of `emailer` (see [Building](../README.md#building) in the
  root README).
- A valid YAML config file (see [Configuration](../docs/configuration.md)).
- A systemd-compatible Linux distribution (systemd v240+ recommended).

## systemd Setup

### 1. Install the binary

```bash
sudo cp emailer /usr/local/bin/emailer
sudo chmod 755 /usr/local/bin/emailer
```

### 2. Create the system user

```bash
sudo useradd --system --no-create-home --shell /usr/sbin/nologin emailer
```

### 3. Create the config file

```bash
sudo mkdir -p /etc/emailer
sudo cp config.yaml /etc/emailer/config.yaml
sudo chown -R emailer:emailer /etc/emailer
sudo chmod 600 /etc/emailer/config.yaml
```

### 4. Install the systemd units

```bash
sudo cp deploy/systemd/emailer.service /etc/systemd/system/emailer.service
sudo cp deploy/systemd/emailer.timer  /etc/systemd/system/emailer.timer
sudo systemctl daemon-reload
```

### 5. Enable and start the timer

```bash
sudo systemctl enable --now emailer.timer
```

### 6. Verify

```bash
# Check timer status
sudo systemctl status emailer.timer

# Check the last run
sudo journalctl -u emailer.service -n 50 --no-pager
```

## Manual Triggering

To run the agent immediately (for testing or ad-hoc execution):

```bash
sudo systemctl start emailer
```

To see what the next scheduled runs are:

```bash
sudo systemctl list-timers --all | grep emailer
```

## Cron Alternative

If systemd is not available, use a traditional cron entry:

```bash
# Run at 02:00, 08:00, 14:00, 20:00 daily
0 2,8,14,20 * * * /usr/local/bin/emailer --config /etc/emailer/config.yaml
```

## Docker Deployment

A multi-stage `Dockerfile` is provided at the repository root for
containerized environments. See the [Dockerfile](../Dockerfile) for details.

When running in Docker, mount the config file and state directory:

```bash
docker run --rm \
  -v /host/config.yaml:/etc/emailer/config.yaml:ro \
  -v /host/state:/var/lib/emailer \
  emailer --config /etc/emailer/config.yaml
```

## Configuration File Locations

The agent searches for configuration in this order (later overrides earlier):

1. **Defaults** (built-in)
2. **YAML/JSON file** specified by `--config <path>`
3. **Environment variables** (see `.env.example`)
4. **CLI flags** (explicit arguments)

Secrets (API keys, passwords) should be provided via environment variables
or a restricted config file (`chmod 600`).

## Exit Codes

| Code | Meaning |
|------|---------|
| 0    | Success (completed, degraded, or partial) |
| 1    | Fatal error (ingest failed, config error, store error) |
| 2    | CLI flag or config validation error |
| 130  | Cancelled (SIGINT/SIGTERM) |

## State Directory

By default, the SQLite database is stored at `./state/emailer.db`. In
production, place it in a persistent, backed-up location:

```yaml
storage:
  state_path: /var/lib/emailer/emailer.db
```

The state file should be readable/writable only by the `emailer` user.