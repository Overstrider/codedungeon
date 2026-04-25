-- codedungeon schema v6
-- SQLite with FTS5. Pure-Go driver (modernc.org/sqlite).
-- All times are unix seconds (INTEGER).

PRAGMA journal_mode = WAL;
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
INSERT OR IGNORE INTO meta (key, value) VALUES ('schema_version', '6');
INSERT OR IGNORE INTO meta (key, value) VALUES ('model_reasoning_effort', '');
INSERT OR IGNORE INTO meta (key, value) VALUES ('model_fast_effort', '');
