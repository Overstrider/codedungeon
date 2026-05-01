-- codedungeon schema v15
-- SQLite with FTS5. Pure-Go driver (modernc.org/sqlite).
-- All times are unix seconds (INTEGER).

PRAGMA journal_mode = DELETE;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    feature      TEXT    NOT NULL,
    branch       TEXT,
    project_mode TEXT,
    mode         TEXT,
    repo_map     TEXT,                  -- JSON: {repo_name: {path, lang, specialist, domain_planner, stack}}
    env          TEXT,                  -- JSON: {playwright_skill, test_auth_missing[]}
    created_at   INTEGER NOT NULL,
    updated_at   INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS phases (
    run_id       INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    phase        TEXT    NOT NULL,
    status       TEXT    NOT NULL,      -- PENDING | IN_PROGRESS | DONE | SKIPPED | FAIL
    notes        TEXT,
    artifacts    TEXT,                  -- JSON array
    started_at   INTEGER,
    finished_at  INTEGER,
    PRIMARY KEY (run_id, phase)
);

CREATE TABLE IF NOT EXISTS handoffs (
    run_id         INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    phase          TEXT    NOT NULL,
    summary        TEXT,
    decisions      TEXT,                -- JSON array
    artifacts      TEXT,                -- JSON array
    traps          TEXT,                -- JSON array
    open_questions TEXT,                -- JSON array
    next_input     TEXT,
    promise        TEXT,
    rendered_md    TEXT,                -- full .md content cache
    created_at     INTEGER NOT NULL,
    PRIMARY KEY (run_id, phase)
);

CREATE TABLE IF NOT EXISTS prompts (
    name       TEXT    NOT NULL,
    version    INTEGER NOT NULL,
    content    TEXT    NOT NULL,
    sha256     TEXT    NOT NULL,
    source     TEXT    NOT NULL,        -- 'embedded' | 'user'
    created_at INTEGER NOT NULL,
    PRIMARY KEY (name, version)
);

CREATE INDEX IF NOT EXISTS idx_prompts_name ON prompts(name);

CREATE TABLE IF NOT EXISTS tasks (
    run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    repo       TEXT    NOT NULL,
    task_id    TEXT    NOT NULL,
    kind       TEXT,                    -- dev | test | fix
    status     TEXT,                    -- pending | in_progress | done | blocked
    title      TEXT,
    depends_on TEXT,                    -- JSON array
    content    TEXT,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (run_id, repo, task_id)
);

