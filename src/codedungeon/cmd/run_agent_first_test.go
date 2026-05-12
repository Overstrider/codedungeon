package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestRunStartReportsGitHubAsFinalizationBlockerOnly(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# agent first\n")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	run := RunCmd()
	run.SetArgs([]string{"--full", "--prompt", "ship agent first"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = run.Execute()
	})
	if execErr != nil {
		t.Fatalf("run start should not hard-fail on missing GitHub PR readiness: %v\n%s", execErr, out)
	}
	payload := decodeAgentFirstPayload(t, out)
	if payload.Status != "ACTION_REQUIRED" || payload.CurrentStep.ID != "planning" {
		t.Fatalf("payload = %+v, want planning action required", payload)
	}
	if !hasBlocker(payload.Blockers, "github_pr_environment", "finalization") {
		t.Fatalf("github finalization blocker missing: %+v", payload.Blockers)
	}
	if strings.TrimSpace(payload.NextAction.Command) == "" || !strings.Contains(payload.NextAction.Command, "codedungeon plan run") {
		t.Fatalf("next action should guide planning: %+v", payload.NextAction)
	}
}

func TestRunAdvanceRecordsStepAndReturnsNextContract(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "https://github.com/example/repo.git")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(root, "bin")
	writeFile(t, filepath.Join(fakeBin, "gh"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(root, ".codedungeon", "plan", "PLAN.md")
	writeFile(t, planPath, "# Plan\n")

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "ship agent first"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	advance := RunCmd()
	advance.SetArgs([]string{"advance", "--step", "Planning", "--status", "completed", "--summary", "plan promoted", "--artifact", planPath})
	var execErr error
	out := captureStdout(t, func() {
		execErr = advance.Execute()
	})
	if execErr != nil {
		t.Fatalf("advance failed: %v\n%s", execErr, out)
	}
	payload := decodeAgentFirstPayload(t, out)
	if payload.CurrentStep.ID != "execution" {
		t.Fatalf("current step = %+v, want execution", payload.CurrentStep)
	}
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.RunEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !hasRunEvent(events, "step_completed", "planning") {
		t.Fatalf("planning completion event missing: %+v", events)
	}
}

func TestRunAdvanceRejectsPlanningWhileProjectRulesBlock(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "blocked planning"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	advance := RunCmd()
	advance.SetArgs([]string{"advance", "--step", "planning", "--status", "completed", "--summary", "plan should not count"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = advance.Execute()
	})
	if execErr == nil {
		t.Fatalf("advance planning should fail while Project Rules are blocking:\n%s", out)
	}
	if !strings.Contains(execErr.Error(), "current step is project_rules") {
		t.Fatalf("unexpected advance error: %v\n%s", execErr, out)
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.RunEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasRunEvent(events, "step_completed", "planning") {
		t.Fatalf("planning completion event was recorded despite Project Rules blocker: %+v", events)
	}
	phase, err := s.GetPhase(run.ID, "4")
	if err != nil {
		t.Fatal(err)
	}
	if phase != nil && phase.Status == "DONE" {
		t.Fatalf("planning phase was marked DONE despite rejected advance: %+v", phase)
	}
}

func TestRunAdvanceUpdatesFullPhaseLedger(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "phase ledger"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}

	for _, step := range []string{"planning", "execution"} {
		advance := RunCmd()
		advance.SetArgs([]string{"advance", "--step", step, "--status", "completed", "--summary", step + " done"})
		if err := advance.Execute(); err != nil {
			t.Fatalf("advance %s failed: %v", step, err)
		}
	}

	insertAgentFirstReviewEvidence(t, root, s, run.ID)
	codeReview := RunCmd()
	codeReview.SetArgs([]string{"advance", "--step", "code_review", "--status", "completed", "--summary", "code_review done"})
	if err := codeReview.Execute(); err != nil {
		t.Fatalf("advance code_review failed: %v", err)
	}

	insertAgentFirstPostReviewVerification(t, root, s, run.ID)
	qa := RunCmd()
	qa.SetArgs([]string{"advance", "--step", "qa", "--status", "completed", "--summary", "qa done"})
	if err := qa.Execute(); err != nil {
		t.Fatalf("advance qa failed: %v", err)
	}

	for _, phaseName := range []string{"0", "1", "2'", "3.5", "4", "5", "5.5", "5.6", "6"} {
		phase, err := s.GetPhase(run.ID, phaseName)
		if err != nil {
			t.Fatal(err)
		}
		if phase == nil || phase.Status != "DONE" {
			t.Fatalf("phase %s = %+v, want DONE", phaseName, phase)
		}
		handoff, err := s.GetHandoff(run.ID, phaseName)
		if err != nil {
			t.Fatal(err)
		}
		if handoff == nil {
			t.Fatalf("phase %s missing handoff", phaseName)
		}
		if phaseName == "2'" && !strings.Contains(handoff.Promise, "PHASE_2PRIME_COMPLETE") {
			t.Fatalf("phase 2' promise = %q, want PHASE_2PRIME_COMPLETE", handoff.Promise)
		}
	}
}

func TestRunAdvanceDoesNotReportReadyToFinalizeBeforeFinalGates(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "https://github.com/example/repo.git")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	fakeBin := filepath.Join(root, "bin")
	writeFile(t, filepath.Join(fakeBin, "gh"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(fakeBin, "gh"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "ship final gates"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}

	var payload agentFirstTestPayload
	for _, step := range []string{"planning", "execution"} {
		advance := RunCmd()
		advance.SetArgs([]string{"advance", "--step", step, "--status", "completed", "--summary", step + " complete"})
		var execErr error
		out := captureStdout(t, func() {
			execErr = advance.Execute()
		})
		if execErr != nil {
			t.Fatalf("advance %s failed: %v\n%s", step, execErr, out)
		}
		payload = decodeAgentFirstPayload(t, out)
	}
	insertAgentFirstReviewEvidence(t, root, s, run.ID)
	codeReview := RunCmd()
	codeReview.SetArgs([]string{"advance", "--step", "code_review", "--status", "completed", "--summary", "code_review complete"})
	var reviewErr error
	reviewOut := captureStdout(t, func() {
		reviewErr = codeReview.Execute()
	})
	if reviewErr != nil {
		t.Fatalf("advance code_review failed: %v\n%s", reviewErr, reviewOut)
	}
	payload = decodeAgentFirstPayload(t, reviewOut)

	insertAgentFirstPostReviewVerification(t, root, s, run.ID)
	qa := RunCmd()
	qa.SetArgs([]string{"advance", "--step", "qa", "--status", "completed", "--summary", "qa complete"})
	var qaErr error
	qaOut := captureStdout(t, func() {
		qaErr = qa.Execute()
	})
	if qaErr != nil {
		t.Fatalf("advance qa failed: %v\n%s", qaErr, qaOut)
	}
	payload = decodeAgentFirstPayload(t, qaOut)

	if payload.CurrentStep.ID != "finalization" {
		t.Fatalf("current step = %+v, want finalization", payload.CurrentStep)
	}
	if payload.Status == runStatusReadyToFinalize {
		t.Fatalf("status = %s before final gates are satisfied; payload=%+v", payload.Status, payload)
	}
	if !hasBlocker(payload.Blockers, "finalization_preflight", "finalization") {
		t.Fatalf("finalization preflight blocker missing: %+v", payload.Blockers)
	}

	finalize := RunCmd()
	finalize.SetArgs([]string{"finalize", "--dry-run"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = finalize.Execute()
	})
	if execErr != nil {
		t.Fatalf("finalize dry-run should return structured blocker, got %v\n%s", execErr, out)
	}
	var dryRun map[string]any
	if err := unmarshalSetupJSON([]byte(out), &dryRun); err != nil {
		t.Fatalf("unmarshal dry-run: %v\n%s", err, out)
	}
	if dryRun["ok"] != false || strings.TrimSpace(fmt.Sprint(dryRun["blocker"])) == "" {
		t.Fatalf("dry-run = %+v, want blocker", dryRun)
	}
}

func TestRunAdvanceRejectsCodeReviewWithoutEvidence(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "review evidence gate"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"planning", "execution"} {
		advance := RunCmd()
		advance.SetArgs([]string{"advance", "--step", step, "--status", "completed"})
		if err := advance.Execute(); err != nil {
			t.Fatalf("advance %s failed: %v", step, err)
		}
	}

	advance := RunCmd()
	advance.SetArgs([]string{"advance", "--step", "code_review", "--status", "completed"})
	if err := advance.Execute(); err == nil || !strings.Contains(err.Error(), "approved review evidence is required") {
		t.Fatalf("code_review err = %v, want approved review evidence gate", err)
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	events, err := s.RunEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasRunEvent(events, "step_completed", "code_review") {
		t.Fatalf("code_review completion event was recorded despite missing evidence: %+v", events)
	}
	phase, err := s.GetPhase(run.ID, "5.5")
	if err != nil {
		t.Fatal(err)
	}
	if phase != nil && phase.Status == "DONE" {
		t.Fatalf("review phase was marked DONE despite missing evidence: %+v", phase)
	}
}

func TestRunAdvanceRejectsQAWithoutPostReviewVerification(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "qa evidence gate"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	for _, step := range []string{"planning", "execution"} {
		advance := RunCmd()
		advance.SetArgs([]string{"advance", "--step", step, "--status", "completed"})
		if err := advance.Execute(); err != nil {
			t.Fatalf("advance %s failed: %v", step, err)
		}
	}
	insertAgentFirstReviewEvidence(t, root, s, run.ID)
	codeReview := RunCmd()
	codeReview.SetArgs([]string{"advance", "--step", "code_review", "--status", "completed"})
	if err := codeReview.Execute(); err != nil {
		t.Fatalf("advance code_review failed: %v", err)
	}

	qa := RunCmd()
	qa.SetArgs([]string{"advance", "--step", "qa", "--status", "completed"})
	if err := qa.Execute(); err == nil || !strings.Contains(err.Error(), "verification ledger is required") {
		t.Fatalf("qa err = %v, want verification gate", err)
	}
	events, err := s.RunEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if hasRunEvent(events, "step_completed", "qa") {
		t.Fatalf("qa completion event was recorded despite missing verification: %+v", events)
	}
	phase, err := s.GetPhase(run.ID, "6")
	if err != nil {
		t.Fatal(err)
	}
	if phase != nil && phase.Status == "DONE" {
		t.Fatalf("qa phase was marked DONE despite missing verification: %+v", phase)
	}
}

func insertAgentFirstReviewEvidence(t *testing.T, root string, s *db.Store, runID int64) {
	t.Helper()
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "agent-first-review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewResult := writeStandaloneReviewResultFixture(t, reviewDir)
	if _, err := s.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            runID,
		ReviewDir:        reviewDir,
		ReviewJSONPath:   reviewResult.ReviewJSONPath,
		ManifestPath:     filepath.Join(reviewDir, "review-manifest.json"),
		Verdict:          "APPROVED",
		PRNumber:         "123",
		BaseSHA:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PersonasExpected: []string{"saboteur"},
		PersonasRun:      []string{"saboteur"},
	}); err != nil {
		t.Fatal(err)
	}
}

