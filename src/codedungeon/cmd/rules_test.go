package cmd

import (
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
