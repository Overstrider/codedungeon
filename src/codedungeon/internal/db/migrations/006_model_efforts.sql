-- Migration 006 -> schema_version=6:
-- Adds model effort meta keys for Codex model-tier execution.

INSERT OR IGNORE INTO meta(key, value) VALUES
    ('model_reasoning_effort', ''),
    ('model_fast_effort', '');

UPDATE meta SET value='6' WHERE key='schema_version';