func insertAgentFirstPostReviewVerification(t *testing.T, root string, s *db.Store, runID int64) {
	t.Helper()
	logPath := filepath.Join(root, ".codedungeon", "logs", "go-test.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	verificationID, err := s.InsertVerificationRecord(db.VerificationRecord{
		RunID:   runID,
		Phase:   "6",
		Command: "go test ./...",
		Status:  "PASS",
		LogPath: logPath,
	})
	if err != nil {
		t.Fatal(err)
	}
	markVerificationRecordAfterLatestReview(t, s, runID, verificationID)
}

func TestWaitingAgentFirstSessionBlocksDifferentRunAndPhaseInit(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "owned run"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	samePrompt := RunCmd()
	samePrompt.SetArgs([]string{"--full", "--prompt", "owned run"})
	if err := samePrompt.Execute(); err != nil {
		t.Fatalf("same waiting agent-first run should resume: %v", err)
	}

	differentPrompt := RunCmd()
	differentPrompt.SetArgs([]string{"--full", "--prompt", "different run"})
	if err := differentPrompt.Execute(); err == nil || !strings.Contains(err.Error(), "autonomous session already running") {
		t.Fatalf("different prompt err = %v, want autonomous session guard", err)
	}

	phase := PhaseCmd()
	phase.SetArgs([]string{"init", "--feature", "different phase", "--branch", "feat/different", "--project-mode", "SINGLE"})
	if err := phase.Execute(); err == nil || !strings.Contains(err.Error(), "autonomous session already owns run state") {
		t.Fatalf("phase init err = %v, want autonomous session guard", err)
	}
}

