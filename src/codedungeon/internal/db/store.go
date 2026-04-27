package db

import (
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

//go:embed migrations/*.sql
var migrationsFS embed.FS

const SchemaVersion = "9"

// Store wraps the sqlite connection and exposes typed helpers.
type Store struct {
	DB   *sql.DB
	Path string
}

// DefaultPath resolves the project-local db path: <cwd>/<configDir>/codedungeon.db.
// configDir defaults to ".codedungeon" when empty. Caller should pass provider.Detect().DBPath()
// or provider runtime dir for provider-agnostic state.
func DefaultPath(configDir ...string) string {
	dir := ".codedungeon"
	if len(configDir) > 0 && configDir[0] != "" {
		dir = configDir[0]
	}
	cwd, _ := os.Getwd()
	return filepath.Join(cwd, dir, "codedungeon.db")
}

// Meta helpers

// SetMeta upserts a key/value pair in the meta table.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.DB.Exec(`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetMeta returns the value for a key, or "" when missing.
func (s *Store) GetMeta(key string) (string, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM meta WHERE key=?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// Open opens (creating if needed) the db file and applies the schema.
// Caller must Close().
func Open(path string) (*Store, error) {
	if path == "" {
		path = DefaultPath()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}
	d, err := sql.Open("sqlite", path+"?_pragma=journal_mode(DELETE)&_pragma=foreign_keys(1)&_time_format=sqlite")
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	if err := d.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{DB: d, Path: path}
	return s, nil
}

// Init applies the embedded schema (idempotent) + runs pending migrations.
// Fresh DB: schema.sql creates everything at current version; Migrate() is a no-op.
// Existing DB: schema.sql is idempotent via IF NOT EXISTS; Migrate() applies
// deltas (e.g. new meta keys, new tables) from `migrations/*.sql`.
func (s *Store) Init() error {
	if _, err := s.DB.Exec(schemaSQL); err != nil {
		return fmt.Errorf("apply schema: %w", err)
	}
	return s.Migrate()
}

// Migrate applies migrations whose number > the current schema_version.
// Files are ordered lexicographically under migrations/ (001_, 002_, ...).
// Safe to run repeatedly.
func (s *Store) Migrate() error {
	cur, err := s.SchemaVersion()
	if err != nil {
		return fmt.Errorf("read schema_version: %w", err)
	}
	curN := 0
	if cur != "" {
		fmt.Sscanf(cur, "%d", &curN)
	}
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		// Filename: NNN_name.sql → migration number = NNN.
		num := 0
		fmt.Sscanf(e.Name(), "%d_", &num)
		if num == 0 || num <= curN {
			continue
		}
		if num == 4 {
			if err := s.ensureArtifactPackColumns(); err != nil {
				return fmt.Errorf("prepare %s: %w", e.Name(), err)
			}
		}
		body, err := migrationsFS.ReadFile("migrations/" + e.Name())
		if err != nil {
			return fmt.Errorf("read %s: %w", e.Name(), err)
		}
		if _, err := s.DB.Exec(string(body)); err != nil {
			return fmt.Errorf("apply %s: %w", e.Name(), err)
		}
	}
	// Ensure schema_version reflects the intended target.
	_, err = s.DB.Exec(`UPDATE meta SET value=? WHERE key='schema_version'`, SchemaVersion)
	return err
}

func (s *Store) ensureArtifactPackColumns() error {
	cols, err := tableColumns(s.DB, "installed_artifacts")
	if err != nil {
		return err
	}
	add := []struct {
		name string
		sql  string
	}{
		{"install_path", `ALTER TABLE installed_artifacts ADD COLUMN install_path TEXT NOT NULL DEFAULT ''`},
		{"provider", `ALTER TABLE installed_artifacts ADD COLUMN provider TEXT NOT NULL DEFAULT 'claude'`},
		{"pack_id", `ALTER TABLE installed_artifacts ADD COLUMN pack_id TEXT NOT NULL DEFAULT 'codedungeon-claude'`},
		{"pack_version", `ALTER TABLE installed_artifacts ADD COLUMN pack_version TEXT NOT NULL DEFAULT '1'`},
		{"kind", `ALTER TABLE installed_artifacts ADD COLUMN kind TEXT NOT NULL DEFAULT ''`},
		{"logical_name", `ALTER TABLE installed_artifacts ADD COLUMN logical_name TEXT NOT NULL DEFAULT ''`},
	}
	for _, col := range add {
		if cols[col.name] {
			continue
		}
		if _, err := s.DB.Exec(col.sql); err != nil {
			return err
		}
	}
	return nil
}

func tableColumns(db *sql.DB, table string) (map[string]bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		cols[name] = true
	}
	return cols, rows.Err()
}

