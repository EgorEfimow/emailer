CREATE TABLE IF NOT EXISTS processed_messages (
    run_id          TEXT NOT NULL REFERENCES runs(id),
    account_label   TEXT NOT NULL,
    uid             INTEGER NOT NULL,
    is_read         INTEGER NOT NULL DEFAULT 0,  -- boolean: 1 = read, 0 = unread
    classification  TEXT NOT NULL,
    digest_excerpt  TEXT NOT NULL,
    processed_at    TEXT NOT NULL,                -- ISO 8601 timestamp

    -- Composite primary key doubles as the dedup index.
    -- A message with the same (account_label, uid) is processed once.
    PRIMARY KEY (account_label, uid)
);

CREATE INDEX IF NOT EXISTS idx_processed_messages_run_id
    ON processed_messages(run_id);