func TestRulesModeCompletionReleasesAgentFirstSession(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	rulesRun := RunCmd()
	rulesRun.SetArgs([]string{"--rules"})
	if err := rulesRun.Execute(); err != nil {
		t.Fatal(err)
	}
	advance := RunCmd()
	advance.SetArgs([]string{"advance", "--step", "project_rules", "--status", "completed", "--summary", "rules compacted"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = advance.Execute()
	})
	if execErr != nil {
		t.Fatalf("advance rules failed: %v\n%s", execErr, out)
	}
	payload := decodeAgentFirstPayload(t, out)
	if payload.Status != runStatusCompleted || payload.CurrentStep.ID != "complete" {
		t.Fatalf("payload = %+v, want completed rules run", payload)
	}

	s := openTestStore(t, root)
	defer s.Close()
	active, err := s.ActiveAnyRunSession()
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("active session = %+v, want rules completion released", active)
	}

	nextRun := RunCmd()
	nextRun.SetArgs([]string{"--full", "--prompt", "after rules"})
	if err := nextRun.Execute(); err != nil {
		t.Fatalf("full run after rules completion should start: %v", err)
	}
}

func TestRunStartDryRunDoesNotLockFutureRuns(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	dryRun := RunCmd()
	dryRun.SetArgs([]string{"--full", "--prompt", "dry probe", "--dry-run"})
	if err := dryRun.Execute(); err != nil {
		t.Fatal(err)
	}
	s := openTestStore(t, root)
	defer s.Close()
	active, err := s.ActiveAnyRunSession()
	if err != nil {
		t.Fatal(err)
	}
	if active != nil {
		t.Fatalf("active session = %+v, want dry-run not to lock future runs", active)
	}
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.Status != "DRY_RUN" {
		t.Fatalf("dry-run latest session = %+v, want DRY_RUN", latest)
	}

	nextRun := RunCmd()
	nextRun.SetArgs([]string{"--full", "--prompt", "real run after dry run"})
	if err := nextRun.Execute(); err != nil {
		t.Fatalf("full run after dry-run should start: %v", err)
	}
}

