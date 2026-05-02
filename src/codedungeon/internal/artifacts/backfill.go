package artifacts

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/loldinis/codedungeon/internal/db"
)

func BackfillRun(store *db.Store, root string, runID int64) (int, error) {
	if store == nil {
		return 0, fmt.Errorf("artifact backfill requires a store")
	}
	if runID <= 0 {
		return 0, fmt.Errorf("artifact backfill requires --run")
	}
	registry := NewRegistry(store, root)
	backfill := backfiller{store: store, registry: registry, root: root, runID: runID}
	if backfill.root == "" {
		backfill.root, _ = os.Getwd()
	}
	for _, fn := range []func() error{
		backfill.qa,
		backfill.review,
		backfill.verification,
		backfill.report,
		backfill.planning,
		backfill.execution,
		backfill.trace,
		backfill.phasesAndHandoffs,
	} {
		if err := fn(); err != nil {
			return backfill.count, err
		}
	}
	return backfill.count, nil
}

type backfiller struct {
	store    *db.Store
	registry Registry
	root     string
	runID    int64
	count    int
}

func (b *backfiller) register(record Record) error {
	if strings.TrimSpace(record.Path) == "" {
		return nil
	}
	abs, _, err := b.registry.resolvePath(record.Path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return nil
	}
	if record.RunID == 0 {
		record.RunID = b.runID
	}
	if _, err := b.registry.Register(record); err != nil {
		return err
	}
	b.count++
	return nil
}

