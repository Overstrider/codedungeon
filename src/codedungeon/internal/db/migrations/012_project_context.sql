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
