-- Migration 007 -> schema_version=7:
-- Keep project-local SQLite friendlier to versioned .codedungeon directories.

PRAGMA journal_mode = DELETE;

UPDATE meta SET value='7' WHERE key='schema_version';
