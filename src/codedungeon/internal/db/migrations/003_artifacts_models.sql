-- Migration 003 → schema_version=3:
-- Adds `installed_artifacts` (for embedded agent/skill tracking — Sprint 7 Stage 4).
-- Adds `model_reasoning` + `model_fast` meta keys (Sprint 7 Stage 3).

CREATE TABLE IF NOT EXISTS installed_artifacts (
    rel_path       TEXT PRIMARY KEY,
    sha256         TEXT NOT NULL,
    binary_version TEXT NOT NULL,
    user_modified  INTEGER NOT NULL DEFAULT 0,
    installed_at   INTEGER NOT NULL
);

INSERT OR IGNORE INTO meta(key, value) VALUES
    ('model_reasoning', ''),
    ('model_fast', '');

UPDATE meta SET value='3' WHERE key='schema_version';
