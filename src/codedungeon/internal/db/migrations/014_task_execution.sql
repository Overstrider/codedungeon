CREATE TABLE IF NOT EXISTS execution_sessions (
    id              TEXT PRIMARY KEY,
    run_id          INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    task_id         TEXT    NOT NULL,
    task_path       TEXT    NOT NULL,
    provider        TEXT    NOT NULL,
    status          TEXT    NOT NULL,
    output_dir      TEXT    NOT NULL,
    attempt         INTEGER NOT NULL DEFAULT 0,
    started_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    finished_at     INTEGER,
    expires_at      INTEGER,
    failure_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_execution_sessions_run ON execution_sessions(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_execution_sessions_status ON execution_sessions(status, started_at);

CREATE TABLE IF NOT EXISTS execution_transitions (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id  TEXT    NOT NULL REFERENCES execution_sessions(id) ON DELETE CASCADE,
    from_status TEXT,
    to_status   TEXT    NOT NULL,
    reason      TEXT,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_execution_transitions_session ON execution_transitions(session_id, id);

CREATE TABLE IF NOT EXISTS execution_attempts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id          TEXT    NOT NULL REFERENCES execution_sessions(id) ON DELETE CASCADE,
    attempt             INTEGER NOT NULL,
    provider_session_id TEXT,
    head_before         TEXT,
    head_after          TEXT,
    backup_ref          TEXT,
    diff_path           TEXT,
    changed_files       TEXT,
    worker_status       TEXT,
    review_status       TEXT,
    verification_status TEXT,
    summary             TEXT,
    result_json         TEXT,
    error_message       TEXT,
    started_at          INTEGER NOT NULL,
    finished_at         INTEGER
);

CREATE INDEX IF NOT EXISTS idx_execution_attempts_session ON execution_attempts(session_id, attempt);

CREATE TABLE IF NOT EXISTS learned_rules (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    session_id TEXT,
    taxonomy   TEXT    NOT NULL,
    title      TEXT    NOT NULL,
    body       TEXT    NOT NULL,
    source     TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_learned_rules_session ON learned_rules(session_id, created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_learned_rules USING fts5(
    taxonomy, title, body,
    content='learned_rules', content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS learned_rules_ai AFTER INSERT ON learned_rules BEGIN
    INSERT INTO fts_learned_rules(rowid, taxonomy, title, body)
    VALUES (new.id, new.taxonomy, new.title, new.body);
END;

CREATE TRIGGER IF NOT EXISTS learned_rules_ad AFTER DELETE ON learned_rules BEGIN
    INSERT INTO fts_learned_rules(fts_learned_rules, rowid, taxonomy, title, body)
    VALUES ('delete', old.id, old.taxonomy, old.title, old.body);
END;

INSERT OR REPLACE INTO meta(key, value) VALUES('schema_version', '14');