// Close closes the underlying connection.
func (s *Store) Close() error {
	if s == nil || s.DB == nil {
		return nil
	}
	return s.DB.Close()
}

// SchemaVersion returns the stored schema version (or empty string).
func (s *Store) SchemaVersion() (string, error) {
	var v string
	err := s.DB.QueryRow(`SELECT value FROM meta WHERE key='schema_version'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// ===== Runs =====

type Run struct {
	ID          int64           `json:"id"`
	Feature     string          `json:"feature"`
	Branch      string          `json:"branch,omitempty"`
	ProjectMode string          `json:"project_mode,omitempty"`
	Mode        string          `json:"mode,omitempty"`
	RepoMap     json.RawMessage `json:"repo_map,omitempty"`
	Env         json.RawMessage `json:"env,omitempty"`
	CreatedAt   int64           `json:"created_at"`
	UpdatedAt   int64           `json:"updated_at"`
}

// CurrentRun returns the latest run by id (active pipeline).
func (s *Store) CurrentRun() (*Run, error) {
	row := s.DB.QueryRow(`
        SELECT id, feature, COALESCE(branch,''), COALESCE(project_mode,''), COALESCE(mode,''),
               COALESCE(repo_map,''), COALESCE(env,''), created_at, updated_at
        FROM runs ORDER BY id DESC LIMIT 1`)
	var r Run
	var repoMap, env string
	if err := row.Scan(&r.ID, &r.Feature, &r.Branch, &r.ProjectMode, &r.Mode, &repoMap, &env, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if repoMap != "" {
		r.RepoMap = json.RawMessage(repoMap)
	}
	if env != "" {
		r.Env = json.RawMessage(env)
	}
	return &r, nil
}

// FindRunByFeature returns a run matching feature string exactly (most recent).
func (s *Store) FindRunByFeature(feature string) (*Run, error) {
	row := s.DB.QueryRow(`
        SELECT id, feature, COALESCE(branch,''), COALESCE(project_mode,''), COALESCE(mode,''),
               COALESCE(repo_map,''), COALESCE(env,''), created_at, updated_at
        FROM runs WHERE feature = ? ORDER BY id DESC LIMIT 1`, feature)
	var r Run
	var repoMap, env string
	if err := row.Scan(&r.ID, &r.Feature, &r.Branch, &r.ProjectMode, &r.Mode, &repoMap, &env, &r.CreatedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if repoMap != "" {
		r.RepoMap = json.RawMessage(repoMap)
	}
	if env != "" {
		r.Env = json.RawMessage(env)
	}
	return &r, nil
}

// CreateRun inserts a new run + seeds the 10 canonical phases as PENDING.
func (s *Store) CreateRun(r *Run) (int64, error) {
	tx, err := s.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	res, err := tx.Exec(`
        INSERT INTO runs (feature, branch, project_mode, mode, repo_map, env, created_at, updated_at)
        VALUES (?,?,?,?,?,?,?,?)`,
		r.Feature, nullStr(r.Branch), nullStr(r.ProjectMode), nullStr(r.Mode),
		nullRaw(r.RepoMap), nullRaw(r.Env), now, now)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	for _, p := range CanonicalPhases() {
		if _, err := tx.Exec(`INSERT INTO phases(run_id, phase, status) VALUES (?,?,?)`, id, p, "PENDING"); err != nil {
			return 0, err
		}
	}
	return id, tx.Commit()
}

// CanonicalPhases returns the 10 phases in execution order.
func CanonicalPhases() []string {
	return []string{"0", "1", "2'", "3.5", "4", "5", "5.5", "5.6", "6", "7"}
}

// UpdateRunConfig updates mutable config fields on a run.
func (s *Store) UpdateRunConfig(id int64, field, value string) error {
	valid := map[string]bool{"feature": true, "branch": true, "project_mode": true, "mode": true}
	if !valid[field] {
		return fmt.Errorf("invalid config field: %s", field)
	}
	_, err := s.DB.Exec(fmt.Sprintf(`UPDATE runs SET %s=?, updated_at=? WHERE id=?`, field), value, time.Now().Unix(), id)
	return err
}

// SetRunJSON sets repo_map or env (JSON blob fields).
func (s *Store) SetRunJSON(id int64, field string, raw json.RawMessage) error {
	if field != "repo_map" && field != "env" {
		return fmt.Errorf("invalid JSON field: %s", field)
	}
	_, err := s.DB.Exec(fmt.Sprintf(`UPDATE runs SET %s=?, updated_at=? WHERE id=?`, field), string(raw), time.Now().Unix(), id)
	return err
}

// ===== Phases =====

type Phase struct {
	RunID      int64    `json:"run_id"`
	Phase      string   `json:"phase"`
	Status     string   `json:"status"`
	Notes      string   `json:"notes,omitempty"`
	Artifacts  []string `json:"artifacts,omitempty"`
	StartedAt  int64    `json:"started_at,omitempty"`
	FinishedAt int64    `json:"finished_at,omitempty"`
}

func (s *Store) GetPhase(runID int64, phase string) (*Phase, error) {
	row := s.DB.QueryRow(`
        SELECT run_id, phase, status, COALESCE(notes,''), COALESCE(artifacts,''),
               COALESCE(started_at,0), COALESCE(finished_at,0)
        FROM phases WHERE run_id=? AND phase=?`, runID, phase)
	var p Phase
	var arts string
	if err := row.Scan(&p.RunID, &p.Phase, &p.Status, &p.Notes, &arts, &p.StartedAt, &p.FinishedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if arts != "" {
		_ = json.Unmarshal([]byte(arts), &p.Artifacts)
	}
	return &p, nil
}

func (s *Store) AllPhases(runID int64) ([]Phase, error) {
	rows, err := s.DB.Query(`
        SELECT run_id, phase, status, COALESCE(notes,''), COALESCE(artifacts,''),
               COALESCE(started_at,0), COALESCE(finished_at,0)
        FROM phases WHERE run_id=? ORDER BY
          CASE phase
            WHEN '0' THEN 0 WHEN '1' THEN 1 WHEN '2''' THEN 2 WHEN '3.5' THEN 3
            WHEN '4' THEN 4 WHEN '5' THEN 5 WHEN '5.5' THEN 6 WHEN '5.6' THEN 7
            WHEN '6' THEN 8 WHEN '7' THEN 9 ELSE 99 END`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Phase
	for rows.Next() {
		var p Phase
		var arts string
		if err := rows.Scan(&p.RunID, &p.Phase, &p.Status, &p.Notes, &arts, &p.StartedAt, &p.FinishedAt); err != nil {
			return nil, err
		}
		if arts != "" {
			_ = json.Unmarshal([]byte(arts), &p.Artifacts)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) SetPhaseStatus(runID int64, phase, status string, notes string, artifacts []string) error {
	now := time.Now().Unix()
	var artJSON string
	if len(artifacts) > 0 {
		b, _ := json.Marshal(artifacts)
		artJSON = string(b)
	}
	// Start time on IN_PROGRESS; finish time on terminal states.
	switch status {
	case "IN_PROGRESS":
		_, err := s.DB.Exec(`UPDATE phases SET status=?, notes=?, artifacts=?, started_at=COALESCE(started_at,?) WHERE run_id=? AND phase=?`,
			status, nullStr(notes), nullStr(artJSON), now, runID, phase)
		return err
	case "DONE", "SKIPPED", "FAIL":
		_, err := s.DB.Exec(`UPDATE phases SET status=?, notes=?, artifacts=?, finished_at=? WHERE run_id=? AND phase=?`,
			status, nullStr(notes), nullStr(artJSON), now, runID, phase)
		return err
	default:
		_, err := s.DB.Exec(`UPDATE phases SET status=?, notes=?, artifacts=? WHERE run_id=? AND phase=?`,
			status, nullStr(notes), nullStr(artJSON), runID, phase)
		return err
	}
}

// NextPending returns the first phase not in DONE/SKIPPED (execution order).
func (s *Store) NextPending(runID int64) (string, error) {
	ps, err := s.AllPhases(runID)
	if err != nil {
		return "", err
	}
	for _, p := range ps {
		if p.Status != "DONE" && p.Status != "SKIPPED" {
			return p.Phase, nil
		}
	}
	return "", nil
}

// ===== Handoffs =====

type Handoff struct {
	RunID         int64    `json:"run_id"`
	Phase         string   `json:"phase"`
	Summary       string   `json:"summary,omitempty"`
	Decisions     []string `json:"decisions,omitempty"`
	Artifacts     []string `json:"artifacts,omitempty"`
	Traps         []string `json:"traps,omitempty"`
	OpenQuestions []string `json:"open_questions,omitempty"`
	NextInput     string   `json:"next_input,omitempty"`
	Promise       string   `json:"promise,omitempty"`
	RenderedMD    string   `json:"rendered_md,omitempty"`
	CreatedAt     int64    `json:"created_at,omitempty"`
}

func (s *Store) UpsertHandoff(h *Handoff) error {
	now := time.Now().Unix()
	dec, _ := json.Marshal(h.Decisions)
	art, _ := json.Marshal(h.Artifacts)
	tr, _ := json.Marshal(h.Traps)
	oq, _ := json.Marshal(h.OpenQuestions)
	_, err := s.DB.Exec(`
        INSERT INTO handoffs(run_id, phase, summary, decisions, artifacts, traps, open_questions, next_input, promise, rendered_md, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(run_id, phase) DO UPDATE SET
          summary=excluded.summary,
          decisions=excluded.decisions,
          artifacts=excluded.artifacts,
          traps=excluded.traps,
          open_questions=excluded.open_questions,
          next_input=excluded.next_input,
          promise=excluded.promise,
          rendered_md=excluded.rendered_md,
          created_at=excluded.created_at`,
		h.RunID, h.Phase, nullStr(h.Summary), string(dec), string(art), string(tr), string(oq),
		nullStr(h.NextInput), nullStr(h.Promise), nullStr(h.RenderedMD), now)
	return err
}

func (s *Store) GetHandoff(runID int64, phase string) (*Handoff, error) {
	row := s.DB.QueryRow(`
        SELECT run_id, phase, COALESCE(summary,''), COALESCE(decisions,'[]'),
               COALESCE(artifacts,'[]'), COALESCE(traps,'[]'), COALESCE(open_questions,'[]'),
               COALESCE(next_input,''), COALESCE(promise,''), COALESCE(rendered_md,''), created_at
        FROM handoffs WHERE run_id=? AND phase=?`, runID, phase)
	var h Handoff
	var dec, art, tr, oq string
	if err := row.Scan(&h.RunID, &h.Phase, &h.Summary, &dec, &art, &tr, &oq, &h.NextInput, &h.Promise, &h.RenderedMD, &h.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(dec), &h.Decisions)
	_ = json.Unmarshal([]byte(art), &h.Artifacts)
	_ = json.Unmarshal([]byte(tr), &h.Traps)
	_ = json.Unmarshal([]byte(oq), &h.OpenQuestions)
	return &h, nil
}

// ===== Prompts =====

type Prompt struct {
	Name      string `json:"name"`
	Version   int    `json:"version"`
	Content   string `json:"content,omitempty"`
	SHA256    string `json:"sha256"`
	Source    string `json:"source"`
	CreatedAt int64  `json:"created_at"`
}

// LatestPrompt returns most recent version; returns nil if absent.
func (s *Store) LatestPrompt(name string) (*Prompt, error) {
	row := s.DB.QueryRow(`SELECT name, version, content, sha256, source, created_at
        FROM prompts WHERE name=? ORDER BY version DESC LIMIT 1`, name)
	var p Prompt
	if err := row.Scan(&p.Name, &p.Version, &p.Content, &p.SHA256, &p.Source, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// GetPrompt returns specific version; nil if absent.
func (s *Store) GetPrompt(name string, version int) (*Prompt, error) {
	row := s.DB.QueryRow(`SELECT name, version, content, sha256, source, created_at
        FROM prompts WHERE name=? AND version=?`, name, version)
	var p Prompt
	if err := row.Scan(&p.Name, &p.Version, &p.Content, &p.SHA256, &p.Source, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// InsertPrompt writes a new prompt version. Caller handles version numbering.
func (s *Store) InsertPrompt(name, content, source string) (int, error) {
	var next int
	err := s.DB.QueryRow(`SELECT COALESCE(MAX(version),0)+1 FROM prompts WHERE name=?`, name).Scan(&next)
	if err != nil {
		return 0, err
	}
	sum := sha256.Sum256([]byte(content))
	sha := hex.EncodeToString(sum[:])
	_, err = s.DB.Exec(`INSERT INTO prompts(name, version, content, sha256, source, created_at)
        VALUES(?,?,?,?,?,?)`, name, next, content, sha, source, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return next, nil
}

// ===== Findings =====

// Finding mirrors the review.Finding universal shape; kept minimal in this
// package to avoid an import cycle. Callers map their own type into these fields.
type Finding struct {
	RunID          int64
	Cycle          int
	Severity       string
	File           string
	LineStart      int
	LineEnd        int
	Title          string
	EvidenceQuote  string
	FlaggedBy      []string
	Actionable     bool
	DesignDecision bool
	Rationale      string
	FullJSON       string
}

// InsertFinding writes one finding row. Triggers populate fts_findings.
func (s *Store) InsertFinding(f Finding) (int64, error) {
	flaggedBy, _ := json.Marshal(f.FlaggedBy)
	res, err := s.DB.Exec(`
        INSERT INTO findings
          (run_id, cycle, severity, file, line_start, line_end, title,
           evidence_quote, flagged_by, actionable, design_decision,
           rationale, full_json, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		f.RunID, f.Cycle, nullStr(f.Severity), nullStr(f.File),
		f.LineStart, f.LineEnd, nullStr(f.Title),
		nullStr(f.EvidenceQuote), string(flaggedBy),
		boolInt(f.Actionable), boolInt(f.DesignDecision),
		nullStr(f.Rationale), nullStr(f.FullJSON), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// MaxFindingCycle returns the highest cycle number for the given run, or 0 if none.
func (s *Store) MaxFindingCycle(runID int64) (int, error) {
	var max int
	err := s.DB.QueryRow(`SELECT COALESCE(MAX(cycle), 0) FROM findings WHERE run_id=?`, runID).Scan(&max)
	return max, err
}

// ===== Gate evidence =====

type ReviewEvidence struct {
	ID               int64
	RunID            int64
	ReviewDir        string
	ReviewJSONPath   string
	ManifestPath     string
	Verdict          string
	PRNumber         string
	BaseSHA          string
	HeadSHA          string
	PersonasExpected []string
	PersonasRun      []string
	CreatedAt        int64
}

func (s *Store) InsertReviewEvidence(e ReviewEvidence) (int64, error) {
	expected, _ := json.Marshal(e.PersonasExpected)
	run, _ := json.Marshal(e.PersonasRun)
	res, err := s.DB.Exec(`
        INSERT INTO review_evidence
          (run_id, review_dir, review_json_path, manifest_path, verdict,
           pr_number, base_sha, head_sha, personas_expected, personas_run, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		e.RunID, e.ReviewDir, e.ReviewJSONPath, e.ManifestPath, e.Verdict,
		e.PRNumber, e.BaseSHA, e.HeadSHA, string(expected), string(run), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) LatestReviewEvidence(runID int64) (*ReviewEvidence, error) {
	row := s.DB.QueryRow(`
        SELECT id, run_id, review_dir, review_json_path, manifest_path, verdict,
               pr_number, base_sha, head_sha, personas_expected, personas_run, created_at
        FROM review_evidence WHERE run_id=? ORDER BY created_at DESC, id DESC LIMIT 1`, runID)
	var e ReviewEvidence
	var expected, run string
	if err := row.Scan(&e.ID, &e.RunID, &e.ReviewDir, &e.ReviewJSONPath, &e.ManifestPath, &e.Verdict,
		&e.PRNumber, &e.BaseSHA, &e.HeadSHA, &expected, &run, &e.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(expected), &e.PersonasExpected)
	_ = json.Unmarshal([]byte(run), &e.PersonasRun)
	return &e, nil
}

type VerificationRecord struct {
	ID        int64
	RunID     int64
	Phase     string
	Command   string
	Status    string
	LogPath   string
	CreatedAt int64
}

func (s *Store) InsertVerificationRecord(r VerificationRecord) (int64, error) {
	res, err := s.DB.Exec(`
        INSERT INTO verification_records(run_id, phase, command, status, log_path, created_at)
        VALUES (?,?,?,?,?,?)`,
		r.RunID, r.Phase, r.Command, r.Status, r.LogPath, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) VerificationRecords(runID int64, phase string) ([]VerificationRecord, error) {
	rows, err := s.DB.Query(`
        SELECT id, run_id, phase, command, status, log_path, created_at
        FROM verification_records WHERE run_id=? AND phase=?
        ORDER BY created_at, id`, runID, phase)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VerificationRecord
	for rows.Next() {
		var r VerificationRecord
		if err := rows.Scan(&r.ID, &r.RunID, &r.Phase, &r.Command, &r.Status, &r.LogPath, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type ReportEvidence struct {
	ID         int64
	RunID      int64
	ReportPath string
	SHA256     string
	CreatedAt  int64
}

func (s *Store) InsertReportEvidence(e ReportEvidence) (int64, error) {
	res, err := s.DB.Exec(`
        INSERT INTO report_evidence(run_id, report_path, sha256, created_at)
        VALUES (?,?,?,?)`, e.RunID, e.ReportPath, e.SHA256, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) LatestReportEvidence(runID int64) (*ReportEvidence, error) {
	row := s.DB.QueryRow(`
        SELECT id, run_id, report_path, sha256, created_at
        FROM report_evidence WHERE run_id=? ORDER BY created_at DESC, id DESC LIMIT 1`, runID)
	var e ReportEvidence
	if err := row.Scan(&e.ID, &e.RunID, &e.ReportPath, &e.SHA256, &e.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &e, nil
}

// ===== Autonomous run custody =====

type RunSession struct {
	ID             string `json:"id"`
	RunID          int64  `json:"run_id"`
	Provider       string `json:"provider"`
	Mode           string `json:"mode"`
	TokenSHA256    string `json:"-"`
	Status         string `json:"status"`
	StartedAt      int64  `json:"started_at"`
	FinishedAt     int64  `json:"finished_at,omitempty"`
	FailureMessage string `json:"failure_message,omitempty"`
}

func (s *Store) InsertRunSession(sess RunSession) error {
	_, err := s.DB.Exec(`
        INSERT INTO run_sessions
          (id, run_id, provider, mode, token_sha256, status, started_at, finished_at, failure_message)
        VALUES (?,?,?,?,?,?,?,?,?)`,
		sess.ID, sess.RunID, sess.Provider, sess.Mode, sess.TokenSHA256,
		sess.Status, time.Now().Unix(), nullInt(sess.FinishedAt), nullStr(sess.FailureMessage))
	return err
}

func (s *Store) UpdateRunSessionStatus(id, status, failure string) error {
	finished := any(nil)
	if status != "RUNNING" {
		finished = time.Now().Unix()
	}
	_, err := s.DB.Exec(`
        UPDATE run_sessions SET status=?, finished_at=?, failure_message=? WHERE id=?`,
		status, finished, nullStr(failure), id)
	return err
}

func (s *Store) LatestRunSession(runID int64) (*RunSession, error) {
	row := s.DB.QueryRow(`
        SELECT id, run_id, provider, mode, token_sha256, status,
               started_at, COALESCE(finished_at,0), COALESCE(failure_message,'')
        FROM run_sessions WHERE run_id=? ORDER BY started_at DESC LIMIT 1`, runID)
	var sess RunSession
	if err := row.Scan(&sess.ID, &sess.RunID, &sess.Provider, &sess.Mode, &sess.TokenSHA256,
		&sess.Status, &sess.StartedAt, &sess.FinishedAt, &sess.FailureMessage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ActiveRunSession(runID int64) (*RunSession, error) {
	row := s.DB.QueryRow(`
        SELECT id, run_id, provider, mode, token_sha256, status,
               started_at, COALESCE(finished_at,0), COALESCE(failure_message,'')
        FROM run_sessions WHERE run_id=? AND status='RUNNING'
        ORDER BY started_at DESC LIMIT 1`, runID)
	var sess RunSession
	if err := row.Scan(&sess.ID, &sess.RunID, &sess.Provider, &sess.Mode, &sess.TokenSHA256,
		&sess.Status, &sess.StartedAt, &sess.FinishedAt, &sess.FailureMessage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

func (s *Store) ActiveAnyRunSession() (*RunSession, error) {
	row := s.DB.QueryRow(`
        SELECT id, run_id, provider, mode, token_sha256, status,
               started_at, COALESCE(finished_at,0), COALESCE(failure_message,'')
        FROM run_sessions WHERE status='RUNNING'
        ORDER BY started_at DESC LIMIT 1`)
	var sess RunSession
	if err := row.Scan(&sess.ID, &sess.RunID, &sess.Provider, &sess.Mode, &sess.TokenSHA256,
		&sess.Status, &sess.StartedAt, &sess.FinishedAt, &sess.FailureMessage); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &sess, nil
}

type RunEvent struct {
	ID        int64  `json:"id"`
	RunID     int64  `json:"run_id"`
	SessionID string `json:"session_id,omitempty"`
	Event     string `json:"event"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

func (s *Store) InsertRunEvent(e RunEvent) (int64, error) {
	res, err := s.DB.Exec(`
        INSERT INTO run_events(run_id, session_id, event, detail, created_at)
        VALUES (?,?,?,?,?)`, e.RunID, nullStr(e.SessionID), e.Event, nullStr(e.Detail), time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type PRReviewPost struct {
	ID               int64  `json:"id"`
	RunID            int64  `json:"run_id"`
	ReviewEvidenceID int64  `json:"review_evidence_id,omitempty"`
	PRNumber         string `json:"pr_number"`
	CommentID        string `json:"comment_id"`
	CommentURL       string `json:"comment_url"`
	BodySHA256       string `json:"body_sha256"`
	PostedBy         string `json:"posted_by"`
	CreatedAt        int64  `json:"created_at"`
}

func (s *Store) InsertPRReviewPost(p PRReviewPost) (int64, error) {
	res, err := s.DB.Exec(`
        INSERT INTO pr_review_posts
          (run_id, review_evidence_id, pr_number, comment_id, comment_url, body_sha256, posted_by, created_at)
        VALUES (?,?,?,?,?,?,?,?)`,
		p.RunID, nullInt(p.ReviewEvidenceID), p.PRNumber, p.CommentID, p.CommentURL,
		p.BodySHA256, p.PostedBy, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) LatestPRReviewPost(runID int64) (*PRReviewPost, error) {
	row := s.DB.QueryRow(`
        SELECT id, run_id, COALESCE(review_evidence_id,0), pr_number, comment_id,
               comment_url, body_sha256, posted_by, created_at
        FROM pr_review_posts WHERE run_id=? ORDER BY created_at DESC, id DESC LIMIT 1`, runID)
	var p PRReviewPost
	if err := row.Scan(&p.ID, &p.RunID, &p.ReviewEvidenceID, &p.PRNumber, &p.CommentID,
		&p.CommentURL, &p.BodySHA256, &p.PostedBy, &p.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

// ===== Tasks =====

type Task struct {
	RunID     int64
	Repo      string
	TaskID    string
	Kind      string // dev | test | fix
	Status    string // pending | in_progress | done | blocked
	Title     string
	DependsOn []string
	Content   string
}

// UpsertTask inserts or updates a task row. Triggers populate fts_tasks.
func (s *Store) UpsertTask(t Task) error {
	dep, _ := json.Marshal(t.DependsOn)
	_, err := s.DB.Exec(`
        INSERT INTO tasks
          (run_id, repo, task_id, kind, status, title, depends_on, content, created_at)
        VALUES (?,?,?,?,?,?,?,?,?)
        ON CONFLICT(run_id, repo, task_id) DO UPDATE SET
          kind=excluded.kind,
          status=excluded.status,
          title=excluded.title,
          depends_on=excluded.depends_on,
          content=excluded.content`,
		t.RunID, t.Repo, t.TaskID, nullStr(t.Kind), nullStr(t.Status),
		nullStr(t.Title), string(dep), nullStr(t.Content), time.Now().Unix())
	return err
}

// ===== Installed artifacts =====

type InstalledArtifact struct {
	RelPath       string
	InstallPath   string
	SHA256        string
	BinaryVersion string
	Provider      string
	PackID        string
	PackVersion   string
	Kind          string
	LogicalName   string
	UserModified  bool
	InstalledAt   int64
}

func (s *Store) UpsertArtifact(a InstalledArtifact) error {
	if a.InstallPath == "" {
		a.InstallPath = a.RelPath
	}
	if a.Provider == "" {
		a.Provider = "claude"
	}
	if a.PackID == "" {
		a.PackID = "codedungeon-claude"
	}
	if a.PackVersion == "" {
		a.PackVersion = "1"
	}
	_, err := s.DB.Exec(`
        INSERT INTO installed_artifacts(rel_path, install_path, sha256, binary_version, provider, pack_id, pack_version, kind, logical_name, user_modified, installed_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(rel_path) DO UPDATE SET
          install_path=excluded.install_path,
          sha256=excluded.sha256,
          binary_version=excluded.binary_version,
          provider=excluded.provider,
          pack_id=excluded.pack_id,
          pack_version=excluded.pack_version,
          kind=excluded.kind,
          logical_name=excluded.logical_name,
          user_modified=excluded.user_modified,
          installed_at=excluded.installed_at`,
		a.RelPath, a.InstallPath, a.SHA256, a.BinaryVersion, a.Provider, a.PackID, a.PackVersion,
		a.Kind, a.LogicalName, boolInt(a.UserModified), a.InstalledAt)
	return err
}

func (s *Store) GetArtifact(relPath string) (*InstalledArtifact, error) {
	row := s.DB.QueryRow(`SELECT rel_path, install_path, sha256, binary_version, provider, pack_id, pack_version, kind, logical_name, user_modified, installed_at FROM installed_artifacts WHERE rel_path=?`, relPath)
	var a InstalledArtifact
	var um int
	if err := row.Scan(&a.RelPath, &a.InstallPath, &a.SHA256, &a.BinaryVersion, &a.Provider, &a.PackID, &a.PackVersion, &a.Kind, &a.LogicalName, &um, &a.InstalledAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	a.UserModified = um == 1
	return &a, nil
}

func (s *Store) ListArtifacts() ([]InstalledArtifact, error) {
	rows, err := s.DB.Query(`SELECT rel_path, install_path, sha256, binary_version, provider, pack_id, pack_version, kind, logical_name, user_modified, installed_at FROM installed_artifacts ORDER BY install_path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstalledArtifact
	for rows.Next() {
		var a InstalledArtifact
		var um int
		if err := rows.Scan(&a.RelPath, &a.InstallPath, &a.SHA256, &a.BinaryVersion, &a.Provider, &a.PackID, &a.PackVersion, &a.Kind, &a.LogicalName, &um, &a.InstalledAt); err != nil {
			return nil, err
		}
		a.UserModified = um == 1
		out = append(out, a)
	}
	return out, rows.Err()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListPrompts returns {name: latest_version}.
func (s *Store) ListPrompts() (map[string]int, error) {
	rows, err := s.DB.Query(`SELECT name, MAX(version) FROM prompts GROUP BY name ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var n string
		var v int
		if err := rows.Scan(&n, &v); err != nil {
			return nil, err
		}
		out[n] = v
	}
	return out, rows.Err()
}

// ===== FTS Search =====

type SearchHit struct {
	Table string         `json:"table"`
	RowID int64          `json:"rowid"`
	Rank  float64        `json:"rank"`
	Row   map[string]any `json:"row"`
}

func (s *Store) Search(table, query string, limit int) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 20
	}
	ftsTable := "fts_" + table
	allowed := map[string]bool{"handoffs": true, "prompts": true, "findings": true, "tasks": true}
	if !allowed[table] {
		return nil, fmt.Errorf("unknown table: %s (allowed: handoffs, prompts, findings, tasks)", table)
	}
	sqlStr := fmt.Sprintf(`SELECT rowid, rank FROM %s WHERE %s MATCH ? ORDER BY rank LIMIT ?`, ftsTable, ftsTable)
	rows, err := s.DB.Query(sqlStr, query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []SearchHit
	for rows.Next() {
		var h SearchHit
		h.Table = table
		if err := rows.Scan(&h.RowID, &h.Rank); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Join back to source table for the row body.
	for i := range hits {
		row, err := s.fetchRow(table, hits[i].RowID)
		if err == nil {
			hits[i].Row = row
		}
	}
	return hits, nil
}

func (s *Store) fetchRow(table string, rowid int64) (map[string]any, error) {
	var query string
	switch table {
	case "handoffs":
		query = `SELECT run_id, phase, summary, COALESCE(promise,''), COALESCE(next_input,'') FROM handoffs WHERE rowid=?`
	case "prompts":
		query = `SELECT name, version, substr(content,1,200) FROM prompts WHERE rowid=?`
	case "findings":
		query = `SELECT run_id, cycle, severity, file, line_start, title FROM findings WHERE id=?`
	case "tasks":
		query = `SELECT run_id, repo, task_id, status, title FROM tasks WHERE rowid=?`
	default:
		return nil, fmt.Errorf("unknown table")
	}
	rows, err := s.DB.Query(query, rowid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	cols, _ := rows.Columns()
	vals := make([]any, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}
	if err := rows.Scan(ptrs...); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for i, c := range cols {
		out[c] = vals[i]
	}
	return out, nil
}

// ===== helpers =====

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullRaw(r json.RawMessage) any {
	if len(r) == 0 {
		return nil
	}
	return string(r)
}

func nullInt(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}

// SplitArtifacts is used by flag parsing (comma/newline tolerant).
func SplitArtifacts(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
