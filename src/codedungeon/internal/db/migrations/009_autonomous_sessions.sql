-- Migration 009 -> schema_version=9:
-- Persist autonomous runner custody, audit events, and posted review comments.

CREATE TABLE IF NOT EXISTS run_sessions (
    id              TEXT PRIMARY KEY,
    run_id          INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    provider        TEXT    NOT NULL,
    mode            TEXT    NOT NULL,
    token_sha256    TEXT    NOT NULL,
    status          TEXT    NOT NULL,
    started_at      INTEGER NOT NULL,
    finished_at     INTEGER,
    failure_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_run_sessions_run ON run_sessions(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_run_sessions_status ON run_sessions(status);

CREATE TABLE IF NOT EXISTS run_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    session_id TEXT,
    event      TEXT    NOT NULL,
    detail     TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_run_events_run ON run_events(run_id, created_at);

CREATE TABLE IF NOT EXISTS pr_review_posts (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id             INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    review_evidence_id INTEGER REFERENCES review_evidence(id) ON DELETE SET NULL,
    pr_number          TEXT    NOT NULL,
    comment_id         TEXT    NOT NULL,
    comment_url        TEXT    NOT NULL,
    body_sha256        TEXT    NOT NULL,
    posted_by          TEXT    NOT NULL,
    created_at         INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pr_review_posts_run ON pr_review_posts(run_id, created_at);

UPDATE meta SET value='9' WHERE key='schema_version';
