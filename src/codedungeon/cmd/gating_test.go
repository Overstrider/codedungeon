package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
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

func TestReviewRunApprovesEmptyFindingsWithCompleteManifestEvidence(t *testing.T) {
	root := setupGatedRun(t)
	dir := filepath.Join(root, ".codedungeon", "reviews", "adv-review")
	writeReviewManifest(t, dir, []string{"saboteur", "newhire", "security", "spec"})
	for _, persona := range []string{"saboteur", "newhire", "security", "spec"} {
		writePersonaFindings(t, dir, persona, nil)
	}

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
	if evidence == nil || evidence.Verdict != "APPROVED" || evidence.PRNumber != "123" {
		t.Fatalf("review evidence not persisted: %+v", evidence)
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
	body, err := json.Marshal(map[string]any{
		"persona":  persona,
		"findings": findings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "findings-"+persona+".json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}
