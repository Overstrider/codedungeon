package recovery

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestAbortActiveRunSessionClosesSessionAndAgents(t *testing.T) {
	store, runID := newRecoveryTestStore(t)
	defer store.Close()

	if err := store.InsertRunSession(db.RunSession{
		ID:       "run-session-1",
		RunID:    runID,
		Provider: "codex",
		Mode:     "full",
		Status:   "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	runnerID, err := store.StartAgentRun(db.AgentRun{
		RunID:     runID,
		SessionID: "run-session-1",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	})
	if err != nil {
		t.Fatal(err)
	}
	workerID, err := store.StartAgentRun(db.AgentRun{
		RunID:     runID,
		SessionID: "run-session-1",
		Phase:     "5",
		Role:      "reviewer",
		AgentType: "cd_review_spec",
	})
	if err != nil {
		t.Fatal(err)
	}
	otherID, err := store.StartAgentRun(db.AgentRun{
		RunID:     runID,
		SessionID: "other-session",
		Phase:     "6",
		Role:      "verifier",
	})
	if err != nil {
		t.Fatal(err)
	}

	report, err := AbortActiveRunSession(store, runID, "stale lock after host restart", AbortOptions{IncludeAutonomousRunner: true})
	if err != nil {
		t.Fatal(err)
	}

	if report.Status != "ABORTED" || report.SessionID != "run-session-1" {
		t.Fatalf("report = %+v, want ABORTED run-session-1", report)
	}
	if report.AbortedAgents != 2 {
		t.Fatalf("aborted agents = %d, want 2", report.AbortedAgents)
	}
	active, err := store.ActiveRunSession(runID)
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("active session = %+v, want none", active)
	}
	agents, err := store.AgentRuns(runID)
	if err != nil {
		t.Fatal(err)
	}
	statusByID := map[int64]string{}
	for _, agent := range agents {
		statusByID[agent.ID] = agent.Status
	}
	if statusByID[runnerID] != "ABORTED" {
		t.Fatalf("runner status = %q, want ABORTED", statusByID[runnerID])
	}
	if statusByID[workerID] != "ABORTED" {
		t.Fatalf("worker status = %q, want ABORTED", statusByID[workerID])
	}
	if statusByID[otherID] != "RUNNING" {
		t.Fatalf("other session agent status = %q, want RUNNING", statusByID[otherID])
	}
	events, err := store.RunEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Event != "session_aborted" {
		t.Fatalf("run events = %+v, want session_aborted", events)
	}
}

