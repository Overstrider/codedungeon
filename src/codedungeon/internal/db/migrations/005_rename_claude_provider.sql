-- Migration 005 -> schema_version=5:
-- Canonicalize Claude provider metadata from legacy aliases to "claude".

UPDATE installed_artifacts
SET provider = 'claude'
WHERE provider IN ('claude-code', 'claude-ce');

UPDATE installed_artifacts
SET pack_id = 'codedungeon-claude'
WHERE pack_id IN ('codedungeon-claude-code', 'codedungeon-claude-ce');

UPDATE meta SET value='5' WHERE key='schema_version';
