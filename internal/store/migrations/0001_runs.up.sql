CREATE TABLE IF NOT EXISTS runs (
    id          TEXT PRIMARY KEY,
    started_at  TEXT NOT NULL,   -- ISO 8601 timestamp
    finished_at TEXT,            -- ISO 8601 timestamp, NULL while running
    status      TEXT NOT NULL DEFAULT 'running',
    message_count INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT ''
);