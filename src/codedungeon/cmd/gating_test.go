package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

func TestReviewRunRejectsEmptyFindingsDirectory(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--dir", dir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("review run succeeded with an empty findings directory")
	}
	if !strings.Contains(err.Error(), "review-manifest") && !strings.Contains(err.Error(), "persona") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewRunRequiresAllManifestPersonas(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur", "newhire", "security", "spec"})
	writePersonaFindings(t, dir, "saboteur", nil)
	writePersonaFindings(t, dir, "newhire", nil)
	writePersonaFindings(t, dir, "security", nil)

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--dir", dir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("review run succeeded with a manifest persona missing")
	}
	if !strings.Contains(err.Error(), "missing persona output") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewRunRejectsEmptyFindingsWithoutStandaloneAdjudication(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur", "newhire", "security", "spec"})
	for _, persona := range []string{"saboteur", "newhire", "security", "spec"} {
		writePersonaFindings(t, dir, persona, nil)
	}

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--dir", dir})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("legacy review run approved empty findings without standalone adjudication")
	}
	if !strings.Contains(err.Error(), "standalone code-review") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewRunRequiresValidatorAndClassifierEvidenceForFinalRun(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur"})
	writePersonaFindings(t, dir, "saboteur", []map[string]any{{
		"severity":       "P1",
		"file":           "main.go",
		"line_start":     10,
		"line_end":       10,
		"title":          "missing cleanup",
		"failure_class":  "cleanup",
		"evidence_quote": "return err",
	}})

	partial := ReviewCmd()
	partial.SetArgs([]string{"run", "--only", "dedupe", "--dir", dir})
	if err := partial.Execute(); err != nil {
		t.Fatalf("partial dedupe should not require validator/classifier evidence: %v", err)
	}

	final := ReviewCmd()
	final.SetArgs([]string{"run", "--dir", dir})
	err := final.Execute()
	if err == nil {
		t.Fatal("final review run succeeded without validator evidence")
	}
	if !strings.Contains(err.Error(), "missing validator evidence") {
		t.Fatalf("unexpected validator error: %v", err)
	}

	writeJSONFile(t, filepath.Join(dir, "validator-001.json"), map[string]any{
		"id":         "F001",
		"confirmed":  true,
		"confidence": "high",
	})
	final = ReviewCmd()
	final.SetArgs([]string{"run", "--dir", dir})
	err = final.Execute()
	if err == nil {
		t.Fatal("final review run succeeded without classifier evidence")
	}
	if !strings.Contains(err.Error(), "missing classifier evidence") {
		t.Fatalf("unexpected classifier error: %v", err)
	}
}

func TestReviewRunRequiresStackEvidenceWhenStackSpecialistDeclared(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur"})
	writePersonaFindings(t, dir, "saboteur", []map[string]any{{
		"severity":       "P1",
		"file":           "main.go",
		"line_start":     1,
		"line_end":       1,
		"title":          "stack specialist candidate",
		"failure_class":  "stack",
		"evidence_quote": strings.Repeat("substantive evidence keeps this test on the stack-specialist gate path. ", 2),
	}})
	writeJSONFile(t, filepath.Join(dir, "validator-001.json"), map[string]any{
		"id":         "F001",
		"confirmed":  true,
		"confidence": "high",
	})
	writeJSONFile(t, filepath.Join(dir, "classifier-001.json"), map[string]any{
		"id":             "F001",
		"classification": "actionable",
		"confidence":     "high",
	})

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--dir", dir, "--stack-specialist", "go-specialist"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("final review run succeeded without stack-specialist evidence")
	}
	if !strings.Contains(err.Error(), "missing stack-specialist evidence") {
		t.Fatalf("unexpected stack evidence error: %v", err)
	}
}

func TestReviewRunRejectsEmptyPersonaWithoutRationale(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur"})
	body, err := json.Marshal(map[string]any{
		"persona":  "saboteur",
		"findings": []map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "findings-saboteur.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--dir", dir})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("review run accepted zero findings without rationale")
	}
	if !strings.Contains(err.Error(), "no_findings_rationale") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewRunChecksCustodyBeforeWritingArtifacts(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur"})
	writePersonaFindings(t, dir, "saboteur", []map[string]any{{
		"severity":       "P1",
		"file":           "main.go",
		"line_start":     1,
		"line_end":       1,
		"title":          "test finding",
		"evidence_quote": "package main",
	}})
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "session-1",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "oneshot",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"run", "--only", "dedupe", "--dir", dir})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("review run succeeded without autonomous session token")
	}
	if !strings.Contains(err.Error(), "autonomous-session-required") {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "findings-merged.json")); !os.IsNotExist(statErr) {
		t.Fatalf("review run wrote findings-merged.json before custody check: %v", statErr)
	}
}

