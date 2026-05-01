package projectcontext

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/loldinis/codedungeon/internal/cartographer"
	"github.com/loldinis/codedungeon/internal/db"
)

const (
	ContextRel  = ".codedungeon/project-context.md"
	ProposalRel = ".codedungeon/project-context.proposal.md"

	ModeAuto     = "auto"
	ModeExisting = "existing"
	ModeEmpty    = "empty"

	StatusMissing  = "missing"
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusStale    = "stale"
	StatusRejected = "rejected"
)

type ContextVersion struct {
	ID           int64  `json:"id"`
	Version      int    `json:"version"`
	Body         string `json:"body"`
	SHA256       string `json:"sha256"`
	SourceDigest string `json:"source_digest"`
	ApprovedBy   string `json:"approved_by"`
	CreatedAt    int64  `json:"created_at"`
}

type ContextProposal struct {
	ID           int64  `json:"id"`
	RunID        int64  `json:"run_id,omitempty"`
	BaseVersion  int    `json:"base_version"`
	ProposedBody string `json:"proposed_body"`
	DiffSummary  string `json:"diff_summary"`
	Status       string `json:"status"`
	CreatedAt    int64  `json:"created_at"`
	UpdatedAt    int64  `json:"updated_at"`
	ApprovedAt   int64  `json:"approved_at,omitempty"`
	ApprovedBy   string `json:"approved_by,omitempty"`
}

type AuditRecord struct {
	ID        int64  `json:"id"`
	RunID     int64  `json:"run_id,omitempty"`
	Kind      string `json:"kind"`
	Ref       string `json:"ref,omitempty"`
	Summary   string `json:"summary"`
	FullJSON  string `json:"full_json"`
	CreatedAt int64  `json:"created_at"`
}

type StatusResult struct {
	OK               bool   `json:"ok"`
	Status           string `json:"status"`
	ContextPath      string `json:"context_path"`
	ProposalPath     string `json:"proposal_path"`
	ActiveVersion    int    `json:"active_version,omitempty"`
	SHA256           string `json:"sha256,omitempty"`
	StaleReason      string `json:"stale_reason,omitempty"`
	PendingProposals int    `json:"pending_proposals"`
}

type InitOptions struct {
	Mode        string
	FirstPrompt string
}

type InitResult struct {
	Mode      string              `json:"mode"`
	Proposal  ContextProposal     `json:"proposal"`
	Scan      cartographer.Result `json:"scan,omitempty"`
	Status    StatusResult        `json:"status"`
	Questions []string            `json:"questions,omitempty"`
}

type Envelope struct {
	ProjectRulesStatus string `json:"PROJECT_RULES_STATUS"`
	ProjectRulesDigest string `json:"PROJECT_RULES_DIGEST"`
	ProjectRulesRead   string `json:"PROJECT_RULES_READ"`
	ContextStatus      string `json:"PROJECT_CONTEXT_STATUS"`
	ContextDigest      string `json:"PROJECT_CONTEXT_DIGEST,omitempty"`
}

type Ledger interface {
	LatestContextVersion() (*ContextVersion, error)
	InsertContextVersion(ContextVersion) (int64, error)
	LatestContextProposal() (*ContextProposal, error)
	ContextProposal(id int64) (*ContextProposal, error)
	InsertContextProposal(ContextProposal) (int64, error)
	UpdateContextProposalStatus(id int64, status, approvedBy string) error
	PendingContextProposalCount() (int, error)
	InsertContextAudit(AuditRecord) (int64, error)
	AuditRecords(query string, limit int) ([]AuditRecord, error)
	Run(id int64) (*db.Run, error)
	LatestReviewEvidence(runID int64) (*db.ReviewEvidence, error)
	VerificationRecords(runID int64, phase string) ([]db.VerificationRecord, error)
	LatestReportEvidence(runID int64) (*db.ReportEvidence, error)
}

type SQLiteStore struct {
	store *db.Store
}

