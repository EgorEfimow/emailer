CREATE TABLE IF NOT EXISTS flags_applied (
    account_label TEXT NOT NULL,
    uid           INTEGER NOT NULL,
    flag          TEXT NOT NULL,   -- e.g. "Useful", "ToDelete", "Ads"
    applied_at    TEXT NOT NULL,   -- ISO 8601 timestamp

    PRIMARY KEY (account_label, uid, flag)
);