func (b *backfiller) qa() error {
	rows, err := b.store.DB.Query(`
        SELECT id, COALESCE(execution_id,''), entrypoint, mode, status, root,
               COALESCE(plan_path,''), evidence_dir
        FROM qa_sessions
        WHERE run_id=?
        ORDER BY started_at, rowid`, b.runID)
	if err != nil {
		return err
	}
	var sessions []db.QASession
	for rows.Next() {
		var session db.QASession
		if err := rows.Scan(&session.ID, &session.ExecutionID, &session.Entrypoint, &session.Mode,
			&session.Status, &session.Root, &session.PlanPath, &session.EvidenceDir); err != nil {
			_ = rows.Close()
			return err
		}
		sessions = append(sessions, session)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, session := range sessions {
		ownerID := session.ID
		metadata := map[string]any{"status": session.Status, "mode": session.Mode, "entrypoint": session.Entrypoint}
		if err := b.register(Record{Module: "qa", OwnerType: "qa_session", OwnerID: ownerID, Phase: "6", Role: "directory", Kind: "directory", Path: session.EvidenceDir, Metadata: metadata}); err != nil {
			return err
		}
		for _, item := range []struct {
			role string
			kind string
			path string
		}{
			{"request", "json", filepath.Join(session.EvidenceDir, "request.json")},
			{"preflight", "json", filepath.Join(session.EvidenceDir, "preflight.json")},
			{"findings", "json", filepath.Join(session.EvidenceDir, "findings.json")},
			{"summary", "markdown", filepath.Join(session.EvidenceDir, "summary.md")},
			{"result", "json", filepath.Join(session.EvidenceDir, "result.json")},
			{"plan", kindForPath(session.PlanPath), session.PlanPath},
		} {
			if err := b.register(Record{Module: "qa", OwnerType: "qa_session", OwnerID: ownerID, Phase: "6", Role: item.role, Kind: item.kind, Path: item.path, Metadata: metadata}); err != nil {
				return err
			}
		}
		checks, err := b.store.QAChecks(session.ID)
		if err != nil {
			return err
		}
		for _, check := range checks {
			checkID := check.ID
			if strings.TrimSpace(checkID) == "" {
				checkID = session.ID + ":" + check.Name
			}
			checkMeta := map[string]any{"session_id": session.ID, "status": check.Status, "name": check.Name, "kind": check.Kind}
			for _, item := range []struct {
				role string
				path string
			}{
				{"log", check.LogPath},
				{"report", check.ReportPath},
			} {
				if err := b.register(Record{Module: "qa", OwnerType: "qa_check", OwnerID: checkID, Phase: "6", Role: item.role, Kind: kindForPath(item.path), Path: item.path, Metadata: checkMeta}); err != nil {
					return err
				}
			}
			for _, path := range check.Artifacts {
				if err := b.register(Record{Module: "qa", OwnerType: "qa_check", OwnerID: checkID, Phase: "6", Role: "artifact", Kind: kindForPath(path), Path: path, Metadata: checkMeta}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *backfiller) review() error {
	rows, err := b.store.DB.Query(`
        SELECT id, review_dir, review_json_path, manifest_path, verdict, pr_number
        FROM review_evidence
        WHERE run_id=?
        ORDER BY created_at, id`, b.runID)
	if err != nil {
		return err
	}
	type reviewRow struct {
		id       int64
		dir      string
		jsonPath string
		manifest string
		verdict  string
		prNumber string
	}
	var reviews []reviewRow
	for rows.Next() {
		var row reviewRow
		if err := rows.Scan(&row.id, &row.dir, &row.jsonPath, &row.manifest, &row.verdict, &row.prNumber); err != nil {
			_ = rows.Close()
			return err
		}
		reviews = append(reviews, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range reviews {
		ownerID := strconv.FormatInt(row.id, 10)
		meta := map[string]any{"verdict": row.verdict, "pr_number": row.prNumber}
		for _, item := range []struct {
			role string
			kind string
			path string
		}{
			{"directory", "directory", row.dir},
			{"review_json", "json", row.jsonPath},
			{"manifest", "json", row.manifest},
			{"review_md", "markdown", filepath.Join(row.dir, "review.md")},
			{"findings", "json", filepath.Join(row.dir, "findings-final.json")},
		} {
			if err := b.register(Record{Module: "review", OwnerType: "review_evidence", OwnerID: ownerID, Phase: "5.5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *backfiller) verification() error {
	rows, err := b.store.DB.Query(`
        SELECT id, phase, command, log_path, status
        FROM verification_records
        WHERE run_id=? AND superseded_at IS NULL
        ORDER BY created_at, id`, b.runID)
	if err != nil {
		return err
	}
	type verificationRow struct {
		id      int64
		phase   string
		command string
		logPath string
		status  string
	}
	var records []verificationRow
	for rows.Next() {
		var row verificationRow
		if err := rows.Scan(&row.id, &row.phase, &row.command, &row.logPath, &row.status); err != nil {
			_ = rows.Close()
			return err
		}
		records = append(records, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range records {
		meta := map[string]any{"command": row.command, "status": row.status}
		if err := b.register(Record{Module: "verification", OwnerType: "verification_record", OwnerID: strconv.FormatInt(row.id, 10), Phase: row.phase, Role: "log", Kind: "log", Path: row.logPath, Metadata: meta}); err != nil {
			return err
		}
	}
	return nil
}

func (b *backfiller) report() error {
	rows, err := b.store.DB.Query(`
        SELECT id, report_path, sha256
        FROM report_evidence
        WHERE run_id=?
        ORDER BY created_at, id`, b.runID)
	if err != nil {
		return err
	}
	type reportRow struct {
		id         int64
		reportPath string
		sum        string
	}
	var reports []reportRow
	for rows.Next() {
		var row reportRow
		if err := rows.Scan(&row.id, &row.reportPath, &row.sum); err != nil {
			_ = rows.Close()
			return err
		}
		reports = append(reports, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range reports {
		meta := map[string]any{"sha256": row.sum}
		ownerID := strconv.FormatInt(row.id, 10)
		if err := b.register(Record{Module: "report", OwnerType: "report_evidence", OwnerID: ownerID, Phase: "7", Role: "report", Kind: "markdown", Path: row.reportPath, Metadata: meta}); err != nil {
			return err
		}
		memoryPath := filepath.Join(b.root, ".codedungeon", "memory", "runs", fmt.Sprintf("run-%d.md", b.runID))
		if err := b.register(Record{Module: "report", OwnerType: "report_evidence", OwnerID: ownerID, Phase: "7", Role: "memory", Kind: "markdown", Path: memoryPath, Metadata: meta}); err != nil {
			return err
		}
	}
	return nil
}

func (b *backfiller) planning() error {
	rows, err := b.store.DB.Query(`
        SELECT id, output_dir, status
        FROM planning_sessions
        WHERE run_id=?
        ORDER BY created_at, id`, b.runID)
	if err != nil {
		return err
	}
	type planningRow struct {
		id        string
		outputDir string
		status    string
	}
	var sessions []planningRow
	for rows.Next() {
		var row planningRow
		if err := rows.Scan(&row.id, &row.outputDir, &row.status); err != nil {
			_ = rows.Close()
			return err
		}
		sessions = append(sessions, row)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, row := range sessions {
		meta := map[string]any{"status": row.status}
		for _, item := range []struct {
			role string
			kind string
			path string
		}{
			{"directory", "directory", row.outputDir},
			{"request", "json", filepath.Join(row.outputDir, "planning-request.json")},
			{"blackboard", "jsonl", filepath.Join(row.outputDir, "blackboard.jsonl")},
			{"evaluation", "json", filepath.Join(row.outputDir, "evaluation.json")},
			{"task_graph", "json", filepath.Join(row.outputDir, "task-graph.json")},
			{"master", "markdown", filepath.Join(row.outputDir, "MASTER.md")},
			{"result", "json", filepath.Join(row.outputDir, "planning-result.json")},
		} {
			if err := b.register(Record{Module: "planning", OwnerType: "planning_session", OwnerID: row.id, Phase: "4", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta}); err != nil {
				return err
			}
		}
		agents, err := b.store.PlanningAgents(row.id)
		if err != nil {
			return err
		}
		for _, agent := range agents {
			if err := b.register(Record{Module: "planning", OwnerType: "planning_agent", OwnerID: strconv.FormatInt(agent.ID, 10), Phase: "4", Role: "agent_output", Kind: "json", Path: agent.OutputPath, Metadata: map[string]any{"role": agent.Role, "status": agent.Status}}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *backfiller) execution() error {
	sessions, err := b.store.ExecutionSessions(b.runID)
	if err != nil {
		return err
	}
	for _, session := range sessions {
		meta := map[string]any{"task_id": session.TaskID, "status": session.Status}
		for _, item := range []struct {
			role string
			kind string
			path string
		}{
			{"directory", "directory", session.OutputDir},
			{"task", "json", filepath.Join(session.OutputDir, "task.json")},
			{"result", "json", filepath.Join(session.OutputDir, "result.json")},
		} {
			if err := b.register(Record{Module: "execution", OwnerType: "execution_session", OwnerID: session.ID, Phase: "5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta}); err != nil {
				return err
			}
		}
		attempts, err := b.store.ExecutionAttempts(session.ID)
		if err != nil {
			return err
		}
		for _, attempt := range attempts {
			ownerID := fmt.Sprintf("%s:%02d", session.ID, attempt.Attempt)
			attemptDir := filepath.Join(session.OutputDir, fmt.Sprintf("attempt-%02d", attempt.Attempt))
			attemptMeta := map[string]any{"session_id": session.ID, "attempt": attempt.Attempt, "worker_status": attempt.WorkerStatus, "verification_status": attempt.VerificationStatus}
			for _, item := range []struct {
				role string
				kind string
				path string
			}{
				{"directory", "directory", attemptDir},
				{"diff", "patch", attempt.DiffPath},
				{"worker_result", "json", filepath.Join(attemptDir, "execution-result.json")},
			} {
				if err := b.register(Record{Module: "execution", OwnerType: "execution_attempt", OwnerID: ownerID, Phase: "5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: attemptMeta}); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (b *backfiller) trace() error {
	agents, err := b.store.AgentRuns(b.runID)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		ownerID := strconv.FormatInt(agent.ID, 10)
		meta := map[string]any{"role": agent.Role, "status": agent.Status, "agent_type": agent.AgentType}
		for _, item := range []struct {
			role string
			path string
		}{
			{"task", agent.TaskPath},
			{"artifact", agent.ArtifactPath},
		} {
			if err := b.register(Record{Module: "trace", OwnerType: "agent_run", OwnerID: ownerID, Phase: agent.Phase, Role: item.role, Kind: kindForPath(item.path), Path: item.path, Metadata: meta}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *backfiller) phasesAndHandoffs() error {
	phases, err := b.store.AllPhases(b.runID)
	if err != nil {
		return err
	}
	for _, phase := range phases {
		for _, path := range phase.Artifacts {
			if err := b.register(Record{Module: "phase", OwnerType: "phase", OwnerID: fmt.Sprintf("%d:%s", b.runID, phase.Phase), Phase: phase.Phase, Role: "artifact", Kind: kindForPath(path), Path: path, Metadata: map[string]any{"status": phase.Status}}); err != nil {
				return err
			}
		}
		handoff, err := b.store.GetHandoff(b.runID, phase.Phase)
		if err != nil {
			return err
		}
		if handoff == nil {
			continue
		}
		for _, path := range handoff.Artifacts {
			if err := b.register(Record{Module: "handoff", OwnerType: "handoff", OwnerID: fmt.Sprintf("%d:%s", b.runID, phase.Phase), Phase: phase.Phase, Role: "artifact", Kind: kindForPath(path), Path: path, Metadata: map[string]any{"summary": handoff.Summary}}); err != nil {
				return err
			}
		}
	}
	return nil
}

func kindForPath(path string) string {
	return KindForPath(path)
}