func NewSQLiteStore(store *db.Store) *SQLiteStore {
	return &SQLiteStore{store: store}
}

func Status(root string, ledger Ledger) (StatusResult, error) {
	result := StatusResult{
		Status:       StatusMissing,
		ContextPath:  ContextRel,
		ProposalPath: ProposalRel,
	}
	pending, err := ledger.PendingContextProposalCount()
	if err != nil {
		return result, err
	}
	result.PendingProposals = pending
	version, err := ledger.LatestContextVersion()
	if err != nil {
		return result, err
	}
	if version == nil {
		if pending > 0 {
			result.Status = StatusPending
		}
		return result, nil
	}
	result.ActiveVersion = version.Version
	result.SHA256 = version.SHA256
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ContextRel)))
	if err != nil {
		result.Status = StatusStale
		result.StaleReason = "approved context file missing"
		return result, nil
	}
	if digestBytes(body) != version.SHA256 {
		result.Status = StatusStale
		result.StaleReason = "approved context file digest changed"
		return result, nil
	}
	if pending > 0 {
		result.Status = StatusPending
		return result, nil
	}
	currentSourceDigest := sourceDigest(root)
	if version.SourceDigest != "" && currentSourceDigest != "" && currentSourceDigest != version.SourceDigest {
		result.Status = StatusStale
		result.StaleReason = "source digest changed"
		return result, nil
	}
	result.Status = StatusApproved
	result.OK = true
	return result, nil
}

func Init(root string, ledger Ledger, opts InitOptions) (InitResult, error) {
	mode := strings.TrimSpace(opts.Mode)
	if mode == "" {
		mode = ModeAuto
	}
	scan, err := cartographer.Scan(root, cartographer.Options{MaxTokens: 50000})
	if err != nil {
		return InitResult{}, err
	}
	if mode == ModeAuto {
		if hasProjectSource(scan) {
			mode = ModeExisting
		} else {
			mode = ModeEmpty
		}
	}
	body := initialContextBody(mode, opts.FirstPrompt, scan)
	baseVersion := 0
	if latest, err := ledger.LatestContextVersion(); err != nil {
		return InitResult{}, err
	} else if latest != nil {
		baseVersion = latest.Version
	}
	proposal := ContextProposal{
		BaseVersion:  baseVersion,
		ProposedBody: body,
		DiffSummary:  fmt.Sprintf("Initial project context proposal for %s project.", mode),
		Status:       StatusPending,
	}
	id, err := ledger.InsertContextProposal(proposal)
	if err != nil {
		return InitResult{}, err
	}
	proposal.ID = id
	if err := writeProposal(root, body); err != nil {
		return InitResult{}, err
	}
	questions := initialQuestions(mode)
	auditPayload := map[string]any{
		"mode":         mode,
		"first_prompt": opts.FirstPrompt,
		"scan":         scan,
		"questions":    questions,
	}
	_ = insertAuditJSON(ledger, AuditRecord{
		Kind:    "init",
		Summary: "Initial project context proposal generated.",
	}, auditPayload)
	status, err := Status(root, ledger)
	if err != nil {
		return InitResult{}, err
	}
	return InitResult{Mode: mode, Proposal: proposal, Scan: scan, Status: status, Questions: questions}, nil
}

