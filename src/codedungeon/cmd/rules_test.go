package cmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectRulesLifecycleApproveCompactStatusAndLint(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n\nRun `go test ./...`.\n")
	rulesPath := filepath.Join(root, ".codedungeon", "project-rules.md")
	writeFile(t, rulesPath, strings.Join([]string{
		"# Project Rules",
		"",
		"Status: DRAFT",
		"Generated: 2026-04-26",
		"",
		"## Sources Reviewed",
		"- README.md",
		"",
		"## Architecture And Boundaries",
		"- Single Go module.",
		"",
		"## Project Rules",
		"- MUST keep work on main.",
		"- MUST run go test ./...",
		"",
		"## Commands And Verification",
		"- VERIFY with go test ./...",
		"",
		"## Security And Data Rules",
		"- MUST NOT commit secrets.",
		"",
		"## Agent Operating Rules",
		"- ASK WHEN requirements conflict.",
		"",
		"## Open Questions",
		"- None.",
		"",
	}, "\n"))

	if st, err := computeProjectRulesStatus(root); err != nil {
		t.Fatal(err)
	} else if st.Status != "draft" {
		t.Fatalf("status = %q, want draft", st.Status)
	}

	if _, err := approveProjectRules(root, "tester"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatalf("compact failed: %v", err)
	}
	if res := lintProjectRules(root); !res.OK {
		t.Fatalf("lint failed after approve/compact: %+v", res)
	}

	st, err := computeProjectRulesStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "approved" {
		t.Fatalf("status = %q, want approved: %+v", st.Status, st)
	}
	if st.RulesDigest == "" || st.SourceDigest == "" {
		t.Fatalf("expected digests in status: %+v", st)
	}

	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n\nChanged verification command.\n")
	st, err = computeProjectRulesStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "stale" || !strings.Contains(st.StaleReason, "source digest changed") {
		t.Fatalf("status after source change = %+v, want stale source digest change", st)
	}
}

func TestProjectRulesStatusRejectsMissingCompactEvidence(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeProjectRulesDraft(t, root)
	if _, err := approveProjectRules(root, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, ".codedungeon", "project-rules.compact.md")); err != nil {
		t.Fatal(err)
	}
	st, err := computeProjectRulesStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.OK || st.Status != "stale" || !strings.Contains(st.StaleReason, "missing project rules evidence") {
		t.Fatalf("status = %+v, want stale missing compact evidence", st)
	}
}

