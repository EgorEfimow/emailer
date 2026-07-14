CREATE TABLE IF NOT EXISTS digests (
    run_id       TEXT NOT NULL REFERENCES runs(id),
    channel      TEXT NOT NULL,            -- e.g. "telegram"
    status       TEXT NOT NULL,            -- "sent", "failed", "skipped"
    payload_hash TEXT NOT NULL             -- SHA-256 hex of the rendered payload
);