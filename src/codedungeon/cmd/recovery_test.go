package cmd

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestRunUnlockUsesRecoveryAndAbortsSessionAgents(t *testing.T) {
	root := setupGatedRun(t)
	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRunSession(db.RunSession{
		ID:       "run-session-1",
		RunID:    run.ID,
		Provider: "codex",
		Mode:     "full",
		Status:   "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "run-session-1",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "run-session-1",
		Phase:     "5",
		Role:      "reviewer",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	cmd := RunCmd()
	cmd.SetArgs([]string{"unlock", "--reason", "stale lock after interrupted shell"})
	out, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK       bool `json:"ok"`
		Recovery struct {
			Status        string `json:"status"`
			SessionID     string `json:"session_id"`
			AbortedAgents int    `json:"aborted_agents"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if !payload.OK || payload.Recovery.Status != "ABORTED" || payload.Recovery.AbortedAgents != 2 {
		t.Fatalf("payload = %+v, want ABORTED with two aborted agents", payload)
	}

	store = openTestStore(t, root)
	defer store.Close()
	agents, err := store.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range agents {
		if agent.Status != "ABORTED" {
			t.Fatalf("agent not aborted by unlock: %+v", agents)
		}
	}
}

func TestRunFinalizeCreatesRecoveredReadySessionAfterFailedRunner(t *testing.T) {
	root := setupGatedRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareRunFinalizationEvidence(t, root, store, run.ID); err != nil {
		t.Fatal(err)
	}
	writeFakeGitBranch(t, filepath.Join(root, "fake-bin"), run.Branch)
	if err := store.InsertRunSession(db.RunSession{
		ID:          "failed-session",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "full",
		TokenSHA256: hashSessionToken("old-token"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateRunSessionStatus("failed-session", "FAILED", "provider child exited 1"); err != nil {
		t.Fatal(err)
	}
	runnerID, err := store.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "failed-session",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.FinishAgentRun(runnerID, "FAILED", "provider child exited 1", "", "provider child exited 1"); err != nil {
		t.Fatal(err)
	}
	staleWorkerID, err := store.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "failed-session",
		Phase:     "5",
		Role:      "reviewer",
		AgentType: "reviewer",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	finalize := RunCmd()
	finalize.SetArgs([]string{"finalize"})
	out, err := executeCommandInDir(root, finalize)
	if err != nil {
		t.Fatalf("finalize failed: %v\n%s", err, out)
	}

	store = openTestStore(t, root)
	defer store.Close()
	latest, err := store.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.ID == "failed-session" || latest.Status != "READY_FOR_USER_REVIEW" {
		t.Fatalf("latest session = %+v, want recovered READY session after failed-session", latest)
	}
	var failedStillFailed int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM run_sessions WHERE id='failed-session' AND status='FAILED'`).Scan(&failedStillFailed); err != nil {
		t.Fatal(err)
	}
	if failedStillFailed != 1 {
		t.Fatalf("original failed session was not preserved")
	}
	var recoveredEvents, readyEvents int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM run_events WHERE run_id=? AND session_id=? AND event='session_recovered' AND detail='failed-session'`, run.ID, latest.ID).Scan(&recoveredEvents); err != nil {
		t.Fatal(err)
	}
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM run_events WHERE run_id=? AND session_id=? AND event='ready_for_user_review'`, run.ID, latest.ID).Scan(&readyEvents); err != nil {
		t.Fatal(err)
	}
	if recoveredEvents != 1 || readyEvents != 1 {
		t.Fatalf("recovery events = %d ready events = %d, want one each", recoveredEvents, readyEvents)
	}
	evidence, err := store.LatestReportEvidence(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil {
		t.Fatal("report evidence missing after recovered finalize")
	}
	reportBody, err := os.ReadFile(evidence.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reportBody), "OK_RECOVERED") || strings.Contains(string(reportBody), "OK_WITH_RETRIES") {
		t.Fatalf("report telemetry should be recovered, got:\n%s", reportBody)
	}
	if !strings.Contains(string(reportBody), "open=0") {
		t.Fatalf("report should not show stale open agents after recovery:\n%s", reportBody)
	}
	agents, err := store.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	statusByID := map[int64]string{}
	for _, agent := range agents {
		statusByID[agent.ID] = agent.Status
	}
	if statusByID[staleWorkerID] != "ABORTED" {
		t.Fatalf("stale worker from failed session was not aborted: %+v", agents)
	}
	observe, err := RenderObserveReport(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(observe, "open agent runs: 0") {
		t.Fatalf("observe should not show stale open agents after recovery:\n%s", observe)
	}
	if strings.Contains(observe, "terminal non-success statuses") {
		t.Fatalf("observe should not warn on recovered terminal agents:\n%s", observe)
	}
	if !strings.Contains(observe, "Recovered terminal agent statuses") {
		t.Fatalf("observe should explain recovered terminal agents:\n%s", observe)
	}
}

func TestProviderChildFailureAutoFinalizesWhenGatesAlreadyPass(t *testing.T) {
	root := setupGatedRun(t)
	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareRunFinalizationEvidence(t, root, store, run.ID); err != nil {
		t.Fatal(err)
	}
	writeFakeGitBranch(t, filepath.Join(root, "fake-bin"), run.Branch)
	if err := store.InsertRunSession(db.RunSession{
		ID:          "child-failed-session",
		RunID:       run.ID,
		Provider:    "claude",
		Mode:        "full",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	runnerID, err := store.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "child-failed-session",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	})
	if err != nil {
		t.Fatal(err)
	}
	staleWorkerID, err := store.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "child-failed-session",
		Phase:     "5.5",
		Role:      "code-review",
		AgentType: "code-review",
	})
	if err != nil {
		t.Fatal(err)
	}

	report, recovered, err := recoverAfterProviderChildFailure(root, store, run.ID, "child-failed-session", "secret", runnerID, errors.New("claude provider_rate_limit"))
	if err != nil {
		t.Fatal(err)
	}
	if !recovered || !strings.Contains(report, "READY_FOR_USER_REVIEW") {
		t.Fatalf("recovered=%v report:\n%s", recovered, report)
	}
	latest, err := store.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.ID == "child-failed-session" || latest.Status != "READY_FOR_USER_REVIEW" {
		t.Fatalf("latest session = %+v, want recovered READY_FOR_USER_REVIEW session", latest)
	}
	var originalFailed int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM run_sessions WHERE id='child-failed-session' AND status='FAILED'`).Scan(&originalFailed); err != nil {
		t.Fatal(err)
	}
	if originalFailed != 1 {
		t.Fatal("original child session should remain FAILED")
	}
	agents, err := store.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	statusByID := map[int64]string{}
	for _, agent := range agents {
		statusByID[agent.ID] = agent.Status
	}
	if statusByID[runnerID] != "FAILED" || statusByID[staleWorkerID] != "ABORTED" {
		t.Fatalf("agent statuses after recovery = %+v", agents)
	}
}

func TestReportAgentTelemetryLineUsesRecoveredStatusForClosedFailures(t *testing.T) {
	got := reportAgentTelemetryLine([]db.AgentRun{
		{ID: 1, Role: "autonomous-runner", Status: "FAILED"},
		{ID: 2, Role: "qa", Status: "COMPLETED"},
	})
	if !strings.HasPrefix(got, "OK_RECOVERED") || strings.Contains(got, "OK_WITH_RETRIES") {
		t.Fatalf("telemetry line = %q, want recovered non-blocking status", got)
	}
}

func TestRunStatusIncludesFailureKindAndResumeCommandForFailedSession(t *testing.T) {
	root := setupGatedRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.InsertRunSession(db.RunSession{
		ID:             "failed-rate-limit",
		RunID:          run.ID,
		Provider:       "claude",
		Mode:           "full",
		Status:         "FAILED",
		FailureMessage: "claude provider_rate_limit out_of_credits",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	status := RunCmd()
	status.SetArgs([]string{"status"})
	out, err := executeCommandInDir(root, status)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK       bool `json:"ok"`
		Recovery struct {
			Status        string `json:"status"`
			FailureKind   string `json:"failure_kind"`
			BlockedReason string `json:"blocked_reason"`
			ResumeCommand string `json:"resume_command"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if !payload.OK || payload.Recovery.Status != "FAILED" || payload.Recovery.FailureKind != "provider_rate_limit" {
		t.Fatalf("payload = %+v", payload)
	}
	if !strings.Contains(payload.Recovery.BlockedReason, "out_of_credits") || !strings.Contains(payload.Recovery.ResumeCommand, "run finalize") {
		t.Fatalf("recovery hints missing: %+v", payload.Recovery)
	}
}

