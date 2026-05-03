package preflight

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

type fakeRunner struct {
	results map[string]tooladapter.CommandResult
	errors  map[string]error
}

func (r fakeRunner) Run(_ context.Context, cmd tooladapter.Command) (tooladapter.CommandResult, error) {
	key := cmd.Name + " " + strings.Join(cmd.Args, " ")
	if err, ok := r.errors[key]; ok {
		return tooladapter.CommandResult{}, err
	}
	if result, ok := r.results[key]; ok {
		return result, nil
	}
	return tooladapter.CommandResult{}, nil
}

func TestRunReportsGitHubAuthAsBlockingFailure(t *testing.T) {
	root := readyRoot(t)
	runner := fakeRunner{
		results: map[string]tooladapter.CommandResult{
			"git --version":             {Stdout: "git version 2.45.0"},
			"git remote get-url origin": {Stdout: "https://github.com/acme/repo.git"},
			"gh --version":              {Stdout: "gh version 2.0.0"},
			"codex --version":           {Stdout: "codex 1.0.0"},
			"git branch --show-current": {Stdout: "feature/preflight"},
			"gh repo view --json nameWithOwner -q .nameWithOwner": {Stdout: "acme/repo"},
		},
		errors: map[string]error{
			"gh auth status": tooladapter.ToolError{Kind: tooladapter.ErrorExit, Tool: "gh", Operation: "auth status", ExitCode: 1, Stderr: "not logged in"},
		},
	}

	report, err := Run(context.Background(), Request{
		Root:     root,
		Provider: provider.Codex{},
		Runner:   runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.OK {
		t.Fatalf("report should fail when gh auth is missing: %+v", report)
	}
	check := findCheck(report, "gh_auth")
	if check == nil {
		t.Fatalf("missing gh_auth check: %+v", report.Checks)
	}
	if check.Status != StatusFail || !check.Blocker {
		t.Fatalf("gh_auth check = %+v, want blocking FAIL", check)
	}
	if !contains(report.NextCommands, "gh auth login") {
		t.Fatalf("next commands should suggest gh auth login: %+v", report.NextCommands)
	}
}

func TestRunRedactsSecretFindings(t *testing.T) {
	root := readyRoot(t)
	writeFile(t, filepath.Join(root, "README.md"), "OPENAI_API_KEY=sk-proj-abcdefghijklmnopqrstuvwxyz123456\n")
	runner := happyRunner()

	report, err := Run(context.Background(), Request{
		Root:     root,
		Provider: provider.Codex{},
		Runner:   runner,
	})
	if err != nil {
		t.Fatal(err)
	}

	check := findCheck(report, "secrets")
	if check == nil {
		t.Fatalf("missing secrets check: %+v", report.Checks)
	}
	if check.Status != StatusFail || !check.Blocker {
		t.Fatalf("secrets check = %+v, want blocking FAIL", check)
	}
	if strings.Contains(check.Detail, "abcdefghijklmnopqrstuvwxyz123456") {
		t.Fatalf("secret detail leaked raw token: %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "sk-proj") || !strings.Contains(check.Detail, "...") {
		t.Fatalf("secret detail should include redacted token prefix: %q", check.Detail)
	}
}

func TestStrictModeTreatsWarningsAsNotReady(t *testing.T) {
	root := readyRoot(t)
	report, err := Run(context.Background(), Request{
		Root:     root,
		Strict:   true,
		Provider: provider.Codex{},
		Runner:   happyRunner(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if report.OK {
		t.Fatalf("strict report should fail on warnings: %+v", report)
	}
	check := findCheck(report, "framework")
	if check == nil {
		t.Fatalf("missing framework warning: %+v", report.Checks)
	}
	if check.Status != StatusWarn || check.Blocker {
		t.Fatalf("framework check = %+v, want non-blocking WARN", check)
	}
}

func readyRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	mkdir(t, filepath.Join(root, ".git"))
	mkdir(t, filepath.Join(root, ".codedungeon"))
	writeFile(t, filepath.Join(root, ".codedungeon", "project-context.md"), "context")
	mkdir(t, filepath.Join(root, ".codedungeon", "plan"))
	writeFile(t, filepath.Join(root, ".codedungeon", "plan", "PLAN.md"), "# plan")
	mkdir(t, filepath.Join(root, ".codedungeon", "tasks"))
	writeFile(t, filepath.Join(root, ".codedungeon", "tasks", "task-001.md"), "# task")
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("model_reasoning", "gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("model_fast", "gpt-5.5"); err != nil {
		t.Fatal(err)
	}
	return root
}

func happyRunner() fakeRunner {
	return fakeRunner{results: map[string]tooladapter.CommandResult{
		"git --version":             {Stdout: "git version 2.45.0"},
		"git remote get-url origin": {Stdout: "https://github.com/acme/repo.git"},
		"git branch --show-current": {Stdout: "feature/preflight"},
		"gh --version":              {Stdout: "gh version 2.0.0"},
		"gh auth status":            {Stdout: "Logged in"},
		"gh repo view --json nameWithOwner -q .nameWithOwner": {Stdout: "acme/repo"},
		"codex --version": {Stdout: "codex 1.0.0"},
	}}
}

func findCheck(report Report, id string) *Check {
	for i := range report.Checks {
		if report.Checks[i].ID == id {
			return &report.Checks[i]
		}
	}
	return nil
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := (tooladapter.OSFileSystem{}).MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := (tooladapter.OSFileSystem{}).MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (tooladapter.OSFileSystem{}).WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
