CREATE TABLE IF NOT EXISTS run_digest_summaries (
    run_id              TEXT PRIMARY KEY,
    finished_at         TEXT NOT NULL,
    accounts_failed     INTEGER NOT NULL DEFAULT 0,
    high_priority_count INTEGER NOT NULL DEFAULT 0,
    payload_json        TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS run_digest_label_counts (
    run_id TEXT NOT NULL,
    label  TEXT NOT NULL,
    count  INTEGER NOT NULL,
    PRIMARY KEY (run_id, label)
);

CREATE TABLE IF NOT EXISTS run_digest_sender_counts (
    run_id TEXT NOT NULL,
    sender TEXT NOT NULL,
    count  INTEGER NOT NULL,
    PRIMARY KEY (run_id, sender)
);

CREATE TABLE IF NOT EXISTS run_digest_domain_counts (
    run_id TEXT NOT NULL,
    domain TEXT NOT NULL,
    count  INTEGER NOT NULL,
    PRIMARY KEY (run_id, domain)
);

CREATE INDEX IF NOT EXISTS idx_run_digest_summaries_finished_at
    ON run_digest_summaries (finished_at);