func TestRulesModeCanAttachToRunWaitingForProjectRules(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "needs rules"})
	var execErr error
	startOut := captureStdout(t, func() {
		execErr = start.Execute()
	})
	if execErr != nil {
		t.Fatalf("start failed: %v\n%s", execErr, startOut)
	}
	startPayload := decodeAgentFirstPayload(t, startOut)
	if startPayload.CurrentStep.ID != "project_rules" {
		t.Fatalf("current step = %+v, want project_rules", startPayload.CurrentStep)
	}

	rulesRun := RunCmd()
	rulesRun.SetArgs([]string{"--rules"})
	var rulesErr error
	rulesOut := captureStdout(t, func() {
		rulesErr = rulesRun.Execute()
	})
	if rulesErr != nil {
		t.Fatalf("rules mode should attach to waiting project_rules run: %v\n%s", rulesErr, rulesOut)
	}
	rulesPayload := decodeAgentFirstPayload(t, rulesOut)
	if rulesPayload.RunID != startPayload.RunID || rulesPayload.CurrentStep.ID != "project_rules" {
		t.Fatalf("rules payload = %+v, want attached run %d at project_rules", rulesPayload, startPayload.RunID)
	}

	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	resume := RunCmd()
	resume.SetArgs([]string{"--full", "--prompt", "needs rules"})
	var resumeErr error
	resumeOut := captureStdout(t, func() {
		resumeErr = resume.Execute()
	})
	if resumeErr != nil {
		t.Fatalf("resume after rules failed: %v\n%s", resumeErr, resumeOut)
	}
	resumePayload := decodeAgentFirstPayload(t, resumeOut)
	if resumePayload.RunID != startPayload.RunID || resumePayload.CurrentStep.ID != "planning" {
		t.Fatalf("resume payload = %+v, want same run planning", resumePayload)
	}
}

