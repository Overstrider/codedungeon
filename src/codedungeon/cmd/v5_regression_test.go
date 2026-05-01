package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestReviewRunNormalizesNumericPRAndPersonaAliases(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"personas_expected": []string{"saboteur", "newhire", "security_auditor", "spec_enforcer"},
		"base_sha":          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"head_sha":          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"pr_number":         123,
		"timestamp":         "2026-04-26T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review-manifest.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	writePersonaFindings(t, dir, "saboteur", []map[string]any{{
		"severity":       "P2",
		"file":           "main.go",
		"line_start":     1,
		"line_end":       1,
		"title":          "documented tradeoff",
		"failure_class":  "contract",
		"evidence_quote": strings.Repeat("substantive evidence for review normalization without empty approval. ", 2),
	}})
	for _, persona := range []string{"newhire", "security_auditor", "spec_enforcer"} {
		writePersonaFindings(t, dir, persona, nil)
	}
	writeJSONFile(t, filepath.Join(dir, "validator-001.json"), map[string]any{
		"id":         "F001",
		"confirmed":  true,
		"confidence": "high",
	})
	writeJSONFile(t, filepath.Join(dir, "classifier-001.json"), map[string]any{
		"id":              "F001",
		"classification":  "design_decision",
		"confidence":      "high",
		"evidence_source": "test",
		"evidence_quote":  strings.Repeat("classified as a non-blocking design decision for normalization coverage. ", 2),
		"rationale":       "keeps the legacy review command on a non-empty path while testing manifest normalization",
	})

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--dir", dir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := s.LatestReviewEvidence(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil || evidence.PRNumber != "123" {
		t.Fatalf("PR number not normalized: %+v", evidence)
	}
	want := strings.Join([]string{"newhire", "saboteur", "security", "spec"}, ",")
	if got := strings.Join(evidence.PersonasExpected, ","); got != want {
		t.Fatalf("expected personas = %q, want %q", got, want)
	}
	if got := strings.Join(evidence.PersonasRun, ","); got != want {
		t.Fatalf("run personas = %q, want %q", got, want)
	}
}

