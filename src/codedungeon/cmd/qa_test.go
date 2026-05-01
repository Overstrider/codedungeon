package cmd

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestDetectTestFrameworkDiscoversMonorepoComponents(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "backend", "Cargo.toml"), `
[package]
name = "api"
version = "0.1.0"
`)
	writeFile(t, filepath.Join(root, "frontend", "package.json"), `{"devDependencies":{"vitest":"latest"}}`)

	result := detectProjectTestFramework(root)
	if result.Framework != "monorepo" {
		t.Fatalf("framework = %q, want monorepo: %+v", result.Framework, result)
	}
	if len(result.Components) != 2 {
		t.Fatalf("components = %+v, want backend and frontend", result.Components)
	}
	if result.RunCmd == "" {
		t.Fatalf("run_cmd should summarize component commands: %+v", result)
	}
	if !containsString(result.RunCmds, "cd backend && cargo test") {
		t.Fatalf("run_cmds missing backend cargo test: %+v", result.RunCmds)
	}
	if !containsString(result.RunCmds, "cd frontend && npx vitest run") {
		t.Fatalf("run_cmds missing frontend vitest: %+v", result.RunCmds)
	}
}

func TestQASecretScanOpenRouterTrackedOnly(t *testing.T) {
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
	writeFile(t, filepath.Join(root, "README.md"), "Set OPENROUTER_API_KEY in your environment; do not commit real values.\n")
	runGit(t, root, "add", "README.md")

	cmd := QACmd()
	cmd.SetArgs([]string{"secret-scan", "--kind", "openrouter", "--tracked-only"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("safe tracked docs should pass secret scan: %v", err)
	}

	writeFile(t, filepath.Join(root, ".env.example"), "OPENROUTER_API_KEY="+"sk-or-v1-"+strings.Repeat("a", 24)+"\n")
	runGit(t, root, "add", ".env.example")
	cmd = QACmd()
	cmd.SetArgs([]string{"secret-scan", "--kind", "openrouter", "--tracked-only"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "secret scan failed") {
		t.Fatalf("committed-looking OpenRouter key was not blocked: %v", err)
	}
}

func TestQARunReadsCommandFromFile(t *testing.T) {
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
	runID, err := store.CreateRun(&db.Run{Feature: "qa cmd file", Branch: "feat/qa", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	commandFile := filepath.Join(root, "verify.cmd")
	writeFile(t, commandFile, "echo qa-cmd-file-ok\n")

	cmd := QACmd()
	cmd.SetArgs([]string{"run", "--phase", "6", "--cmd-file", commandFile})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err = db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	records, err := store.VerificationRecords(runID, "6")
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	if strings.TrimSpace(records[0].Command) != "echo qa-cmd-file-ok" || records[0].Status != "PASS" {
		t.Fatalf("unexpected record: %+v", records[0])
	}
}

func TestQARunStandaloneAutoDoesNotRequireActiveRun(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/qa\n\ngo 1.25.0\n")
	writeFile(t, filepath.Join(root, "smoke_test.go"), "package qa\n\nimport \"testing\"\n\nfunc TestSmoke(t *testing.T) {}\n")
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	cmd := QACmd()
	cmd.SetArgs([]string{"run", "--auto"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("standalone qa run should not require active run: %v", err)
	}

	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session := latestQASessionByStatus(t, store.DB, "PASS")
	if session == "" {
		t.Fatal("qa session was not persisted")
	}
}

func TestQAPreflightE2EReportsMissingPlaywrightAsBlocked(t *testing.T) {
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

	cmd := QACmd()
	cmd.SetArgs([]string{"preflight", "--mode", "e2e"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("preflight should report blocked without failing cobra execution: %v", err)
	}

	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session := latestQASessionByStatus(t, store.DB, "BLOCKED")
	if session == "" {
		t.Fatal("blocked qa session was not persisted")
	}
	var name, status string
	var required int
	if err := store.DB.QueryRow(`SELECT name, status, required FROM qa_dependencies WHERE session_id=?`, session).Scan(&name, &status, &required); err != nil {
		t.Fatal(err)
	}
	if name != "playwright" || status != "missing" || required != 1 {
		t.Fatalf("dependency = %s %s required=%d, want playwright missing required=1", name, status, required)
	}
}

func latestQASessionByStatus(t *testing.T, database *sql.DB, status string) string {
	t.Helper()
	var id string
	err := database.QueryRow(`SELECT id FROM qa_sessions WHERE status=? ORDER BY started_at DESC LIMIT 1`, status).Scan(&id)
	if err != nil {
		if err == sql.ErrNoRows {
			return ""
		}
		t.Fatal(err)
	}
	return id
}
