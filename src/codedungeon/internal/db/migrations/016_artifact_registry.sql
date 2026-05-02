CREATE TABLE IF NOT EXISTS artifacts (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    module        TEXT    NOT NULL,
    owner_type    TEXT    NOT NULL,
    owner_id      TEXT    NOT NULL,
    phase         TEXT,
    role          TEXT    NOT NULL,
    kind          TEXT    NOT NULL,
    path          TEXT    NOT NULL,
    abs_path      TEXT,
    artifact_type TEXT    NOT NULL,
    media_type    TEXT,
    sha256        TEXT,
    bytes         INTEGER,
    metadata_json TEXT    NOT NULL DEFAULT '{}',
    created_at    INTEGER NOT NULL,
    UNIQUE(module, owner_type, owner_id, role, path)
);

CREATE INDEX IF NOT EXISTS idx_artifacts_run_module ON artifacts(run_id, module, created_at);
CREATE INDEX IF NOT EXISTS idx_artifacts_owner ON artifacts(module, owner_type, owner_id);
CREATE INDEX IF NOT EXISTS idx_artifacts_path ON artifacts(path);
CREATE INDEX IF NOT EXISTS idx_artifacts_sha ON artifacts(sha256);

INSERT OR REPLACE INTO meta(key, value) VALUES('schema_version', '16');
