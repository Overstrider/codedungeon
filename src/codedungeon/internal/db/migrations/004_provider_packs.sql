-- Migration 004 -> schema_version=4:
-- Adds provider-native pack metadata and project-relative install paths.

UPDATE installed_artifacts
SET install_path = CASE
    WHEN install_path = '' THEN '.claude/' || rel_path
    ELSE install_path
END;

UPDATE meta SET value='4' WHERE key='schema_version';
