package cmd

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestArtifactsBackfillIndexesExistingQAEvidence(t *testing.T) {
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
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	runID, err := store.CreateRun(&db.Run{Feature: "artifact backfill", Branch: "main", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	evidenceDir := filepath.Join(root, ".codedungeon", "qa", "sessions", "qa-1")
	writeFile(t, filepath.Join(evidenceDir, "summary.md"), "# summary\n")
	writeFile(t, filepath.Join(evidenceDir, "result.json"), `{"status":"PASS"}`)
	writeFile(t, filepath.Join(evidenceDir, "logs", "check.log"), "ok\n")
	if err := store.UpsertQASession(db.QASession{
		ID:          "qa-1",
		RunID:       runID,
		Entrypoint:  "standalone",
		Mode:        "auto",
		Status:      "PASS",
		Root:        root,
		EvidenceDir: evidenceDir,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.InsertQACheck(db.QACheck{
		ID:        "qa-1-check",
		SessionID: "qa-1",
		Kind:      "command",
		Name:      "go test",
		Status:    "PASS",
		Command:   "go test ./...",
		LogPath:   filepath.Join(evidenceDir, "logs", "check.log"),
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := ArtifactsCmd()
	cmd.SetArgs([]string{"backfill", "--run", strconv.FormatInt(runID, 10)})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err = db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	rows, err := store.ArtifactsByRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) < 3 {
		t.Fatalf("expected qa artifacts to be backfilled, got %+v", rows)
	}
	if !hasArtifact(rows, "qa", "summary", ".codedungeon/qa/sessions/qa-1/summary.md") {
		t.Fatalf("missing qa summary artifact: %+v", rows)
	}
	if !hasArtifact(rows, "qa", "log", ".codedungeon/qa/sessions/qa-1/logs/check.log") {
		t.Fatalf("missing qa log artifact: %+v", rows)
	}
}

func hasArtifact(rows []db.Artifact, module, role, path string) bool {
	for _, row := range rows {
		if row.Module == module && row.Role == role && row.Path == path {
			return true
		}
	}
	return false
}
