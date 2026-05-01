CREATE TABLE IF NOT EXISTS qa_sessions (
    id              TEXT PRIMARY KEY,
    run_id          INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    execution_id    TEXT,
    entrypoint      TEXT    NOT NULL,
    mode            TEXT    NOT NULL,
    status          TEXT    NOT NULL,
    root            TEXT    NOT NULL,
    plan_path       TEXT,
    evidence_dir    TEXT    NOT NULL,
    started_at      INTEGER NOT NULL,
    updated_at      INTEGER NOT NULL,
    finished_at     INTEGER,
    failure_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_qa_sessions_run ON qa_sessions(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_qa_sessions_status ON qa_sessions(status, started_at);

CREATE TABLE IF NOT EXISTS qa_checks (
    id             TEXT PRIMARY KEY,
    session_id     TEXT    NOT NULL REFERENCES qa_sessions(id) ON DELETE CASCADE,
    kind           TEXT    NOT NULL,
    name           TEXT    NOT NULL,
    status         TEXT    NOT NULL,
    command        TEXT,
    cwd            TEXT,
    exit_code      INTEGER,
    duration_ms    INTEGER,
    log_path       TEXT,
    report_path    TEXT,
    artifacts_json TEXT,
    started_at     INTEGER NOT NULL,
    finished_at    INTEGER
);

CREATE INDEX IF NOT EXISTS idx_qa_checks_session ON qa_checks(session_id, started_at);

CREATE TABLE IF NOT EXISTS qa_dependencies (
    id           TEXT PRIMARY KEY,
    session_id   TEXT    NOT NULL REFERENCES qa_sessions(id) ON DELETE CASCADE,
    name         TEXT    NOT NULL,
    required     INTEGER NOT NULL,
    status       TEXT    NOT NULL,
    version      TEXT,
    install_hint TEXT,
    detail       TEXT
);

CREATE INDEX IF NOT EXISTS idx_qa_dependencies_session ON qa_dependencies(session_id, name);

CREATE TABLE IF NOT EXISTS qa_findings (
    id            TEXT PRIMARY KEY,
    session_id    TEXT    NOT NULL REFERENCES qa_sessions(id) ON DELETE CASCADE,
    severity      TEXT    NOT NULL,
    category      TEXT    NOT NULL,
    title         TEXT    NOT NULL,
    detail        TEXT,
    evidence_path TEXT,
    fix_task_path TEXT,
    created_at    INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_qa_findings_session ON qa_findings(session_id, created_at);

CREATE TABLE IF NOT EXISTS qa_artifacts (
    id         TEXT PRIMARY KEY,
    session_id TEXT    NOT NULL REFERENCES qa_sessions(id) ON DELETE CASCADE,
    check_id   TEXT,
    kind       TEXT    NOT NULL,
    path       TEXT    NOT NULL,
    sha256     TEXT,
    bytes      INTEGER,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_qa_artifacts_session ON qa_artifacts(session_id, created_at);

INSERT OR REPLACE INTO meta(key, value) VALUES('schema_version', '15');
