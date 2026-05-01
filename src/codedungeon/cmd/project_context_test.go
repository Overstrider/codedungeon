package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestProjectContextCLIInitApproveEnvelopeAndRulesCompatibility(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")

	initCmd := ProjectContextCmd()
	initCmd.SetArgs([]string{"init", "--mode", "auto", "--first-prompt", "Build a CLI"})
	if err := runCommandInDir(root, initCmd); err != nil {
		t.Fatalf("project-context init failed: %v", err)
	}
	proposalBody, err := os.ReadFile(filepath.Join(root, ".codedungeon", "project-context.proposal.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(proposalBody), "Project Context") {
		t.Fatalf("proposal missing context header:\n%s", string(proposalBody))
	}

	rejectCmd := ProjectContextCmd()
	rejectCmd.SetArgs([]string{"reject", "--proposal", "1"})
	if err := runCommandInDir(root, rejectCmd); err != nil {
		t.Fatalf("project-context reject failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "project-context.proposal.md")); !os.IsNotExist(err) {
		t.Fatalf("rejected proposal file should be removed, stat err=%v", err)
	}

	initCmd = ProjectContextCmd()
	initCmd.SetArgs([]string{"init", "--mode", "auto", "--first-prompt", "Build a CLI"})
	if err := runCommandInDir(root, initCmd); err != nil {
		t.Fatalf("second project-context init failed: %v", err)
	}

	approveCmd := ProjectContextCmd()
	approveCmd.SetArgs([]string{"approve", "--proposal", "2", "--by", "tester"})
	if err := runCommandInDir(root, approveCmd); err != nil {
		t.Fatalf("project-context approve failed: %v", err)
	}
	contextBody, err := os.ReadFile(filepath.Join(root, ".codedungeon", "project-context.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(contextBody), "- Project") {
		t.Fatalf("approved context should contain compact bullets:\n%s", string(contextBody))
	}

	envCmd := ProjectContextCmd()
	envCmd.SetArgs([]string{"envelope"})
	out, err := executeCommandInDir(root, envCmd)
	if err != nil {
		t.Fatalf("project-context envelope failed: %v\n%s", err, out)
	}
	var env map[string]any
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatal(err)
	}
	if env["PROJECT_RULES_READ"] != "yes" || env["PROJECT_RULES_STATUS"] == "" {
		t.Fatalf("unexpected envelope: %+v", env)
	}
}

func TestFinalizeCreatesPendingProjectContextProposal(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.email=test@example.com", "-c", "user.name=Test", "commit", "-m", "init")
	runGit(t, root, "checkout", "-b", "feat/export-command")
	remote := filepath.Join(t.TempDir(), "origin.git")
	runGit(t, root, "init", "--bare", remote)
	runGit(t, root, "remote", "add", "origin", remote)
	runGit(t, root, "push", "-u", "origin", "feat/export-command")
	writeFile(t, filepath.Join(root, ".codedungeon", "project-context.md"), "# Project Context\n\n- Project is a demo.\n")
	store := openTestStore(t, root)
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	runID, err := store.CreateRun(&db.Run{
		Feature:     "Add export command",
		Branch:      "feat/export-command",
		Mode:        "FULL",
		ProjectMode: "SINGLE",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := prepareRunFinalizationEvidence(t, root, store, runID); err != nil {
		t.Fatal(err)
	}

	if _, err := finalizeRun(root, store, &db.Run{ID: runID, Feature: "Add export command", Branch: "feat/export-command", Mode: "FULL"}, "session-1", "", 0); err != nil {
		t.Fatalf("finalizeRun failed: %v", err)
	}
	proposalBody, err := os.ReadFile(filepath.Join(root, ".codedungeon", "project-context.proposal.md"))
	if err != nil {
		t.Fatalf("project context proposal missing after finalize: %v", err)
	}
	if !strings.Contains(string(proposalBody), "Add export command") {
		t.Fatalf("proposal missing run feature:\n%s", string(proposalBody))
	}
	statusCmd := ProjectContextCmd()
	statusCmd.SetArgs([]string{"status"})
	out, err := executeCommandInDir(root, statusCmd)
	if err != nil {
		t.Fatalf("status failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, `"pending_proposals": 1`) {
		t.Fatalf("status did not report pending proposal:\n%s", out)
	}
}

func runCommandInDir(root string, command *cobra.Command) error {
	oldWD, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}
	defer func() { _ = os.Chdir(oldWD) }()
	return command.Execute()
}

func executeCommandInDir(root string, command *cobra.Command) (string, error) {
	oldWD, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if err := os.Chdir(root); err != nil {
		return "", err
	}
	defer func() { _ = os.Chdir(oldWD) }()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	os.Stdout = writer
	execErr := command.Execute()
	_ = writer.Close()
	os.Stdout = originalStdout
	out, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if execErr != nil {
		return string(out), execErr
	}
	if readErr != nil {
		return string(out), readErr
	}
	return string(out), nil
}

func prepareRunFinalizationEvidence(t *testing.T, root string, s *db.Store, runID int64) error {
	t.Helper()
	for _, phase := range []string{"0", "1", "2'", "3.5", "4"} {
		if err := s.SetPhaseStatus(runID, phase, "DONE", "pre-final gate complete", nil); err != nil {
			return err
		}
	}
	reviewBody := "CodeDungeon Code Review Verdict APPROVED Review Integrity PASS Findings Structured test review was generated by the standalone review module. Review Summary Final adjudication approved this fixture."
	writeFakeGH(t, filepath.Join(root, "fake-bin"), reviewBody)
	reviewDir := filepath.Join(root, ".codedungeon", "reviews", "project-context-finalize")
	if err := os.MkdirAll(reviewDir, 0o755); err != nil {
		return err
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
		return err
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
		return err
	}
	logPath := filepath.Join(root, ".codedungeon", "logs", "go-test.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		return err
	}
	_, err = s.InsertVerificationRecord(db.VerificationRecord{
		RunID:   runID,
		Phase:   "6",
		Command: "go test ./...",
		Status:  "PASS",
		LogPath: logPath,
	})
	return err
}