func writeFakeGitBranch(t *testing.T, dir, branch string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	winBody := strings.Join([]string{
		"@echo off",
		"if \"%1\"==\"branch\" if \"%2\"==\"--show-current\" (echo " + branch + "& exit /b 0)",
		"if \"%1\"==\"rev-parse\" (echo origin/" + branch + "& exit /b 0)",
		"if \"%1\"==\"rev-list\" (echo 0& exit /b 0)",
		"exit /b 1",
		"",
	}, "\r\n")
	if err := os.WriteFile(filepath.Join(dir, "git.cmd"), []byte(winBody), 0o755); err != nil {
		t.Fatal(err)
	}
	shBody := strings.Join([]string{
		"#!/bin/sh",
		`if [ "$1" = "branch" ] && [ "$2" = "--show-current" ]; then printf '%s\n' '` + shellSingleQuote(branch) + `'; exit 0; fi`,
		`if [ "$1" = "rev-parse" ]; then printf '%s\n' 'origin/` + shellSingleQuote(branch) + `'; exit 0; fi`,
		`if [ "$1" = "rev-list" ]; then printf '%s\n' '0'; exit 0; fi`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(shBody), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestExecuteStatusIncludesRecoveryHints(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	outputDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-recover")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertExecutionSession(db.ExecutionSession{
		ID:        "exec-recover",
		RunID:     run.ID,
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
		SessionID:  "exec-recover",
		Attempt:    1,
		HeadBefore: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BackupRef:  "refs/codedungeon/backup/exec-recover/attempt-01",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"status", "--session", "exec-recover"})
	out, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK       bool `json:"ok"`
		Recovery struct {
			Expired       bool     `json:"expired"`
			Stale         bool     `json:"stale"`
			NextCommands  []string `json:"next_commands"`
			RollbackHints []struct {
				Target string `json:"target"`
			} `json:"rollback_hints"`
			CleanupHints []struct {
				SafeToDelete bool `json:"safe_to_delete"`
			} `json:"cleanup_hints"`
		} `json:"recovery"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if !payload.OK || !payload.Recovery.Expired || !payload.Recovery.Stale {
		t.Fatalf("payload = %+v, want expired stale recovery", payload)
	}
	if len(payload.Recovery.RollbackHints) == 0 || payload.Recovery.RollbackHints[0].Target == "" {
		t.Fatalf("rollback hints missing: %+v", payload.Recovery.RollbackHints)
	}
	if len(payload.Recovery.CleanupHints) != 1 || payload.Recovery.CleanupHints[0].SafeToDelete {
		t.Fatalf("cleanup hints = %+v, want unsafe running output", payload.Recovery.CleanupHints)
	}
	if !strings.Contains(strings.Join(payload.Recovery.NextCommands, "\n"), "--reset-session") {
		t.Fatalf("next commands = %+v, want reset-session hint", payload.Recovery.NextCommands)
	}
}

func TestExecuteRollbackReturnsRecoveryPlan(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertExecutionSession(db.ExecutionSession{
		ID:        "exec-rollback",
		RunID:     run.ID,
		TaskID:    "TASK-ROLLBACK",
		TaskPath:  "task.json",
		Provider:  "codex",
		Status:    "FAILED",
		OutputDir: filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-rollback"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertExecutionAttempt(db.ExecutionAttempt{
		SessionID:  "exec-rollback",
		Attempt:    1,
		HeadBefore: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		BackupRef:  "refs/codedungeon/backup/exec-rollback/attempt-01",
	}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"rollback", "--session", "exec-rollback", "--to", "attempt-1", "--confirm"})
	out, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK       bool `json:"ok"`
		Rollback struct {
			Target          string `json:"target"`
			BackupRef       string `json:"backup_ref"`
			RequiresConfirm bool   `json:"requires_confirm"`
			Destructive     bool   `json:"destructive"`
		} `json:"rollback"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if !payload.OK || payload.Rollback.Target == "" || !payload.Rollback.RequiresConfirm || !payload.Rollback.Destructive {
		t.Fatalf("payload = %+v, want typed rollback plan", payload)
	}
}

func TestCleanupSessionsDryRunOnlyReportsSafeTerminalOutputs(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	completedDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-complete")
	runningDir := filepath.Join(root, ".codedungeon", "execute", "sessions", "exec-running")
	for _, dir := range []string{completedDir, runningDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "result.json"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, sess := range []db.ExecutionSession{
		{ID: "exec-complete", RunID: run.ID, TaskID: "A", TaskPath: "a.json", Provider: "codex", Status: "COMPLETED", OutputDir: completedDir},
		{ID: "exec-running", RunID: run.ID, TaskID: "B", TaskPath: "b.json", Provider: "codex", Status: "RUNNING", OutputDir: runningDir},
	} {
		if err := store.UpsertExecutionSession(sess); err != nil {
			t.Fatal(err)
		}
	}
	store.Close()

	cmd := CleanupCmd()
	cmd.SetArgs([]string{"--sessions", "--dry-run"})
	out, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		OK      bool     `json:"ok"`
		Deleted []string `json:"deleted"`
		Skipped []struct {
			SessionID string `json:"session_id"`
		} `json:"skipped"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if !payload.OK || len(payload.Deleted) != 1 || !strings.Contains(payload.Deleted[0], "exec-complete") {
		t.Fatalf("payload = %+v, want one dry-run completed output", payload)
	}
	if len(payload.Skipped) != 1 || payload.Skipped[0].SessionID != "exec-running" {
		t.Fatalf("skipped = %+v, want running session skipped", payload.Skipped)
	}
	if _, err := os.Stat(completedDir); err != nil {
		t.Fatalf("dry-run deleted completed dir: %v", err)
	}
}