func ProposeFromRun(root string, ledger Ledger, runID int64) (ContextProposal, error) {
	run, err := ledger.Run(runID)
	if err != nil {
		return ContextProposal{}, err
	}
	if run == nil {
		return ContextProposal{}, fmt.Errorf("run not found: %d", runID)
	}
	baseVersion := 0
	baseBody := defaultContextBody()
	if latest, err := ledger.LatestContextVersion(); err != nil {
		return ContextProposal{}, err
	} else if latest != nil {
		baseVersion = latest.Version
		baseBody = latest.Body
	} else if body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ContextRel))); err == nil && strings.TrimSpace(string(body)) != "" {
		baseBody = string(body)
	}
	review, _ := ledger.LatestReviewEvidence(runID)
	verification, _ := ledger.VerificationRecords(runID, "6")
	report, _ := ledger.LatestReportEvidence(runID)
	proposed := appendRunBullets(baseBody, run, review, verification, report)
	proposal := ContextProposal{
		RunID:        runID,
		BaseVersion:  baseVersion,
		ProposedBody: proposed,
		DiffSummary:  fmt.Sprintf("Add run %d outcome to direct project context.", runID),
		Status:       StatusPending,
	}
	id, err := ledger.InsertContextProposal(proposal)
	if err != nil {
		return ContextProposal{}, err
	}
	proposal.ID = id
	if err := writeProposal(root, proposed); err != nil {
		return ContextProposal{}, err
	}
	insertRunAudit(ledger, run, review, verification, report)
	return proposal, nil
}

func ApproveProposal(root string, ledger Ledger, proposalID int64, approvedBy string) (ContextVersion, error) {
	proposal, err := ledger.ContextProposal(proposalID)
	if err != nil {
		return ContextVersion{}, err
	}
	if proposal == nil {
		return ContextVersion{}, fmt.Errorf("project context proposal not found: %d", proposalID)
	}
	if proposal.Status != StatusPending {
		return ContextVersion{}, fmt.Errorf("project context proposal %d is %s", proposalID, proposal.Status)
	}
	latest, err := ledger.LatestContextVersion()
	if err != nil {
		return ContextVersion{}, err
	}
	nextVersion := 1
	if latest != nil {
		nextVersion = latest.Version + 1
	}
	if strings.TrimSpace(approvedBy) == "" {
		approvedBy = "unknown"
	}
	version := ContextVersion{
		Version:      nextVersion,
		Body:         normalizeContextBody(proposal.ProposedBody),
		SHA256:       digestString(normalizeContextBody(proposal.ProposedBody)),
		SourceDigest: sourceDigest(root),
		ApprovedBy:   approvedBy,
	}
	id, err := ledger.InsertContextVersion(version)
	if err != nil {
		return ContextVersion{}, err
	}
	version.ID = id
	if err := ledger.UpdateContextProposalStatus(proposalID, StatusApproved, approvedBy); err != nil {
		return ContextVersion{}, err
	}
	if err := writeContext(root, version.Body); err != nil {
		return ContextVersion{}, err
	}
	_ = os.Remove(filepath.Join(root, filepath.FromSlash(ProposalRel)))
	return version, nil
}

func RejectProposal(root string, ledger Ledger, proposalID int64) error {
	proposal, err := ledger.ContextProposal(proposalID)
	if err != nil {
		return err
	}
	if proposal == nil {
		return fmt.Errorf("project context proposal not found: %d", proposalID)
	}
	switch proposal.Status {
	case StatusPending:
		if err := ledger.UpdateContextProposalStatus(proposalID, StatusRejected, ""); err != nil {
			return err
		}
	case StatusRejected:
	default:
		return fmt.Errorf("project context proposal %d is %s", proposalID, proposal.Status)
	}
	pending, err := ledger.PendingContextProposalCount()
	if err != nil {
		return err
	}
	if pending == 0 {
		_ = os.Remove(filepath.Join(root, filepath.FromSlash(ProposalRel)))
	}
	return nil
}

func BuildEnvelope(root string, ledger Ledger, rulesStatus, rulesDigest string) (Envelope, error) {
	status, err := Status(root, ledger)
	if err != nil {
		return Envelope{}, err
	}
	if rulesStatus == "" {
		rulesStatus = StatusMissing
	}
	if rulesDigest == "" {
		rulesDigest = "none"
	}
	return Envelope{
		ProjectRulesStatus: rulesStatus,
		ProjectRulesDigest: rulesDigest,
		ProjectRulesRead:   "yes",
		ContextStatus:      status.Status,
		ContextDigest:      status.SHA256,
	}, nil
}