func TestDiscoverTreatsBackendFrontendUnderOneGitRootAsSingleMonorepo(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "https://github.com/acme/gpt-copy-v5.git")
	writeFile(t, filepath.Join(root, "backend", "Cargo.toml"), `[package]
name = "api"
version = "0.1.0"
edition = "2024"

[dependencies]
axum = "0.8"
`)
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"dependencies":{"next":"latest"}}`)

	result, err := discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if result.ProjectMode != "SINGLE" {
		t.Fatalf("project_mode = %s, want SINGLE: %+v", result.ProjectMode, result)
	}
	if len(result.RepoMap) != 1 {
		t.Fatalf("repo_map len = %d, want 1: %+v", len(result.RepoMap), result.RepoMap)
	}
	got := result.RepoMap[0]
	if got.Name != "gpt-copy-v5" || got.Path != "." || got.Lang != "monorepo" {
		t.Fatalf("unexpected monorepo entry: %+v", got)
	}
	if !strings.Contains(got.Stack, "Rust") || !strings.Contains(got.Stack, "Next") {
		t.Fatalf("stack should summarize components: %+v", got)
	}
}

func TestAggregateReposFallsBackToReviewEvidenceWhenRepoMapIsEmpty(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            run.ID,
		ReviewDir:        filepath.Join(root, ".codedungeon", "reviews", "adv-review"),
		ReviewJSONPath:   filepath.Join(root, ".codedungeon", "reviews", "adv-review", "review.json"),
		ManifestPath:     filepath.Join(root, ".codedungeon", "reviews", "adv-review", "review-manifest.json"),
		Verdict:          "APPROVED",
		PRNumber:         "123",
		BaseSHA:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PersonasExpected: []string{"saboteur"},
		PersonasRun:      []string{"saboteur"},
	}); err != nil {
		t.Fatal(err)
	}

	repos := aggregateRepos(s, run)
	if len(repos) != 1 {
		t.Fatalf("repos len = %d, want 1: %+v", len(repos), repos)
	}
	if repos[0].PRNumber != "123" || repos[0].Verdict != "APPROVED" {
		t.Fatalf("review evidence fallback not used: %+v", repos[0])
	}
}

func TestQAFreshSupersedesOldPhaseVerificationRecords(t *testing.T) {
	root := setupGatedRun(t)
	oldLog := filepath.Join(root, ".codedungeon", "logs", "old-fail.log")
	if err := os.MkdirAll(filepath.Dir(oldLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldLog, []byte("old failure"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{
		RunID:   run.ID,
		Phase:   "6",
		Command: "old failing check",
		Status:  "FAIL",
		LogPath: oldLog,
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	qa := QACmd()
	qa.SetArgs([]string{"run", "--phase", "6", "--cmd", "echo ok", "--fresh"})
	if err := qa.Execute(); err != nil {
		t.Fatal(err)
	}

	phase := PhaseCmd()
	phase.SetArgs([]string{"done", "6", "--summary", "verified", "--promise", "PHASE_6_COMPLETE"})
	if err := phase.Execute(); err != nil {
		t.Fatalf("phase 6 gate should ignore superseded old failure: %v", err)
	}
}

func TestQARunReturnsErrorWhenCommandFails(t *testing.T) {
	root := setupGatedRun(t)

	qa := QACmd()
	qa.SetArgs([]string{"run", "--phase", "6", "--cmd", "exit 7", "--cwd", root})
	err := qa.Execute()
	if err == nil {
		t.Fatal("qa run returned nil for a failing command")
	}

	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	records, err := s.VerificationRecords(run.ID, "6")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].Status != "FAIL" {
		t.Fatalf("failed qa command was not recorded as FAIL: %+v", records)
	}
}

func TestRunStartBlocksFullWhenProjectRulesMissingBeforeCreatingRun(t *testing.T) {
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

	run := RunCmd()
	run.SetArgs([]string{"--full", "--prompt", "ship feature", "--dry-run"})
	err = run.Execute()
	if err == nil {
		t.Fatal("full run started without approved Project Rules")
	}
	if !strings.Contains(err.Error(), "project-rules-gate") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".codedungeon", "codedungeon.db")); !os.IsNotExist(statErr) {
		t.Fatalf("run DB should not be created when Project Rules block start: %v", statErr)
	}
}

func TestResolveRunForStartResumesFailedSamePromptMode(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	runID, err := s.CreateRun(&db.Run{Feature: "retry me", Branch: "feat/retry-me", Mode: "ONESHOT", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "failed-session",
		RunID:       runID,
		Provider:    "codex",
		Mode:        "oneshot",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "FAILED",
	}); err != nil {
		t.Fatal(err)
	}

	gotID, branch, resumed, err := resolveRunForStart(s, "retry me", "oneshot", "feat/new")
	if err != nil {
		t.Fatal(err)
	}
	if gotID != runID || !resumed || branch != "feat/retry-me" {
		t.Fatalf("resume = id %d branch %q resumed %v, want id %d original branch true", gotID, branch, resumed, runID)
	}
	var count int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM runs WHERE feature='retry me'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("resume created extra run rows: %d", count)
	}
}

func TestResolveRunForStartBlocksRunningSamePromptMode(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	runID, err := s.CreateRun(&db.Run{Feature: "retry me", Branch: "feat/retry-me", Mode: "ONESHOT", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "running-session",
		RunID:       runID,
		Provider:    "codex",
		Mode:        "oneshot",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}

	_, _, _, err = resolveRunForStart(s, "retry me", "oneshot", "feat/new")
	if err == nil {
		t.Fatal("running same-prompt session did not block")
	}
	if !strings.Contains(err.Error(), "run unlock") {
		t.Fatalf("running session error should mention run unlock: %v", err)
	}
}

func TestOpenDBMigrationRequiredIsTypedAndDoesNotEmitJSON(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	s, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("cd_version", "old-version"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
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
	oldVersion := versionOverride
	versionOverride = "new-version"
	t.Cleanup(func() { versionOverride = oldVersion })

	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	oldStdout := os.Stdout
	os.Stdout = write
	t.Cleanup(func() { os.Stdout = oldStdout })

	cmd := &cobra.Command{Use: "phase"}
	cmd.Flags().String("db", "", "")
	store, openErr := OpenDB(cmd)
	_ = write.Close()
	os.Stdout = oldStdout
	out := make([]byte, 256)
	n, _ := read.Read(out)
	if store != nil {
		_ = store.Close()
	}
	if openErr == nil {
		t.Fatal("OpenDB succeeded despite version drift")
	}
	if !isMigrationRequired(openErr) {
		t.Fatalf("OpenDB error is not migration-required: %T %v", openErr, openErr)
	}
	if strings.TrimSpace(string(out[:n])) != "" {
		t.Fatalf("OpenDB emitted JSON directly: %s", out[:n])
	}
}

func TestRunFinalizeAbortsOpenSessionAgentsOnFailure(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "session-1",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "full",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Phase:     "5",
		Role:      "reviewer",
		AgentType: "cd_review_spec",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Setenv(envSessionID, "session-1")
	t.Setenv(envSessionToken, "secret")

	finalize := RunCmd()
	finalize.SetArgs([]string{"finalize"})
	err = finalize.Execute()
	if err == nil {
		t.Fatal("finalize should fail because report gates are incomplete")
	}

	s = openTestStore(t, root)
	defer s.Close()
	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	statusByRole := map[string]string{}
	for _, agent := range agents {
		statusByRole[agent.Role] = agent.Status
	}
	if statusByRole["reviewer"] != "ABORTED" {
		t.Fatalf("open session agent not aborted after finalize failure: %+v", agents)
	}
	if statusByRole["autonomous-runner"] != "RUNNING" {
		t.Fatalf("runner should stay open for parent failure handling: %+v", agents)
	}
	sess, err := s.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil || sess.Status != "RUNNING" {
		t.Fatalf("failed finalize should not close autonomous custody session: %+v", sess)
	}
}

func TestRunFinalizeDryRunReportsBlockersWithoutMutatingAgents(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "session-1",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "full",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Phase:     "5",
		Role:      "reviewer",
		AgentType: "cd_review_spec",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Setenv(envSessionID, "session-1")
	t.Setenv(envSessionToken, "secret")

	finalize := RunCmd()
	finalize.SetArgs([]string{"finalize", "--dry-run"})
	if err := finalize.Execute(); err != nil {
		t.Fatal(err)
	}

	s = openTestStore(t, root)
	defer s.Close()
	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].Status != "RUNNING" {
		t.Fatalf("dry-run finalized or aborted agents: %+v", agents)
	}
	sess, err := s.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil || sess.Status != "RUNNING" {
		t.Fatalf("dry-run changed custody session: %+v", sess)
	}
}

func TestRunFinalizeDoesNotMarkFinalPhasesWhenFinalGatesFail(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"0", "1", "2'", "3.5", "4"} {
		if err := s.SetPhaseStatus(run.ID, phase, "DONE", "pre-final gate complete", nil); err != nil {
			t.Fatal(err)
		}
	}
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewResult := writeStandaloneReviewResultFixture(t, reviewDir)
	if _, err := s.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            run.ID,
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
	logPath := filepath.Join(root, ".codedungeon", "logs", "go-test.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{
		RunID:   run.ID,
		Phase:   "6",
		Command: "go test ./...",
		Status:  "PASS",
		LogPath: logPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "session-1",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "full",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Setenv(envSessionID, "session-1")
	t.Setenv(envSessionToken, "secret")

	finalize := RunCmd()
	finalize.SetArgs([]string{"finalize"})
	if err := finalize.Execute(); err == nil {
		t.Fatal("finalize should fail before final gates are complete")
	}

	s = openTestStore(t, root)
	defer s.Close()
	for _, phaseName := range []string{"5", "5.5", "5.6", "6", "7"} {
		phase, err := s.GetPhase(run.ID, phaseName)
		if err != nil {
			t.Fatal(err)
		}
		if phase == nil || phase.Status == "DONE" {
			t.Fatalf("phase %s was finalized despite failed final gates: %+v", phaseName, phase)
		}
		handoff, err := s.GetHandoff(run.ID, phaseName)
		if err != nil {
			t.Fatal(err)
		}
		if handoff != nil {
			t.Fatalf("phase %s handoff was written despite failed final gates: %+v", phaseName, handoff)
		}
	}
	evidence, err := s.LatestReportEvidence(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if evidence != nil {
		t.Fatalf("report evidence was written despite failed final gates: %+v", evidence)
	}
}

func TestRunFinalizeSuccessMarksReadyAndCompletesRunner(t *testing.T) {
	root := setupGatedRun(t)
	writeFile(t, filepath.Join(root, "README.md"), "# gated\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")
	runGit(t, root, "checkout", "-b", "feature/gating")
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, root, "init", "--bare", remote)
	runGit(t, root, "remote", "add", "origin", remote)
	runGit(t, root, "push", "-u", "origin", "feature/gating")

	reviewBody := "## CodeDungeon Code Review Final Adjudication approved."
	writeFakeGH(t, filepath.Join(root, "fake-bin"), reviewBody)

	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"0", "1", "2'", "3.5", "4"} {
		if err := s.SetPhaseStatus(run.ID, phase, "DONE", "pre-final gate complete", nil); err != nil {
			t.Fatal(err)
		}
	}
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewResult := writeStandaloneReviewResultFixture(t, reviewDir)
	reviewID, err := s.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            run.ID,
		ReviewDir:        reviewDir,
		ReviewJSONPath:   reviewResult.ReviewJSONPath,
		ManifestPath:     filepath.Join(reviewDir, "review-manifest.json"),
		Verdict:          "APPROVED",
		PRNumber:         "123",
		BaseSHA:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PersonasExpected: []string{"saboteur"},
		PersonasRun:      []string{"saboteur"},
	})
	if err != nil {
		t.Fatal(err)
	}
	bodyHash := sha256.Sum256([]byte(strings.TrimSpace(reviewBody)))
	if _, err := s.InsertPRReviewPost(db.PRReviewPost{
		RunID:            run.ID,
		ReviewEvidenceID: reviewID,
		PRNumber:         "123",
		CommentID:        "456",
		CommentURL:       "https://github.com/acme/example/pull/123#issuecomment-456",
		BodySHA256:       hex.EncodeToString(bodyHash[:]),
	}); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, ".codedungeon", "logs", "go-test.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{
		RunID:   run.ID,
		Phase:   "6",
		Command: "go test ./...",
		Status:  "PASS",
		LogPath: logPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "session-1",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "full",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	runnerID, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	})
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Setenv(envSessionID, "session-1")
	t.Setenv(envSessionToken, "secret")

	finalize := RunCmd()
	finalize.SetArgs([]string{"finalize"})
	if err := finalize.Execute(); err != nil {
		t.Fatal(err)
	}

	s = openTestStore(t, root)
	defer s.Close()
	for _, phaseName := range []string{"5", "5.5", "5.6", "6", "7"} {
		phase, err := s.GetPhase(run.ID, phaseName)
		if err != nil {
			t.Fatal(err)
		}
		if phase == nil || phase.Status != "DONE" {
			t.Fatalf("phase %s was not finalized: %+v", phaseName, phase)
		}
	}
	evidence, err := s.LatestReportEvidence(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil {
		t.Fatal("report evidence was not recorded")
	}
	sess, err := s.LatestRunSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if sess == nil || sess.Status != "READY_FOR_USER_REVIEW" {
		t.Fatalf("session not marked ready: %+v", sess)
	}
	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	statusByID := map[int64]string{}
	for _, agent := range agents {
		statusByID[agent.ID] = agent.Status
	}
	if statusByID[runnerID] != "COMPLETED" {
		t.Fatalf("runner not completed: %+v", agents)
	}
	var readyEvents int
	if err := s.DB.QueryRow(`SELECT COUNT(1) FROM run_events WHERE run_id=? AND session_id=? AND event='ready_for_user_review'`, run.ID, "session-1").Scan(&readyEvents); err != nil {
		t.Fatal(err)
	}
	if readyEvents != 1 {
		t.Fatalf("ready event count = %d, want 1", readyEvents)
	}
	reportBody, err := os.ReadFile(evidence.ReportPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(reportBody), "open=0") {
		t.Fatalf("final report should not show open agents:\n%s", reportBody)
	}
}

func TestCompleteOpenRunnerAgentsCompletesRunnerAndRecordsEvent(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	runnerID, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := completeOpenRunnerAgents(s, run.ID, "session-1", 0, "final report rendered"); err != nil {
		t.Fatal(err)
	}

	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 || agents[0].ID != runnerID || agents[0].Status != "COMPLETED" {
		t.Fatalf("runner was not completed: %+v", agents)
	}
	events, err := s.AgentEvents(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, event := range events {
		if event.AgentRunID == runnerID && event.Event == "agent_completed" {
			found = true
		}
	}
	if !found {
		t.Fatalf("runner completion event not recorded: %+v", events)
	}
}

func TestVirtualFinalizationAgentsCompletesExcludedRunnerInReportSnapshot(t *testing.T) {
	agents := []db.AgentRun{
		{ID: 10, SessionID: "session-1", Role: "autonomous-runner", Status: "RUNNING"},
		{ID: 11, SessionID: "session-1", Role: "reviewer", Status: "RUNNING"},
	}

	virtual := virtualFinalizationAgents(agents, "session-1", 10)

	statusByID := map[int64]string{}
	for _, agent := range virtual {
		statusByID[agent.ID] = agent.Status
	}
	if statusByID[10] != "COMPLETED" {
		t.Fatalf("excluded runner should be completed in report snapshot: %+v", virtual)
	}
	if statusByID[11] != "ABORTED" {
		t.Fatalf("open non-runner should be aborted in report snapshot: %+v", virtual)
	}
	if got := reportAgentTelemetryLine(virtual); !strings.Contains(got, "open=0") {
		t.Fatalf("virtual telemetry should not report open agents: %s", got)
	}
}

func writeFakeGH(t *testing.T, dir, reviewBody string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	winBody := strings.ReplaceAll(reviewBody, "\n", " ")
	cmdBody := strings.Join([]string{
		"@echo off",
		"if \"%1\"==\"pr\" if \"%2\"==\"list\" (echo 123& exit /b 0)",
		"if \"%1\"==\"pr\" if \"%2\"==\"view\" (echo OPEN& exit /b 0)",
		"if \"%1\"==\"repo\" if \"%2\"==\"view\" (echo acme/example& exit /b 0)",
		"if \"%1\"==\"api\" (echo " + winBody + "& exit /b 0)",
		"exit /b 1",
		"",
	}, "\r\n")
	if err := os.WriteFile(filepath.Join(dir, "gh.cmd"), []byte(cmdBody), 0o755); err != nil {
		t.Fatal(err)
	}
	shBody := strings.Join([]string{
		"#!/bin/sh",
		`if [ "$1" = "pr" ] && [ "$2" = "list" ]; then echo 123; exit 0; fi`,
		`if [ "$1" = "pr" ] && [ "$2" = "view" ]; then echo OPEN; exit 0; fi`,
		`if [ "$1" = "repo" ] && [ "$2" = "view" ]; then echo acme/example; exit 0; fi`,
		`if [ "$1" = "api" ]; then printf '%s\n' '` + shellSingleQuote(reviewBody) + `'; exit 0; fi`,
		"exit 1",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(shBody), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func shellSingleQuote(s string) string {
	return strings.ReplaceAll(s, `'`, `'\''`)
}

func TestAbortOpenAgentRunsKeepsAutonomousRunnerOpen(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Role:      "autonomous-runner",
		AgentType: "codedungeon-runner",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		SessionID: "session-1",
		Phase:     "5",
		Role:      "reviewer",
		AgentType: "cd_review_spec",
	}); err != nil {
		t.Fatal(err)
	}

	if err := abortOpenAgentRuns(s, run.ID, "session-1", 0, "finalizing", "done"); err != nil {
		t.Fatal(err)
	}

	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	statusByRole := map[string]string{}
	for _, agent := range agents {
		statusByRole[agent.Role] = agent.Status
	}
	if statusByRole["autonomous-runner"] != "RUNNING" {
		t.Fatalf("autonomous runner should stay open until runner end: %+v", agents)
	}
	if statusByRole["reviewer"] != "ABORTED" {
		t.Fatalf("non-runner open agent should be aborted: %+v", agents)
	}
}

func TestMarkFinalReportPhaseDoneRecordsPhaseSevenHandoff(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(root, ".codedungeon", "reports", "run-1.md")

	if err := markFinalReportPhaseDone(s, run.ID, reportPath); err != nil {
		t.Fatal(err)
	}

	phase, err := s.GetPhase(run.ID, "7")
	if err != nil {
		t.Fatal(err)
	}
	if phase == nil || phase.Status != "DONE" {
		t.Fatalf("phase 7 not marked done: %+v", phase)
	}
	handoff, err := s.GetHandoff(run.ID, "7")
	if err != nil {
		t.Fatal(err)
	}
	if handoff == nil || !strings.Contains(handoff.Promise, "PHASE_7_COMPLETE") {
		t.Fatalf("phase 7 handoff missing completion promise: %+v", handoff)
	}
	if len(handoff.Artifacts) != 1 || handoff.Artifacts[0] != reportPath {
		t.Fatalf("phase 7 handoff artifacts = %+v, want %s", handoff.Artifacts, reportPath)
	}
}