func TestRunStatusIncludesTimelineAndNextAction(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "ship timeline"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	status := RunCmd()
	status.SetArgs([]string{"status"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = status.Execute()
	})
	if execErr != nil {
		t.Fatalf("status failed: %v\n%s", execErr, out)
	}
	payload := decodeAgentFirstPayload(t, out)
	if len(payload.Timeline) == 0 {
		t.Fatalf("timeline missing from status: %+v", payload)
	}
	if payload.NextAction.Command == "" {
		t.Fatalf("next action missing from status: %+v", payload)
	}
}

func TestRunUnlockAbortsWaitingAgentFirstSession(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	start := RunCmd()
	start.SetArgs([]string{"--full", "--prompt", "unlock waiting"})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}
	unlock := RunCmd()
	unlock.SetArgs([]string{"unlock", "--reason", "stale agent-first session"})
	if err := unlock.Execute(); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sess, err := s.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil || sess.Status != "ABORTED" || !strings.Contains(sess.FailureMessage, "stale agent-first") {
		t.Fatalf("session not aborted by unlock: %+v", sess)
	}
}

type agentFirstTestPayload struct {
	OK          bool                    `json:"ok"`
	AgentFirst  bool                    `json:"agent_first"`
	Status      string                  `json:"status"`
	RunID       int64                   `json:"run_id"`
	SessionID   string                  `json:"session_id"`
	CurrentStep agentFirstTestStep      `json:"current_step"`
	NextAction  agentFirstTestAction    `json:"next_action"`
	Blockers    []agentFirstTestBlocker `json:"blockers"`
	Timeline    []db.RunEvent           `json:"timeline"`
}

type agentFirstTestStep struct {
	ID string `json:"id"`
}

type agentFirstTestAction struct {
	Command string `json:"command"`
}

type agentFirstTestBlocker struct {
	ID   string `json:"id"`
	Gate string `json:"gate"`
}

func decodeAgentFirstPayload(t *testing.T, out string) agentFirstTestPayload {
	t.Helper()
	var payload agentFirstTestPayload
	if err := unmarshalSetupJSON([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal agent-first payload: %v\n%s", err, out)
	}
	if !payload.OK || !payload.AgentFirst || payload.RunID == 0 || payload.SessionID == "" {
		t.Fatalf("payload missing agent-first identity: %+v\n%s", payload, out)
	}
	return payload
}

func hasBlocker(blockers []agentFirstTestBlocker, id, gate string) bool {
	for _, blocker := range blockers {
		if blocker.ID == id && blocker.Gate == gate {
			return true
		}
	}
	return false
}

func hasRunEvent(events []db.RunEvent, event, detail string) bool {
	for _, e := range events {
		if e.Event == event && strings.Contains(e.Detail, detail) {
			return true
		}
	}
	return false
}
