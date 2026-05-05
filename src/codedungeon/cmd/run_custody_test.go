package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStartProviderFailureEmitsCustodyStatus(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runGit(t, root, "remote", "add", "origin", "https://github.com/example/repo.git")
	writeFile(t, filepath.Join(root, "README.md"), "# Custody test\n")
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
	oldExecutor := providerChildExecutor
	providerChildExecutor = func(root, mode, prompt string, runID int64, sessionID, token string) error {
		return fmt.Errorf("provider crashed before PR")
	}
	t.Cleanup(func() { providerChildExecutor = oldExecutor })

	cmd := RunCmd()
	cmd.SetArgs([]string{"--full", "--prompt", "Provider child failure custody"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = cmd.Execute()
	})
	if execErr == nil {
		t.Fatal("expected run failure")
	}
	var payload map[string]any
	if err := unmarshalSetupJSON([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal failure output: %v\n%s", err, out)
	}
	if payload["custody_status"] != "NO_CODEDUNGEON_DELIVERY_CREATED" {
		t.Fatalf("custody_status = %v, want NO_CODEDUNGEON_DELIVERY_CREATED\n%s", payload["custody_status"], out)
	}
	if !strings.Contains(fmt.Sprint(payload["error"]), "no finalized CodeDungeon delivery exists") {
		t.Fatalf("error should state no finalized delivery exists:\n%s", out)
	}
	commands, ok := payload["recovery_commands"].([]any)
	if !ok || len(commands) == 0 || !strings.Contains(commands[0].(string), "codedungeon observe") {
		t.Fatalf("recovery_commands = %#v, want runner recovery commands", payload["recovery_commands"])
	}
}

func writeProjectRulesDraft(t *testing.T, root string) {
	t.Helper()
	writeFile(t, filepath.Join(root, ".codedungeon", "project-rules.md"), strings.Join([]string{
		"# Project Rules",
		"",
		"Status: DRAFT",
		"",
		"## Sources Reviewed",
		"- README.md",
		"",
		"## Architecture And Boundaries",
		"- Single repo custody test.",
		"",
		"## Project Rules",
		"- MUST keep custody state explicit.",
		"",
		"## Commands And Verification",
		"- VERIFY with go test ./...",
		"",
		"## Security And Data Rules",
		"- MUST NOT commit secrets.",
		"",
		"## Agent Operating Rules",
		"- ASK WHEN blocked.",
		"",
		"## Open Questions",
		"- None.",
		"",
	}, "\n"))
}