CREATE TABLE IF NOT EXISTS findings (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id          INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    cycle           INTEGER NOT NULL,
    severity        TEXT,
    file            TEXT,
    line_start      INTEGER,
    line_end        INTEGER,
    title           TEXT,
    evidence_quote  TEXT,
    flagged_by      TEXT,               -- JSON array
    actionable      INTEGER,            -- 0/1
    design_decision INTEGER,            -- 0/1
    rationale       TEXT,
    full_json       TEXT,
    created_at      INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_findings_run ON findings(run_id, cycle);

CREATE TABLE IF NOT EXISTS review_evidence (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id            INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    review_dir        TEXT    NOT NULL,
    review_json_path  TEXT    NOT NULL,
    manifest_path     TEXT    NOT NULL,
    verdict           TEXT    NOT NULL,
    pr_number         TEXT    NOT NULL,
    base_sha          TEXT    NOT NULL,
    head_sha          TEXT    NOT NULL,
    personas_expected TEXT    NOT NULL,
    personas_run      TEXT    NOT NULL,
    created_at        INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_review_evidence_run ON review_evidence(run_id, created_at);

CREATE TABLE IF NOT EXISTS verification_records (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    phase      TEXT    NOT NULL,
    command    TEXT    NOT NULL,
    status     TEXT    NOT NULL,
    log_path   TEXT    NOT NULL,
    created_at INTEGER NOT NULL,
    superseded_at INTEGER
);

CREATE INDEX IF NOT EXISTS idx_verification_records_run_phase ON verification_records(run_id, phase, created_at);

CREATE TABLE IF NOT EXISTS report_evidence (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id      INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    report_path TEXT    NOT NULL,
    sha256      TEXT    NOT NULL,
    created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_report_evidence_run ON report_evidence(run_id, created_at);

CREATE TABLE IF NOT EXISTS run_sessions (
    id              TEXT PRIMARY KEY,
    run_id          INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    provider        TEXT    NOT NULL,
    mode            TEXT    NOT NULL,
    token_sha256    TEXT    NOT NULL,
    status          TEXT    NOT NULL, -- RUNNING | READY_FOR_USER_REVIEW | COMPLETED | FAILED | ABORTED
    started_at      INTEGER NOT NULL,
    finished_at     INTEGER,
    failure_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_run_sessions_run ON run_sessions(run_id, started_at);
CREATE INDEX IF NOT EXISTS idx_run_sessions_status ON run_sessions(status);

CREATE TABLE IF NOT EXISTS run_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    session_id TEXT,
    event      TEXT    NOT NULL,
    detail     TEXT,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_run_events_run ON run_events(run_id, created_at);

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
    status           TEXT    NOT NULL, -- RUNNING | COMPLETED | FAILED | ABORTED
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

CREATE TABLE IF NOT EXISTS pr_review_posts (
    id                 INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id             INTEGER NOT NULL REFERENCES runs(id) ON DELETE CASCADE,
    review_evidence_id INTEGER REFERENCES review_evidence(id) ON DELETE SET NULL,
    pr_number          TEXT    NOT NULL,
    comment_id         TEXT    NOT NULL,
    comment_url        TEXT    NOT NULL,
    body_sha256        TEXT    NOT NULL,
    posted_by          TEXT    NOT NULL,
    created_at         INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_pr_review_posts_run ON pr_review_posts(run_id, created_at);

CREATE TABLE IF NOT EXISTS project_context_versions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    version       INTEGER NOT NULL UNIQUE,
    body          TEXT    NOT NULL,
    sha256        TEXT    NOT NULL,
    source_digest TEXT    NOT NULL,
    approved_by   TEXT    NOT NULL,
    created_at    INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS project_context_proposals (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id        INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    base_version  INTEGER NOT NULL DEFAULT 0,
    proposed_body TEXT    NOT NULL,
    diff_summary  TEXT    NOT NULL,
    status        TEXT    NOT NULL,
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    approved_at   INTEGER,
    approved_by   TEXT
);

CREATE INDEX IF NOT EXISTS idx_project_context_proposals_status ON project_context_proposals(status, created_at);
CREATE INDEX IF NOT EXISTS idx_project_context_proposals_run ON project_context_proposals(run_id, created_at);

CREATE TABLE IF NOT EXISTS project_context_audit (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    run_id     INTEGER REFERENCES runs(id) ON DELETE SET NULL,
    kind       TEXT    NOT NULL,
    ref        TEXT,
    summary    TEXT    NOT NULL,
    full_json  TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_project_context_audit_kind ON project_context_audit(kind, created_at);
CREATE INDEX IF NOT EXISTS idx_project_context_audit_run ON project_context_audit(run_id, created_at);

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

-- FTS5 virtual tables (external content — mirror rowid of source table)
CREATE VIRTUAL TABLE IF NOT EXISTS fts_handoffs USING fts5(
    summary, decisions, traps, rendered_md,
    content='handoffs', content_rowid='rowid'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_prompts USING fts5(
    name, content
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_findings USING fts5(
    title, evidence_quote, rationale,
    content='findings', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_tasks USING fts5(
    title, content,
    content='tasks', content_rowid='rowid'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_project_context_versions USING fts5(
    body,
    content='project_context_versions', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_project_context_proposals USING fts5(
    proposed_body, diff_summary,
    content='project_context_proposals', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_project_context_audit USING fts5(
    summary, full_json,
    content='project_context_audit', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_planning_blackboard USING fts5(
    role, kind, title, summary, full_json,
    content='planning_blackboard', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_planning_task_graphs USING fts5(
    graph_json,
    content='planning_task_graphs', content_rowid='id'
);

CREATE VIRTUAL TABLE IF NOT EXISTS fts_learned_rules USING fts5(
    taxonomy, title, body,
    content='learned_rules', content_rowid='id'
);

-- Sync triggers for external-content FTS5 tables
CREATE TRIGGER IF NOT EXISTS handoffs_ai AFTER INSERT ON handoffs BEGIN
    INSERT INTO fts_handoffs(rowid, summary, decisions, traps, rendered_md)
    VALUES (new.rowid, new.summary, new.decisions, new.traps, new.rendered_md);
END;

CREATE TRIGGER IF NOT EXISTS handoffs_ad AFTER DELETE ON handoffs BEGIN
    INSERT INTO fts_handoffs(fts_handoffs, rowid, summary, decisions, traps, rendered_md)
    VALUES ('delete', old.rowid, old.summary, old.decisions, old.traps, old.rendered_md);
END;

CREATE TRIGGER IF NOT EXISTS handoffs_au AFTER UPDATE ON handoffs BEGIN
    INSERT INTO fts_handoffs(fts_handoffs, rowid, summary, decisions, traps, rendered_md)
    VALUES ('delete', old.rowid, old.summary, old.decisions, old.traps, old.rendered_md);
    INSERT INTO fts_handoffs(rowid, summary, decisions, traps, rendered_md)
    VALUES (new.rowid, new.summary, new.decisions, new.traps, new.rendered_md);
END;

CREATE TRIGGER IF NOT EXISTS findings_ai AFTER INSERT ON findings BEGIN
    INSERT INTO fts_findings(rowid, title, evidence_quote, rationale)
    VALUES (new.id, new.title, new.evidence_quote, new.rationale);
END;

CREATE TRIGGER IF NOT EXISTS findings_ad AFTER DELETE ON findings BEGIN
    INSERT INTO fts_findings(fts_findings, rowid, title, evidence_quote, rationale)
    VALUES ('delete', old.id, old.title, old.evidence_quote, old.rationale);
END;

CREATE TRIGGER IF NOT EXISTS tasks_ai AFTER INSERT ON tasks BEGIN
    INSERT INTO fts_tasks(rowid, title, content) VALUES (new.rowid, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS tasks_ad AFTER DELETE ON tasks BEGIN
    INSERT INTO fts_tasks(fts_tasks, rowid, title, content) VALUES ('delete', old.rowid, old.title, old.content);
END;

CREATE TRIGGER IF NOT EXISTS tasks_au AFTER UPDATE ON tasks BEGIN
    INSERT INTO fts_tasks(fts_tasks, rowid, title, content) VALUES ('delete', old.rowid, old.title, old.content);
    INSERT INTO fts_tasks(rowid, title, content) VALUES (new.rowid, new.title, new.content);
END;

CREATE TRIGGER IF NOT EXISTS prompts_ai AFTER INSERT ON prompts BEGIN
    INSERT INTO fts_prompts(rowid, name, content) VALUES (new.rowid, new.name, new.content);
END;

CREATE TRIGGER IF NOT EXISTS prompts_ad AFTER DELETE ON prompts BEGIN
    INSERT INTO fts_prompts(fts_prompts, rowid, name, content) VALUES ('delete', old.rowid, old.name, old.content);
END;

CREATE TRIGGER IF NOT EXISTS project_context_versions_ai AFTER INSERT ON project_context_versions BEGIN
    INSERT INTO fts_project_context_versions(rowid, body) VALUES (new.id, new.body);
END;

CREATE TRIGGER IF NOT EXISTS project_context_versions_ad AFTER DELETE ON project_context_versions BEGIN
    INSERT INTO fts_project_context_versions(fts_project_context_versions, rowid, body) VALUES ('delete', old.id, old.body);
END;

CREATE TRIGGER IF NOT EXISTS project_context_versions_au AFTER UPDATE ON project_context_versions BEGIN
    INSERT INTO fts_project_context_versions(fts_project_context_versions, rowid, body) VALUES ('delete', old.id, old.body);
    INSERT INTO fts_project_context_versions(rowid, body) VALUES (new.id, new.body);
END;

CREATE TRIGGER IF NOT EXISTS project_context_proposals_ai AFTER INSERT ON project_context_proposals BEGIN
    INSERT INTO fts_project_context_proposals(rowid, proposed_body, diff_summary) VALUES (new.id, new.proposed_body, new.diff_summary);
END;

CREATE TRIGGER IF NOT EXISTS project_context_proposals_ad AFTER DELETE ON project_context_proposals BEGIN
    INSERT INTO fts_project_context_proposals(fts_project_context_proposals, rowid, proposed_body, diff_summary) VALUES ('delete', old.id, old.proposed_body, old.diff_summary);
END;

CREATE TRIGGER IF NOT EXISTS project_context_proposals_au AFTER UPDATE ON project_context_proposals BEGIN
    INSERT INTO fts_project_context_proposals(fts_project_context_proposals, rowid, proposed_body, diff_summary) VALUES ('delete', old.id, old.proposed_body, old.diff_summary);
    INSERT INTO fts_project_context_proposals(rowid, proposed_body, diff_summary) VALUES (new.id, new.proposed_body, new.diff_summary);
END;

CREATE TRIGGER IF NOT EXISTS project_context_audit_ai AFTER INSERT ON project_context_audit BEGIN
    INSERT INTO fts_project_context_audit(rowid, summary, full_json) VALUES (new.id, new.summary, new.full_json);
END;

CREATE TRIGGER IF NOT EXISTS project_context_audit_ad AFTER DELETE ON project_context_audit BEGIN
    INSERT INTO fts_project_context_audit(fts_project_context_audit, rowid, summary, full_json) VALUES ('delete', old.id, old.summary, old.full_json);
END;

CREATE TRIGGER IF NOT EXISTS project_context_audit_au AFTER UPDATE ON project_context_audit BEGIN
    INSERT INTO fts_project_context_audit(fts_project_context_audit, rowid, summary, full_json) VALUES ('delete', old.id, old.summary, old.full_json);
    INSERT INTO fts_project_context_audit(rowid, summary, full_json) VALUES (new.id, new.summary, new.full_json);
END;

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

CREATE TRIGGER IF NOT EXISTS learned_rules_ai AFTER INSERT ON learned_rules BEGIN
    INSERT INTO fts_learned_rules(rowid, taxonomy, title, body)
    VALUES (new.id, new.taxonomy, new.title, new.body);
END;

CREATE TRIGGER IF NOT EXISTS learned_rules_ad AFTER DELETE ON learned_rules BEGIN
    INSERT INTO fts_learned_rules(fts_learned_rules, rowid, taxonomy, title, body)
    VALUES ('delete', old.id, old.taxonomy, old.title, old.body);
END;

-- installed_artifacts: tracks embedded agents/skills/commands written to project
-- (schema_version=3 adds this; migration 003 creates it on existing DBs).
CREATE TABLE IF NOT EXISTS installed_artifacts (
    rel_path       TEXT PRIMARY KEY,
    install_path   TEXT NOT NULL DEFAULT '',
    sha256         TEXT NOT NULL,
    binary_version TEXT NOT NULL,
    provider       TEXT NOT NULL DEFAULT 'claude',
    pack_id        TEXT NOT NULL DEFAULT 'codedungeon-claude',
    pack_version   TEXT NOT NULL DEFAULT '1',
    kind           TEXT NOT NULL DEFAULT '',
    logical_name   TEXT NOT NULL DEFAULT '',
    user_modified  INTEGER NOT NULL DEFAULT 0,
    installed_at   INTEGER NOT NULL
);

-- bootstrap: schema_version
INSERT OR IGNORE INTO meta (key, value) VALUES ('schema_version', '15');
INSERT OR IGNORE INTO meta (key, value) VALUES ('model_reasoning_effort', '');
INSERT OR IGNORE INTO meta (key, value) VALUES ('model_fast_effort', '');
