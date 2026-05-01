CREATE TABLE IF NOT EXISTS planning_sessions (
    id                      TEXT PRIMARY KEY,
    run_id                  INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    mode                    TEXT    NOT NULL,
    prompt                  TEXT    NOT NULL,
    prompt_sha256           TEXT    NOT NULL,
    project_context_sha256  TEXT    NOT NULL,
    rules_status            TEXT,
    rules_digest            TEXT,
    rules_read              TEXT,
    human_gate_policy       TEXT    NOT NULL,
    status                  TEXT    NOT NULL,
    output_dir              TEXT    NOT NULL,
    created_at              INTEGER NOT NULL,
    updated_at              INTEGER NOT NULL,
    finished_at             INTEGER,
    failure_message         TEXT
);

CREATE INDEX IF NOT EXISTS idx_planning_sessions_run ON planning_sessions(run_id, created_at);
CREATE INDEX IF NOT EXISTS idx_planning_sessions_status ON planning_sessions(status, created_at);

CREATE TABLE IF NOT EXISTS planning_agents (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT    NOT NULL REFERENCES planning_sessions(id) ON DELETE CASCADE,
    run_id       INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    role         TEXT    NOT NULL,
    round        INTEGER NOT NULL,
    provider     TEXT,
    model        TEXT,
    agent_name   TEXT,
    status       TEXT    NOT NULL,
    confidence   REAL,
    output_path  TEXT,
    summary      TEXT,
    error        TEXT,
    started_at   INTEGER NOT NULL,
    finished_at  INTEGER
);

CREATE INDEX IF NOT EXISTS idx_planning_agents_session ON planning_agents(session_id, id);
CREATE INDEX IF NOT EXISTS idx_planning_agents_run ON planning_agents(run_id, started_at);

CREATE TABLE IF NOT EXISTS planning_blackboard (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT    NOT NULL REFERENCES planning_sessions(id) ON DELETE CASCADE,
    run_id       INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    role         TEXT    NOT NULL,
    kind         TEXT    NOT NULL,
    title        TEXT,
    summary      TEXT,
    full_json    TEXT    NOT NULL,
    created_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_planning_blackboard_session ON planning_blackboard(session_id, id);
CREATE INDEX IF NOT EXISTS idx_planning_blackboard_run ON planning_blackboard(run_id, created_at);

CREATE TABLE IF NOT EXISTS planning_evaluations (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id        TEXT    NOT NULL REFERENCES planning_sessions(id) ON DELETE CASCADE,
    run_id            INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    verdict           TEXT    NOT NULL,
    score             REAL,
    needs_user_input  INTEGER NOT NULL,
    questions         TEXT,
    issues            TEXT,
    full_json         TEXT    NOT NULL,
    created_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_planning_evaluations_session ON planning_evaluations(session_id, id);
CREATE INDEX IF NOT EXISTS idx_planning_evaluations_run ON planning_evaluations(run_id, created_at);

CREATE TABLE IF NOT EXISTS planning_task_graphs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id   TEXT    NOT NULL REFERENCES planning_sessions(id) ON DELETE CASCADE,
    run_id       INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    version      INTEGER NOT NULL,
    status       TEXT    NOT NULL,
    graph_json   TEXT    NOT NULL,
    created_at   INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_planning_task_graphs_session ON planning_task_graphs(session_id, version);
CREATE INDEX IF NOT EXISTS idx_planning_task_graphs_run ON planning_task_graphs(run_id, created_at);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_planning_blackboard USING fts5(
    role, kind, title, summary, full_json,
    content='planning_blackboard', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_planning_task_graphs USING fts5(
    graph_json,
    content='planning_task_graphs', content_rowid='id'
);

CREATE TRIGGER IF NOT EXISTS planning_blackboard_ai AFTER INSERT ON planning_blackboard BEGIN
    INSERT INTO fts_planning_blackboard(rowid, role, kind, title, summary, full_json)
    VALUES (new.id, new.role, new.kind, new.title, new.summary, new.full_json);
END;

CREATE TRIGGER IF NOT EXISTS planning_blackboard_ad AFTER DELETE ON planning_blackboard BEGIN
    INSERT INTO fts_planning_blackboard(fts_planning_blackboard, rowid, role, kind, title, summary, full_json)
    VALUES ('delete', old.id, old.role, old.kind, old.title, old.summary, old.full_json);
END;

CREATE TRIGGER IF NOT EXISTS planning_blackboard_au AFTER UPDATE ON planning_blackboard BEGIN
    INSERT INTO fts_planning_blackboard(fts_planning_blackboard, rowid, role, kind, title, summary, full_json)
    VALUES ('delete', old.id, old.role, old.kind, old.title, old.summary, old.full_json);
    INSERT INTO fts_planning_blackboard(rowid, role, kind, title, summary, full_json)
    VALUES (new.id, new.role, new.kind, new.title, new.summary, new.full_json);
END;

CREATE TRIGGER IF NOT EXISTS planning_task_graphs_ai AFTER INSERT ON planning_task_graphs BEGIN
    INSERT INTO fts_planning_task_graphs(rowid, graph_json) VALUES (new.id, new.graph_json);
END;

CREATE TRIGGER IF NOT EXISTS planning_task_graphs_ad AFTER DELETE ON planning_task_graphs BEGIN
    INSERT INTO fts_planning_task_graphs(fts_planning_task_graphs, rowid, graph_json) VALUES ('delete', old.id, old.graph_json);
END;

CREATE TRIGGER IF NOT EXISTS planning_task_graphs_au AFTER UPDATE ON planning_task_graphs BEGIN
    INSERT INTO fts_planning_task_graphs(fts_planning_task_graphs, rowid, graph_json) VALUES ('delete', old.id, old.graph_json);
    INSERT INTO fts_planning_task_graphs(rowid, graph_json) VALUES (new.id, new.graph_json);
END;
