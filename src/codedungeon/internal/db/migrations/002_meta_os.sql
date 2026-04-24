-- Migration 002 → schema_version=2:
-- No column adds; just reserve meta keys for os/project_root/cd_version/bootstrapped_at.
-- Values are written by `bootstrap` at runtime.
INSERT OR IGNORE INTO meta(key, value) VALUES
    ('os', ''),
    ('project_root', ''),
    ('cd_version', ''),
    ('bootstrapped_at', '');
UPDATE meta SET value='2' WHERE key='schema_version';
