-- Migration 008 -> schema_version=8:
-- Persist deterministic review, verification, and report evidence for gates.

CREATE TABLE IF NOT EXISTS review_evidence (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id            INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    review_dir        TEXT    NOT NULL,
    review_json_path  TEXT    NOT NULL,
    manifest_path     TEXT    NOT NULL,
    verdict           TEXT    NOT NULL,
    pr_number         TEXT    NOT NULL,
    base_sha          TEXT    NOT NULL,
    head_sha          TEXT    NOT NULL,
    personas_expected TEXT    NOT NULL,
    personas_run      TEXT    NOT NULL,
    created_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_review_evidence_run ON review_evidence(run_id, created_at);

CREATE TABLE IF NOT EXISTS verification_records (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    phase      TEXT    NOT NULL,
    command    TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    log_path   TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_verification_records_run_phase ON verification_records(run_id, phase, created_at);

CREATE TABLE IF NOT EXISTS report_evidence (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    report_path TEXT    NOT NULL,
    sha256      TEXT    NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_report_evidence_run ON report_evidence(run_id, created_at);

UPDATE meta SET value='8' WHERE key='schema_version';