func TestReviewPostRejectsDirMismatchBeforeGitHub(t *testing.T) {
	root := setupGatedRun(t)
	evidenceDir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	otherDir := filepath.Join(root, ".codedungeon", "reviews", "other")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result := writeStandaloneReviewResultFixture(t, evidenceDir)
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertReviewEvidence(db.ReviewEvidence{
		RunID:            run.ID,
		ReviewDir:        evidenceDir,
		ReviewJSONPath:   result.ReviewJSONPath,
		ManifestPath:     filepath.Join(evidenceDir, "review-manifest.json"),
		Verdict:          "APPROVED",
		PRNumber:         "123",
		BaseSHA:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PersonasExpected: []string{"saboteur"},
		PersonasRun:      []string{"saboteur"},
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := ReviewCmd()
	cmd.SetArgs([]string{"post", "--dir", otherDir})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("review post accepted a directory different from latest review evidence")
	}
	if !strings.Contains(err.Error(), "--dir does not match latest review evidence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewEvidenceRejectsSkippedValidatorClassifierMetadata(t *testing.T) {
	root := setupGatedRun(t)
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(reviewDir, "review.md"), []byte("## "+provider.Detect().ReviewCommentMarker()+"\n\nApproved."), 0o644); err != nil {
		t.Fatal(err)
	}
	reviewJSONPath := filepath.Join(reviewDir, "review.json")
	if err := os.WriteFile(reviewJSONPath, []byte(`{"verdict":"APPROVED","findings":[],"personas_run":["saboteur"],"validator_model":"SKIPPED","classifier_model":"SKIPPED"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	err := validateReviewEvidence(&db.ReviewEvidence{
		ReviewDir:        reviewDir,
		ReviewJSONPath:   reviewJSONPath,
		Verdict:          "APPROVED",
		PRNumber:         "123",
		BaseSHA:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PersonasExpected: []string{"saboteur"},
		PersonasRun:      []string{"saboteur"},
	})
	if err == nil {
		t.Fatal("review evidence accepted SKIPPED validator/classifier metadata")
	}
	if !strings.Contains(err.Error(), "review-result.json") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReviewEvidenceRejectsMismatchedProviderMarkdownMarker(t *testing.T) {
	root := setupGatedRun(t)
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	result := writeStandaloneReviewResultFixture(t, reviewDir)
	wrongMarker := provider.Detect().ReviewCommentMarker()
	if err := os.WriteFile(filepath.Join(reviewDir, "review.md"), []byte("## "+wrongMarker+"\n\nApproved."), 0o644); err != nil {
		t.Fatal(err)
	}

	err := validateReviewEvidence(&db.ReviewEvidence{
		ReviewDir:        reviewDir,
		ReviewJSONPath:   result.ReviewJSONPath,
		Verdict:          "APPROVED",
		PRNumber:         "123",
		BaseSHA:          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		HeadSHA:          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		PersonasExpected: []string{"saboteur"},
		PersonasRun:      []string{"saboteur"},
	})
	if err == nil {
		t.Fatal("review evidence accepted a review.md marker from another provider")
	}
	if !strings.Contains(err.Error(), "standalone review section") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutonomousSessionBlocksPhaseMutationWithoutToken(t *testing.T) {
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
		Mode:        "oneshot",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"done", "0", "--summary", "blocked", "--promise", "PHASE_0_COMPLETE"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("phase mutation succeeded without autonomous session token")
	}
	if !strings.Contains(err.Error(), "autonomous-session-required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAutonomousSessionAllowsPhaseMutationWithToken(t *testing.T) {
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
		Mode:        "oneshot",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()
	t.Setenv(envSessionID, "session-1")
	t.Setenv(envSessionToken, "secret")

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"done", "0", "--summary", "ok", "--promise", "PHASE_0_COMPLETE"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("phase mutation rejected valid autonomous token: %v", err)
	}
}

func TestPhaseInitBlockedDuringActiveAutonomousSession(t *testing.T) {
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
	s.Close()

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"init", "--feature", "other", "--branch", "feat/other", "--project-mode", "SINGLE"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("phase init created a second run during an active autonomous session")
	}
	if !strings.Contains(err.Error(), "autonomous session already owns run state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQARecordDisabledDuringAutonomousSession(t *testing.T) {
	root := setupGatedRun(t)
	logPath := filepath.Join(root, ".codedungeon", "logs", "pass.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          "session-1",
		RunID:       run.ID,
		Provider:    "codex",
		Mode:        "oneshot",
		TokenSHA256: hashSessionToken("secret"),
		Status:      "RUNNING",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := QACmd()
	cmd.SetArgs([]string{"record", "--phase", "6", "--cmd", "go test ./...", "--status", "PASS", "--log", logPath})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("qa record succeeded during autonomous session")
	}
	if !strings.Contains(err.Error(), "qa record disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPhaseDoneFiveRejectsApprovedVerdictWithoutReviewEvidence(t *testing.T) {
	setupGatedRun(t)

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"done", "5", "--summary", "approved", "--promise", "PHASE_5_COMPLETE", "--verdict", "APPROVED"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("phase done 5 accepted APPROVED without review evidence")
	}
	if !strings.Contains(err.Error(), "review evidence") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPhaseDoneSevenRejectsMissingVerificationAndRenderedReport(t *testing.T) {
	setupGatedRun(t)

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"done", "7", "--summary", "final", "--promise", "COMPLETE", "--verdict", "APPROVED"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("phase done 7 accepted missing verification/report evidence")
	}
	if !strings.Contains(err.Error(), "phase-7") && !strings.Contains(err.Error(), "report") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestQARecordPersistsVerificationLedger(t *testing.T) {
	root := setupGatedRun(t)
	logPath := filepath.Join(root, ".codedungeon", "logs", "go-test.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := QACmd()
	cmd.SetArgs([]string{"record", "--phase", "6", "--cmd", "go test ./...", "--status", "PASS", "--log", logPath})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
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
	if len(records) != 1 || records[0].Status != "PASS" || records[0].Command != "go test ./..." {
		t.Fatalf("unexpected records: %+v", records)
	}
}

func TestPhaseSixGateUsesLatestVerificationRecordPerCommand(t *testing.T) {
	root := setupGatedRun(t)
	failLog := filepath.Join(root, ".codedungeon", "logs", "fail.log")
	passLog := filepath.Join(root, ".codedungeon", "logs", "pass.log")
	if err := os.MkdirAll(filepath.Dir(failLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failLog, []byte("old failure"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passLog, []byte("later pass"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{RunID: run.ID, Phase: "6", Command: "go test ./...", Status: "FAIL", LogPath: failLog}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{RunID: run.ID, Phase: "6", Command: "go test ./...", Status: "PASS", LogPath: passLog}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"done", "6", "--summary", "verified", "--promise", "PHASE_6_COMPLETE"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("phase done 6 rejected later PASS over old FAIL: %v", err)
	}
}

func TestPhaseSixGateRejectsLatestFailurePerCommand(t *testing.T) {
	root := setupGatedRun(t)
	passLog := filepath.Join(root, ".codedungeon", "logs", "pass.log")
	failLog := filepath.Join(root, ".codedungeon", "logs", "fail.log")
	if err := os.MkdirAll(filepath.Dir(passLog), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(passLog, []byte("old pass"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failLog, []byte("later failure"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{RunID: run.ID, Phase: "6", Command: "go test ./...", Status: "PASS", LogPath: passLog}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertVerificationRecord(db.VerificationRecord{RunID: run.ID, Phase: "6", Command: "go test ./...", Status: "FAIL", LogPath: failLog}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	cmd := PhaseCmd()
	cmd.SetArgs([]string{"done", "6", "--summary", "verified", "--promise", "PHASE_6_COMPLETE"})
	err = cmd.Execute()
	if err == nil {
		t.Fatal("phase done 6 accepted latest FAIL")
	}
	if !strings.Contains(err.Error(), "verification command failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestReportRenderFailsBeforeCompletionGates(t *testing.T) {
	setupGatedRun(t)

	cmd := ReportCmd()
	cmd.SetArgs([]string{"render"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("report render succeeded before gates were complete")
	}
	if !strings.Contains(err.Error(), "report-gate") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFinalizeRunInvokesWorkflowQAWhenVerificationMissing(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/workflowqa\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(root, "smoke_test.go"), "package workflowqa\n\nimport \"testing\"\n\nfunc TestSmoke(t *testing.T) {}\n")
	writeFile(t, filepath.Join(root, ".codedungeon", "project-context.md"), "# Project Context\n\n- Demo project.\n")
	runGit(t, root, "add", ".")
	runGit(t, root, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")
	runGit(t, root, "checkout", "-b", "feature/gating")
	runGit(t, root, "config", "branch.feature/gating.remote", ".")
	runGit(t, root, "config", "branch.feature/gating.merge", "refs/heads/feature/gating")

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	reviewBody := "CodeDungeon Code Review Verdict APPROVED Review Integrity PASS Findings Structured test review was generated by the standalone review module. Review Summary Final adjudication approved this fixture."
	writeFakeGH(t, filepath.Join(root, "fake-bin"), reviewBody)
	s := openTestStore(t, root)
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	runID, err := s.CreateRun(&db.Run{Feature: "workflow qa", Branch: "feature/gating", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"0", "1", "2'", "3.5", "4"} {
		if err := s.SetPhaseStatus(runID, phase, "DONE", "pre-final gate complete", nil); err != nil {
			t.Fatal(err)
		}
	}
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reviewResult := writeStandaloneReviewResultFixture(t, reviewDir)
	reviewID, err := s.InsertReviewEvidence(db.ReviewEvidence{
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
	})
	if err != nil {
		t.Fatal(err)
	}
	bodyHash := sha256.Sum256([]byte(strings.TrimSpace(reviewBody)))
	if _, err := s.InsertPRReviewPost(db.PRReviewPost{
		RunID:            runID,
		ReviewEvidenceID: reviewID,
		PRNumber:         "123",
		CommentID:        "456",
		CommentURL:       "https://github.com/acme/example/pull/123#issuecomment-456",
		BodySHA256:       hex.EncodeToString(bodyHash[:]),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := finalizeRun(root, s, &db.Run{ID: runID, Feature: "workflow qa", Branch: "feature/gating", Mode: "FULL"}, "session-1", "", 0); err != nil {
		t.Fatalf("finalizeRun should invoke QA automatically: %v", err)
	}
	qaSession, err := s.LatestQASession(runID)
	if err != nil {
		t.Fatal(err)
	}
	if qaSession == nil || qaSession.Status != "PASS" || qaSession.Entrypoint != "workflow" {
		t.Fatalf("qa session = %+v, want workflow PASS", qaSession)
	}
	records, err := s.VerificationRecords(runID, "6")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) == 0 || records[len(records)-1].Status != "PASS" {
		t.Fatalf("verification records = %+v, want PASS generated by QA", records)
	}
}

func TestCompactRunModesSkipPreReportPhaseLedger(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	id, err := s.CreateRun(&db.Run{Feature: "small fix", Branch: "feat/small-fix", Mode: "ONESHOT", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareRunForMode(s, id, "oneshot"); err != nil {
		t.Fatal(err)
	}
	phases, err := s.AllPhases(id)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, phase := range phases {
		if phase.Phase == "7" {
			continue
		}
		if phase.Status != "SKIPPED" {
			t.Fatalf("phase %s status = %s, want SKIPPED", phase.Phase, phase.Status)
		}
	}
	if root == "" {
		t.Fatal("setup did not return root")
	}
}

func TestProjectRulesHookCoversMergeBypassPatterns(t *testing.T) {
	script := projectRulesHookScript(".codex/bin/codedungeon", "enforce")
	for _, required := range []string{
		"HEAD:main",
		"/pulls/",
		"refs/heads/main",
		"codedungeon(\\.exe)?\\s+review\\s+(run|post)",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("hook script missing %q:\n%s", required, script)
		}
	}
}

func setupGatedRun(t *testing.T) string {
	t.Helper()
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
	cmd := PhaseCmd()
	cmd.SetArgs([]string{"init", "--feature", "gating", "--branch", "feature/gating", "--project-mode", "SINGLE"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	return root
}

func openTestStore(t *testing.T, root string) *db.Store {
	t.Helper()
	s, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func writeReviewManifest(t *testing.T, dir string, personas []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(map[string]any{
		"personas_expected": personas,
		"base_sha":          "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		"head_sha":          "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		"pr_number":         "123",
		"timestamp":         "2026-04-26T00:00:00Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "review-manifest.json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePersonaFindings(t *testing.T, dir, persona string, findings []map[string]any) {
	t.Helper()
	payload := map[string]any{
		"persona":  persona,
		"findings": findings,
	}
	if len(findings) == 0 {
		payload["reviewed_files"] = 1
		payload["no_findings_rationale"] = "persona reviewed the diff and found no actionable issues"
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "findings-"+persona+".json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeJSONFile(t *testing.T, path string, payload any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