func (s *SQLiteStore) LatestContextVersion() (*ContextVersion, error) {
	row := s.store.DB.QueryRow(`
        SELECT id, version, body, sha256, source_digest, approved_by, created_at
        FROM project_context_versions ORDER BY version DESC LIMIT 1`)
	var v ContextVersion
	if err := row.Scan(&v.ID, &v.Version, &v.Body, &v.SHA256, &v.SourceDigest, &v.ApprovedBy, &v.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &v, nil
}

func (s *SQLiteStore) InsertContextVersion(v ContextVersion) (int64, error) {
	now := time.Now().Unix()
	res, err := s.store.DB.Exec(`
        INSERT INTO project_context_versions(version, body, sha256, source_digest, approved_by, created_at)
        VALUES (?,?,?,?,?,?)`, v.Version, v.Body, v.SHA256, v.SourceDigest, v.ApprovedBy, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) LatestContextProposal() (*ContextProposal, error) {
	row := s.store.DB.QueryRow(`
        SELECT id, COALESCE(run_id,0), base_version, proposed_body, diff_summary, status,
               created_at, updated_at, COALESCE(approved_at,0), COALESCE(approved_by,'')
        FROM project_context_proposals ORDER BY created_at DESC, id DESC LIMIT 1`)
	return scanProposal(row)
}

func (s *SQLiteStore) ContextProposal(id int64) (*ContextProposal, error) {
	row := s.store.DB.QueryRow(`
        SELECT id, COALESCE(run_id,0), base_version, proposed_body, diff_summary, status,
               created_at, updated_at, COALESCE(approved_at,0), COALESCE(approved_by,'')
        FROM project_context_proposals WHERE id=?`, id)
	return scanProposal(row)
}

func (s *SQLiteStore) InsertContextProposal(p ContextProposal) (int64, error) {
	now := time.Now().Unix()
	res, err := s.store.DB.Exec(`
        INSERT INTO project_context_proposals(run_id, base_version, proposed_body, diff_summary, status, created_at, updated_at)
        VALUES (?,?,?,?,?,?,?)`, nullInt(p.RunID), p.BaseVersion, normalizeContextBody(p.ProposedBody), p.DiffSummary, p.Status, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) UpdateContextProposalStatus(id int64, status, approvedBy string) error {
	now := time.Now().Unix()
	approvedAt := any(nil)
	if status == StatusApproved {
		approvedAt = now
	}
	res, err := s.store.DB.Exec(`
        UPDATE project_context_proposals
        SET status=?, updated_at=?, approved_at=?, approved_by=?
        WHERE id=?`, status, now, approvedAt, nullStr(approvedBy), id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("project context proposal not found: %d", id)
	}
	return nil
}

func (s *SQLiteStore) PendingContextProposalCount() (int, error) {
	var count int
	err := s.store.DB.QueryRow(`SELECT COUNT(1) FROM project_context_proposals WHERE status=?`, StatusPending).Scan(&count)
	return count, err
}

func (s *SQLiteStore) InsertContextAudit(a AuditRecord) (int64, error) {
	res, err := s.store.DB.Exec(`
        INSERT INTO project_context_audit(run_id, kind, ref, summary, full_json, created_at)
        VALUES (?,?,?,?,?,?)`, nullInt(a.RunID), a.Kind, nullStr(a.Ref), a.Summary, a.FullJSON, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) AuditRecords(query string, limit int) ([]AuditRecord, error) {
	if limit <= 0 {
		limit = 20
	}
	var rows *sql.Rows
	var err error
	if strings.TrimSpace(query) == "" {
		rows, err = s.store.DB.Query(`
            SELECT id, COALESCE(run_id,0), kind, COALESCE(ref,''), summary, full_json, created_at
            FROM project_context_audit ORDER BY created_at DESC, id DESC LIMIT ?`, limit)
	} else {
		match := auditFTSQuery(query)
		like := "%" + strings.TrimSpace(query) + "%"
		rows, err = s.store.DB.Query(`
            WITH matched(rowid) AS (
                SELECT rowid FROM fts_project_context_audit WHERE fts_project_context_audit MATCH ?
                UNION
                SELECT id FROM project_context_audit WHERE kind LIKE ? OR COALESCE(ref,'') LIKE ?
            )
            SELECT a.id, COALESCE(a.run_id,0), a.kind, COALESCE(a.ref,''), a.summary, a.full_json, a.created_at
            FROM matched m
            JOIN project_context_audit a ON a.id = m.rowid
            ORDER BY a.created_at DESC, a.id DESC LIMIT ?`, match, like, like, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditRecord{}
	for rows.Next() {
		var a AuditRecord
		if err := rows.Scan(&a.ID, &a.RunID, &a.Kind, &a.Ref, &a.Summary, &a.FullJSON, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) Run(id int64) (*db.Run, error) { return s.store.GetRun(id) }

func (s *SQLiteStore) LatestReviewEvidence(runID int64) (*db.ReviewEvidence, error) {
	return s.store.LatestReviewEvidence(runID)
}

func (s *SQLiteStore) VerificationRecords(runID int64, phase string) ([]db.VerificationRecord, error) {
	return s.store.VerificationRecords(runID, phase)
}

func (s *SQLiteStore) LatestReportEvidence(runID int64) (*db.ReportEvidence, error) {
	return s.store.LatestReportEvidence(runID)
}

func scanProposal(row *sql.Row) (*ContextProposal, error) {
	var p ContextProposal
	if err := row.Scan(&p.ID, &p.RunID, &p.BaseVersion, &p.ProposedBody, &p.DiffSummary,
		&p.Status, &p.CreatedAt, &p.UpdatedAt, &p.ApprovedAt, &p.ApprovedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func hasProjectSource(scan cartographer.Result) bool {
	for _, file := range scan.Files {
		if isOperationalSource(file.Path) {
			return true
		}
	}
	return false
}

func isOperationalSource(rel string) bool {
	rel = filepath.ToSlash(rel)
	if strings.HasPrefix(rel, ".codedungeon/") || strings.HasPrefix(rel, ".codex/") ||
		strings.HasPrefix(rel, ".claude/") || strings.HasPrefix(rel, ".agents/") {
		return false
	}
	base := filepath.Base(rel)
	switch base {
	case "README.md", "AGENTS.md", "CLAUDE.md", "go.mod", "package.json", "Cargo.toml", "pyproject.toml":
		return true
	}
	return strings.Contains(rel, "/") && !strings.HasPrefix(rel, ".")
}

func initialContextBody(mode, firstPrompt string, scan cartographer.Result) string {
	if mode == ModeEmpty {
		return normalizeContextBody(fmt.Sprintf(`# Project Context

## Current Project
- Project starts from first prompt: %s.

## Workflow Notes
- Project context is pending user approval before becoming active.
`, fallback(firstPrompt, "initial project planning")))
	}
	files := representativeFiles(scan, 5)
	count := operationalFileCount(scan)
	return normalizeContextBody(fmt.Sprintf(`# Project Context

## Current Project
- Project has %d scanned context/source file(s).
- Primary files include %s.

## Workflow Notes
- Project context is pending user approval before becoming active.
`, count, strings.Join(files, ", ")))
}

func defaultContextBody() string {
	return "# Project Context\n\n## Current Project\n- Project context has not been approved yet.\n"
}

func appendRunBullets(base string, run *db.Run, review *db.ReviewEvidence, verification []db.VerificationRecord, report *db.ReportEvidence) string {
	lines := []string{strings.TrimSpace(base)}
	if !strings.Contains(base, "## Recent Changes") {
		lines = append(lines, "", "## Recent Changes")
	}
	feature := sanitizeSentence(run.Feature)
	lines = append(lines, fmt.Sprintf("- Project now has %s.", fallback(feature, fmt.Sprintf("run %d changes", run.ID))))
	if review != nil && review.Verdict != "" {
		lines = append(lines, fmt.Sprintf("- Latest code review verdict is %s.", review.Verdict))
	}
	passCount := 0
	for _, record := range verification {
		if record.Status == "PASS" && record.SupersededAt == 0 {
			passCount++
		}
	}
	if passCount > 0 {
		lines = append(lines, fmt.Sprintf("- Latest verification passed with %d command(s).", passCount))
	}
	if report != nil {
		lines = append(lines, "- Latest final report evidence was recorded.")
	}
	return normalizeContextBody(strings.Join(lines, "\n"))
}

func representativeFiles(scan cartographer.Result, limit int) []string {
	var files []string
	for _, file := range scan.Files {
		if isOperationalSource(file.Path) {
			files = append(files, file.Path)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return []string{"no source files yet"}
	}
	if len(files) > limit {
		files = files[:limit]
	}
	return files
}

func operationalFileCount(scan cartographer.Result) int {
	count := 0
	for _, file := range scan.Files {
		if isOperationalSource(file.Path) {
			count++
		}
	}
	return count
}

func initialQuestions(mode string) []string {
	if mode == ModeEmpty {
		return []string{"What product goal should define the initial project context?"}
	}
	return []string{"Which existing project constraints should be treated as mandatory rules?"}
}

func insertRunAudit(ledger Ledger, run *db.Run, review *db.ReviewEvidence, verification []db.VerificationRecord, report *db.ReportEvidence) {
	_ = insertAuditJSON(ledger, AuditRecord{RunID: run.ID, Kind: "run", Ref: fmt.Sprintf("run:%d", run.ID), Summary: "Run outcome considered for project context."}, run)
	if review != nil {
		_ = insertAuditJSON(ledger, AuditRecord{RunID: run.ID, Kind: "review", Ref: review.ReviewJSONPath, Summary: "Code review evidence considered for project context."}, review)
	}
	if len(verification) > 0 {
		_ = insertAuditJSON(ledger, AuditRecord{RunID: run.ID, Kind: "verification", Ref: "phase:6", Summary: "Verification evidence considered for project context."}, verification)
	}
	if report != nil {
		_ = insertAuditJSON(ledger, AuditRecord{RunID: run.ID, Kind: "report", Ref: report.ReportPath, Summary: "Final report evidence considered for project context."}, report)
	}
}

func insertAuditJSON(ledger Ledger, record AuditRecord, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	record.FullJSON = string(body)
	_, err = ledger.InsertContextAudit(record)
	return err
}

func auditFTSQuery(query string) string {
	var terms []string
	for _, raw := range strings.Fields(strings.ToLower(query)) {
		cleaned := strings.Map(func(r rune) rune {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
				return r
			}
			return ' '
		}, raw)
		for _, term := range strings.Fields(cleaned) {
			terms = append(terms, term+"*")
		}
	}
	if len(terms) == 0 {
		return `""`
	}
	return strings.Join(terms, " AND ")
}

func writeProposal(root, body string) error {
	return writeProjectContextFile(root, ProposalRel, body)
}

func writeContext(root, body string) error {
	return writeProjectContextFile(root, ContextRel, body)
}

func writeProjectContextFile(root, rel, body string) error {
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(normalizeContextBody(body)), 0o644)
}

func normalizeContextBody(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		body = defaultContextBody()
	}
	return body + "\n"
}

func sourceDigest(root string) string {
	scan, err := cartographer.Scan(root, cartographer.Options{MaxTokens: 50000})
	if err != nil {
		return ""
	}
	h := sha256.New()
	for _, file := range scan.Files {
		if !isOperationalSource(file.Path) {
			continue
		}
		path := filepath.Join(root, filepath.FromSlash(file.Path))
		body, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		h.Write([]byte(file.Path))
		h.Write([]byte{0})
		h.Write(body)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func digestString(body string) string {
	return digestBytes([]byte(body))
}

func digestBytes(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func sanitizeSentence(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimSuffix(value, ".")
	return value
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func nullStr(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}
