CREATE TABLE IF NOT EXISTS agent_runs (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id           INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    session_id       TEXT,
    phase            TEXT,
    role             TEXT    NOT NULL,
    agent_type       TEXT,
    agent_name       TEXT,
    model            TEXT,
    reasoning_effort TEXT,
    task_path        TEXT,
    input_summary    TEXT,
    status           TEXT    NOT NULL,
    output_summary   TEXT,
    artifact_path    TEXT,
    error_message    TEXT,
    started_at       INTEGER NOT NULL,
    finished_at      INTEGER
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_run ON agent_runs(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_agent_runs_status ON agent_runs(run_id, status);

CREATE TABLE IF NOT EXISTS agent_events (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id       INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    agent_run_id INTEGER REFERENCES agent_runs(id) ON DELETE SET NULL,
    session_id   TEXT,
    phase        TEXT,
    event        TEXT    NOT NULL,
    detail       TEXT,
    created_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_agent_events_run ON agent_events(run_id, created_at);

UPDATE meta SET value='10' WHERE key='schema_version';