func TestWaitingAgentFirstSessionIsActiveForRecovery(t *testing.T) {
	store, runID := newRecoveryTestStore(t)
	defer store.Close()

	if err := store.InsertRunSession(db.RunSession{
		ID:       "agent-first-waiting",
		RunID:    runID,
		Provider: "codex",
		Mode:     "full",
		Status:   "WAITING_FOR_AGENT",
	}); err != nil {
		t.Fatal(err)
	}

	report, err := InspectRunSession(store, runID, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !report.Active || report.Status != "WAITING_FOR_AGENT" || report.SessionID != "agent-first-waiting" {
		t.Fatalf("report = %+v, want active WAITING_FOR_AGENT", report)
	}
	aborted, err := AbortActiveRunSession(store, runID, "agent-first stale", AbortOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if aborted.Status != "ABORTED" || aborted.SessionID != "agent-first-waiting" {
		t.Fatalf("abort report = %+v, want waiting session aborted", aborted)
	}
}

func TestInspectExecutionSessionReportsExpiredRollbackAndCleanupHints(t *testing.T) {
	store, runID := newRecoveryTestStore(t)
	defer store.Close()
	root := filepath.Dir(filepath.Dir(store.Path))
	outputDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-1")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertExecutionSession(db.ExecutionSession{
		ID:        "exec-1",
		RunID:     runID,
		TaskID:    "TASK-1",
		TaskPath:  "task.json",
		Provider:  "codex",
		Status:    "RUNNING",
		OutputDir: outputDir,
		StartedAt: 1000,
		UpdatedAt: 1000,
		ExpiresAt: 1500,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertExecutionAttempt(db.ExecutionAttempt{
		SessionID:  "exec-1",
		Attempt:    1,
		HeadBefore: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadAfter:  "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		BackupRef:  "refs/codedungeon/backup/exec-1/attempt-01",
	}); err != nil {
		t.Fatal(err)
	}

	report, err := InspectExecutionSession(store, "exec-1", root, time.Unix(2000, 0))
	if err != nil {
		t.Fatal(err)
	}

	if !report.Expired || !report.Stale {
		t.Fatalf("report expired/stale = %v/%v, want true/true", report.Expired, report.Stale)
	}
	if len(report.RollbackHints) == 0 || report.RollbackHints[0].Target != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("rollback hints = %+v, want first target head_before", report.RollbackHints)
	}
	if !strings.Contains(strings.Join(report.NextCommands, "\n"), "--reset-session") {
		t.Fatalf("next commands = %+v, want reset-session hint", report.NextCommands)
	}
	if len(report.CleanupHints) != 1 || report.CleanupHints[0].SafeToDelete {
		t.Fatalf("cleanup hints = %+v, want unsafe while RUNNING", report.CleanupHints)
	}

	plan, err := BuildRollbackPlan(store, "exec-1", "attempt-1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.Target != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" || !plan.RequiresConfirm || !plan.Destructive {
		t.Fatalf("rollback plan = %+v, want destructive confirmed head_before target", plan)
	}
}

func TestExecutionCleanupCandidatesMarkOnlyTerminalSessionOutputsSafe(t *testing.T) {
	store, runID := newRecoveryTestStore(t)
	defer store.Close()
	root := filepath.Dir(filepath.Dir(store.Path))
	completedDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-complete")
	failedDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-failed")
	runningDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-running")
	outsideDir := filepath.Join(root, "outside-session")
	for _, dir := range []string{completedDir, failedDir, runningDir, outsideDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, sess := range []db.ExecutionSession{
		{ID: "exec-complete", RunID: runID, TaskID: "A", TaskPath: "a.json", Provider: "codex", Status: "COMPLETED", OutputDir: completedDir},
		{ID: "exec-failed", RunID: runID, TaskID: "B", TaskPath: "b.json", Provider: "codex", Status: "FAILED", OutputDir: failedDir},
		{ID: "exec-running", RunID: runID, TaskID: "C", TaskPath: "c.json", Provider: "codex", Status: "RUNNING", OutputDir: runningDir},
		{ID: "exec-outside", RunID: runID, TaskID: "D", TaskPath: "d.json", Provider: "codex", Status: "COMPLETED", OutputDir: outsideDir},
	} {
		if err := store.UpsertExecutionSession(sess); err != nil {
			t.Fatal(err)
		}
	}

	candidates, err := ExecutionCleanupCandidates(store, runID, root)
	if err != nil {
		t.Fatal(err)
	}

	safeBySession := map[string]bool{}
	for _, candidate := range candidates {
		safeBySession[candidate.SessionID] = candidate.SafeToDelete
	}
	if !safeBySession["exec-complete"] || !safeBySession["exec-failed"] {
		t.Fatalf("terminal candidates = %+v, want complete/failed safe", candidates)
	}
	if safeBySession["exec-running"] {
		t.Fatalf("running candidate marked safe: %+v", candidates)
	}
	if safeBySession["exec-outside"] {
		t.Fatalf("outside candidate marked safe: %+v", candidates)
	}
}

func newRecoveryTestStore(t *testing.T) (*db.Store, int64) {
	t.Helper()
	root := t.TempDir()
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(); err != nil {
		store.Close()
		t.Fatal(err)
	}
	runID, err := store.CreateRun(&db.Run{Feature: "recovery", Branch: "feature/recovery", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	return store, runID
}