func TestRulesStatusUsesLocalCodeDungeonRootOverOuterGitRoot(t *testing.T) {
	outer := t.TempDir()
	runGit(t, outer, "init")
	inner := filepath.Join(outer, "realms", "tetoz")
	writeFile(t, filepath.Join(inner, ".codedungeon", "codedungeon.db"), "")
	writeFile(t, filepath.Join(inner, "README.md"), "# Tetoz\n")
	writeFile(t, filepath.Join(inner, ".codedungeon", "project-rules.md"), strings.Join([]string{
		"# Project Rules",
		"",
		"Status: DRAFT",
		"",
		"## Sources Reviewed",
		"- README.md",
		"",
		"## Architecture And Boundaries",
		"- Multi repo Tetoz realm.",
		"",
		"## Project Rules",
		"- MUST keep CodeDungeon state local to this realm.",
		"",
		"## Commands And Verification",
		"- VERIFY with codedungeon rules status.",
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

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(inner); err != nil {
		t.Fatal(err)
	}

	out := captureStdout(t, func() {
		cmd := rulesStatusCmd()
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	var st projectRulesStatus
	if err := json.Unmarshal([]byte(out), &st); err != nil {
		t.Fatalf("unmarshal rules status: %v\n%s", err, out)
	}
	if st.Status != "draft" {
		t.Fatalf("rules status = %q, want draft from local CodeDungeon root: %+v", st.Status, st)
	}
}

func TestNestedRulesCommandsAndRunDryRunUseSameCodeDungeonRoot(t *testing.T) {
	outer := t.TempDir()
	runGit(t, outer, "init")
	runGit(t, outer, "remote", "add", "origin", "https://github.com/example/tetoz.git")
	inner := filepath.Join(outer, "realms", "tetoz")
	writeFile(t, filepath.Join(inner, ".codedungeon", "codedungeon.db"), "")
	writeFile(t, filepath.Join(inner, "README.md"), "# Tetoz\n")
	writeProjectRulesDraft(t, inner)
	writeFile(t, filepath.Join(outer, ".codedungeon", "project-rules.md"), strings.Join([]string{
		"# Project Rules",
		"",
		"Status: DRAFT",
		"",
		"## Sources Reviewed",
		"- README.md",
		"",
		"## Architecture And Boundaries",
		"- Outer workspace should not be used.",
		"",
		"## Project Rules",
		"- MUST keep outer rules draft.",
		"",
		"## Commands And Verification",
		"- VERIFY outer should not run.",
		"",
		"## Security And Data Rules",
		"- MUST NOT use outer rules.",
		"",
		"## Agent Operating Rules",
		"- ASK WHEN blocked.",
		"",
		"## Open Questions",
		"- None.",
		"",
	}, "\n"))
	fakeBin := filepath.Join(outer, "bin")
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
	if err := os.Chdir(inner); err != nil {
		t.Fatal(err)
	}

	for _, args := range [][]string{
		{"approve", "--by", "test"},
		{"compact"},
		{"digest"},
		{"status"},
	} {
		cmd := RulesCmd()
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil {
			t.Fatalf("rules %v failed: %v", args, err)
		}
	}
	st, err := computeProjectRulesStatus(inner)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "approved" {
		t.Fatalf("inner rules status = %q, want approved", st.Status)
	}
	outerStatus, err := computeProjectRulesStatus(outer)
	if err != nil {
		t.Fatal(err)
	}
	if outerStatus.Status != "draft" {
		t.Fatalf("outer rules status = %q, want draft to prove commands used inner root", outerStatus.Status)
	}

	runCmd := RunCmd()
	runCmd.SetArgs([]string{"--full", "--prompt", "Ship Tetoz multi repo", "--dry-run"})
	out := captureStdout(t, func() {
		if err := runCmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	var payload map[string]any
	if err := unmarshalSetupJSON([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal run dry-run output: %v\n%s", err, out)
	}
	projectRules, ok := payload["project_rules"].(map[string]any)
	if !ok {
		t.Fatalf("project_rules missing from dry-run output: %#v", payload)
	}
	if projectRules["status"] != "approved" {
		t.Fatalf("dry-run project rules status = %v, want approved from inner root\n%s", projectRules["status"], out)
	}
}

func TestProjectRulesSourceDigestIgnoresGeneratedRuntimeArtifacts(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeFile(t, filepath.Join(root, ".git", "info", "exclude"), "examples/v2/\n")
	writeFile(t, filepath.Join(root, ".codedungeon", "project-rules.md"), strings.Join([]string{
		"# Project Rules",
		"",
		"Status: DRAFT",
		"",
		"## Sources Reviewed",
		"- README.md",
		"",
		"## Architecture And Boundaries",
		"- Single repo.",
		"",
		"## Project Rules",
		"- MUST run tests.",
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

	if _, err := approveProjectRules(root, "tester"); err != nil {
		t.Fatalf("approve failed: %v", err)
	}
	if _, err := compactProjectRules(root); err != nil {
		t.Fatalf("compact failed: %v", err)
	}

	writeFile(t, filepath.Join(root, ".codedungeon", "README.md"), "generated runtime doc changed\n")
	writeFile(t, filepath.Join(root, ".codex", "config.toml"), "[features]\nchanged = true\n")
	writeFile(t, filepath.Join(root, ".agents", "skills", "demo", "SKILL.md"), "---\nname: demo\n---\nchanged\n")
	writeFile(t, filepath.Join(root, "examples", "v2", "frontend", "package.json"), `{"dependencies":{"next":"15"}}`)

	st, err := computeProjectRulesStatus(root)
	if err != nil {
		t.Fatal(err)
	}
	if st.Status != "approved" {
		t.Fatalf("status after generated artifact changes = %+v, want approved", st)
	}
	for _, src := range st.Sources {
		if strings.HasPrefix(src, ".codedungeon/") ||
			strings.HasPrefix(src, ".codex/") ||
			strings.HasPrefix(src, ".agents/") ||
			strings.HasPrefix(src, "examples/v2/") {
			t.Fatalf("generated source %q should not be in project rules digest: %+v", src, st.Sources)
		}
	}
}

func TestProjectRulesSourceDigestIncludesIgnoredRepoBoundarySources(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, ".gitignore"), "/backend/\n/portal/\n/app/\n")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeFile(t, filepath.Join(root, "backend", "AGENTS.md"), "# Backend Guide\n")
	writeFile(t, filepath.Join(root, "backend", "Cargo.toml"), "[package]\nname = \"backend\"\n")
	writeFile(t, filepath.Join(root, "portal", "AGENTS.md"), "# Portal Guide\n")
	writeFile(t, filepath.Join(root, "portal", "package.json"), `{"scripts":{"lint":"next lint"}}`)
	writeFile(t, filepath.Join(root, "app", "AGENTS.md"), "# App Guide\n")
	writeFile(t, filepath.Join(root, "app", "gradle", "libs.versions.toml"), "[versions]\n")

	_, sources, err := computeProjectRulesSourceDigest(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"backend/AGENTS.md",
		"backend/Cargo.toml",
		"portal/AGENTS.md",
		"portal/package.json",
		"app/AGENTS.md",
		"app/gradle/libs.versions.toml",
	} {
		if !containsString(sources, want) {
			t.Fatalf("sources missing ignored repo boundary source %q: %v", want, sources)
		}
	}
}

func TestProjectRulesLintRejectsApprovedRulesWithOpenQuestions(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	writeFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeFile(t, filepath.Join(root, ".codedungeon", "project-rules.md"), strings.Join([]string{
		"# Project Rules",
		"",
		"Status: APPROVED",
		"",
		"## Sources Reviewed",
		"- README.md",
		"",
		"## Architecture And Boundaries",
		"- Single repo.",
		"",
		"## Project Rules",
		"- MUST run tests.",
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
		"- Which deploy target?",
		"",
	}, "\n"))
	writeFile(t, filepath.Join(root, ".codedungeon", "project-rules.compact.md"), strings.Join([]string{
		"# Project Rules Compact",
		"PROJECT_RULES_STATUS: APPROVED",
		"PROJECT_RULES_SOURCE: .codedungeon/project-rules.md",
		"- MUST run tests.",
		"",
	}, "\n"))

	res := lintProjectRules(root)
	if res.OK {
		t.Fatalf("lint OK = true, want false")
	}
	if !containsString(res.Errors, "approved rules must not have unresolved open questions") {
		t.Fatalf("lint errors = %v", res.Errors)
	}
}

func TestRulesGateBlocksCompletionInEnforceMode(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")

	res := evaluateRulesGate(root, gateOptions{
		Event:   "Stop",
		Mode:    "enforce",
		Message: "Status COMPLETE\nVerification: PASS",
	})
	if res.OK {
		t.Fatalf("gate OK = true, want false")
	}
	if !strings.Contains(res.Message, "PROJECT_RULES_STATUS") {
		t.Fatalf("gate message = %q, want project rules blocker", res.Message)
	}
}

func TestHooksInstallWritesProviderSpecificHookFiles(t *testing.T) {
	for _, tc := range []struct {
		provider string
		wantHook string
		wantCfg  string
	}{
		{"codex", filepath.Join(".codex", "hooks", "project-rules-gate.ps1"), filepath.Join(".codex", "config.toml")},
		{"claude", filepath.Join(".claude", "hooks", "project-rules-gate.ps1"), filepath.Join(".claude", "settings.json")},
	} {
		t.Run(tc.provider, func(t *testing.T) {
			root := t.TempDir()
			runGit(t, root, "init")
			if err := installProjectRulesHooks(root, tc.provider, "warn"); err != nil {
				t.Fatal(err)
			}
			assertFileExists(t, filepath.Join(root, tc.wantHook))
			cfg, err := os.ReadFile(filepath.Join(root, tc.wantCfg))
			if err != nil {
				t.Fatal(err)
			}
			body := string(cfg)
			for _, required := range []string{"codedungeon rules gate", "warn", "project-rules-gate.ps1"} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s missing %q:\n%s", tc.wantCfg, required, body)
				}
			}
		})
	}
}

func TestClaudeHookScriptBlocksStopEventsWithExitCodeTwo(t *testing.T) {
	script := projectRulesHookScript(".claude/bin/codedungeon", "enforce")
	for _, required := range []string{
		"$eventName -eq \"Stop\"",
		"$eventName -eq \"SubagentStop\"",
		"exit 2",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("claude hook script missing documented blocking behavior %q:\n%s", required, script)
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	defer func() { os.Stdout = originalStdout }()
	fn()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}
