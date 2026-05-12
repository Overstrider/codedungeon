package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunStartReturnsAgentFirstContractWithoutProviderChild(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
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
	calledProviderChild := false
	providerChildExecutor = func(root, mode, prompt string, runID int64, sessionID, token string) error {
		calledProviderChild = true
		return fmt.Errorf("provider crashed before PR")
	}
	t.Cleanup(func() { providerChildExecutor = oldExecutor })

	cmd := RunCmd()
	cmd.SetArgs([]string{"--full", "--prompt", "Provider child failure custody"})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	if calledProviderChild {
		t.Fatal("agent-first run start should not invoke provider child executor")
	}
	var payload map[string]any
	if err := unmarshalSetupJSON([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal start output: %v\n%s", err, out)
	}
	if payload["agent_first"] != true || payload["status"] != "ACTION_REQUIRED" {
		t.Fatalf("unexpected agent-first payload: %+v\n%s", payload, out)
	}
	current, ok := payload["current_step"].(map[string]any)
	if !ok || current["id"] != "planning" {
		t.Fatalf("current_step = %#v, want planning\n%s", payload["current_step"], out)
	}
	next, ok := payload["next_action"].(map[string]any)
	if !ok || !strings.Contains(fmt.Sprint(next["command"]), "codedungeon plan run") {
		t.Fatalf("next_action = %#v, want planning command\n%s", payload["next_action"], out)
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
	if sess == nil || sess.Status != "WAITING_FOR_AGENT" {
		t.Fatalf("session = %+v, want WAITING_FOR_AGENT", sess)
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
