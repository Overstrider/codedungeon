package taskexec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func TestLoadConfigAppliesDefaultsFileAndEnv(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, ".ralphrc"), map[string]any{
		"session_ttl_hours": 12,
		"timeout_seconds":   99,
		"auto_push":         true,
		"allowed_tools":     []string{"shell:go test ./...", "git:status"},
	})
	t.Setenv("CODEDUNGEON_EXEC_TIMEOUT_SECONDS", "7")
	t.Setenv("CODEDUNGEON_EXEC_AUTO_PUSH", "false")

	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SessionTTLHours != 12 {
		t.Fatalf("SessionTTLHours = %d, want 12", cfg.SessionTTLHours)
	}
	if cfg.TimeoutSeconds != 7 {
		t.Fatalf("TimeoutSeconds = %d, want env override 7", cfg.TimeoutSeconds)
	}
	if cfg.AutoCommit != true {
		t.Fatalf("AutoCommit = false, want default true")
	}
	if cfg.AutoPush != false {
		t.Fatalf("AutoPush = true, want env override false")
	}
	if len(cfg.AllowedTools) != 2 {
		t.Fatalf("AllowedTools = %v, want file values", cfg.AllowedTools)
	}
}

func TestLoadConfigAcceptsPowerShellUTF8BOM(t *testing.T) {
	root := t.TempDir()
	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{"runner":"codex","max_iterations":1}`)...)
	if err := os.WriteFile(filepath.Join(root, ".ralphrc"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxIterations != 1 {
		t.Fatalf("MaxIterations = %d, want 1", cfg.MaxIterations)
	}
}

func TestLoadConfigAppliesPromptPlannerDefaultsAndRalphrcOverrides(t *testing.T) {
	root := t.TempDir()
	writeJSON(t, filepath.Join(root, ".ralphrc"), map[string]any{
		"execution": map[string]any{
			"prompt_planner": map[string]any{
				"enabled":          false,
				"model":            "gpt-5.4",
				"reasoning_effort": "medium",
				"timeout":          "2m",
				"ephemeral":        false,
				"fallback_models":  []string{"gpt-5.3-codex"},
			},
		},
	})

	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PromptPlanner.Enabled {
		t.Fatal("PromptPlanner.Enabled = true, want override false")
	}
	if cfg.PromptPlanner.Model != "gpt-5.4" {
		t.Fatalf("PromptPlanner.Model = %q, want gpt-5.4", cfg.PromptPlanner.Model)
	}
	if cfg.PromptPlanner.ReasoningEffort != "medium" {
		t.Fatalf("PromptPlanner.ReasoningEffort = %q, want medium", cfg.PromptPlanner.ReasoningEffort)
	}
	if cfg.PromptPlanner.TimeoutSeconds != 120 {
		t.Fatalf("PromptPlanner.TimeoutSeconds = %d, want 120", cfg.PromptPlanner.TimeoutSeconds)
	}
	if cfg.PromptPlanner.Ephemeral {
		t.Fatal("PromptPlanner.Ephemeral = true, want override false")
	}
	if len(cfg.PromptPlanner.FallbackModels) != 1 || cfg.PromptPlanner.FallbackModels[0] != "gpt-5.3-codex" {
		t.Fatalf("PromptPlanner.FallbackModels = %v", cfg.PromptPlanner.FallbackModels)
	}
}

func TestLoadConfigAcceptsDottedPromptPlannerRalphrc(t *testing.T) {
	root := t.TempDir()
	body := strings.Join([]string{
		"execution.prompt_planner.enabled=true",
		"execution.prompt_planner.model=gpt-5.4",
		"execution.prompt_planner.reasoning_effort=high",
		"execution.prompt_planner.timeout=10m",
		"execution.prompt_planner.ephemeral=true",
		`execution.prompt_planner.fallback_models=["gpt-5.3-codex","gpt-5.4-mini"]`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(root, ".ralphrc"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.PromptPlanner.Enabled || cfg.PromptPlanner.Model != "gpt-5.4" || cfg.PromptPlanner.TimeoutSeconds != 600 {
		t.Fatalf("prompt planner config not parsed: %+v", cfg.PromptPlanner)
	}
	if got := strings.Join(cfg.PromptPlanner.FallbackModels, ","); got != "gpt-5.3-codex,gpt-5.4-mini" {
		t.Fatalf("fallback models = %q", got)
	}
}

func TestReadJSONFileAcceptsPowerShellUTF8BOM(t *testing.T) {
	path := filepath.Join(t.TempDir(), "task.json")
	body := append([]byte{0xEF, 0xBB, 0xBF}, []byte(`{
  "id":"TASK-BOM",
  "repo":".",
  "title":"BOM task",
  "objective":"Read JSON files saved by PowerShell on Windows.",
  "write_scope":["README.md"],
  "acceptance_criteria":["task loads"],
  "verification_commands":["go test ./internal/taskexec"]
}`)...)
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	task, err := readJSONFile[taskplanning.TaskSpec](path)
	if err != nil {
		t.Fatal(err)
	}
	if task.ID != "TASK-BOM" {
		t.Fatalf("task ID = %q", task.ID)
	}
}

func TestSafetyPolicyBlocksDestructiveCommandsAndAllowsWhitelistedGit(t *testing.T) {
	policy := SafetyPolicy{AllowedTools: []string{
		"shell:go test ./...",
		"git:status",
		"git:diff",
		"git:commit",
	}}
	for _, cmd := range []string{
		"rm -rf .",
		"git reset --hard HEAD",
		"git clean -fdx",
		"powershell Remove-Item -Recurse -Force .",
	} {
		if err := policy.ValidateShellCommand(cmd); err == nil {
			t.Fatalf("ValidateShellCommand(%q) succeeded, want block", cmd)
		}
	}
	if err := policy.ValidateGit("status"); err != nil {
		t.Fatalf("ValidateGit(status) = %v", err)
	}
	if err := policy.ValidateGit("push"); err == nil {
		t.Fatal("ValidateGit(push) succeeded without whitelist")
	}
	if err := policy.ValidateShellCommand("go test ./..."); err != nil {
		t.Fatalf("ValidateShellCommand(go test ./...) = %v", err)
	}
}

func TestEffectiveConfigAllowsTaskVerificationCommands(t *testing.T) {
	cfg := Config{
		Runner:       "codex",
		AllowedTools: []string{"git:status"},
	}
	task := taskplanning.TaskSpec{
		ID:                   "TASK-VERIFY",
		Repo:                 ".",
		Title:                "Verify task",
		Objective:            "Verify custom command permission.",
		WriteScope:           []string{"cmd"},
		AcceptanceCriteria:   []string{"custom command is allowed"},
		VerificationCommands: []string{"pnpm test -- --runInBand", "go test ./cmd"},
	}

	effective := EffectiveConfigForTask(cfg, task)
	for _, want := range []string{"git:status", "shell:pnpm test -- --runInBand", "shell:go test ./cmd"} {
		if !containsString(effective.AllowedTools, want) {
			t.Fatalf("allowed tools missing %q: %v", want, effective.AllowedTools)
		}
	}
	if err := (SafetyPolicy{AllowedTools: effective.AllowedTools}).ValidateShellCommand("pnpm test -- --runInBand"); err != nil {
		t.Fatalf("task verification command was not allowed: %v", err)
	}
}

func TestSessionStoreRequiresExplicitResumeAndExpiresOldSession(t *testing.T) {
	root := t.TempDir()
	store := openTaskExecStore(t, root)
	defer store.Close()
	runID := createTaskExecRun(t, store)
	sessionDir := filepath.Join(root, ".codedungeon", "execute", "sessions")
	sessions := NewSessionStore(store, sessionDir, 24)

	sess, err := sessions.Start(StartSessionRequest{
		RunID:    runID,
		TaskID:   "TASK-001",
		TaskPath: "task.json",
		Provider: "codex",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Resume("", false); err == nil {
		t.Fatal("Resume with empty id succeeded; want explicit --resume id")
	}
	if _, err := sessions.Resume(sess.ID, false); err != nil {
		t.Fatalf("Resume valid session failed: %v", err)
	}
	if err := store.UpdateExecutionSessionStartedAt(sess.ID, sess.StartedAt-25*60*60); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.Resume(sess.ID, false); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("Resume expired error = %v, want expired", err)
	}
	if err := store.UpdateExecutionSessionAttempt(sess.ID, 3); err != nil {
		t.Fatal(err)
	}
	if reset, err := sessions.Resume(sess.ID, true); err != nil || reset.Status != StatusRunning || reset.Attempt != 0 {
		t.Fatalf("Resume reset = %+v err=%v, want running", reset, err)
	}
}

func TestEngineRunsTaskRecordsTransitionAndCommitsAfterPassingVerification(t *testing.T) {
	root := t.TempDir()
	store := openTaskExecStore(t, root)
	defer store.Close()
	runID := createTaskExecRun(t, store)
	taskPath := filepath.Join(root, "task.json")
	writeJSON(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-001",
		Repo:                 ".",
		Kind:                 "dev",
		Title:                "Add executor",
		Objective:            "Implement one executor behavior.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"engine commits after tests pass"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})
	git := &fakeGit{head: "before", changed: []string{"internal/taskexec/engine.go"}}
	runner := &fakeRunner{result: AgentResult{Status: WorkerPassed, Summary: "implemented task", SessionID: "provider-session"}}
	verifier := &fakeVerifier{results: []VerificationResult{{Command: "go test ./internal/taskexec", Status: "PASS", LogPath: "pass.log"}}}
	cfg := DefaultConfig()
	cfg.AutoCommit = true

	result, err := Execute(context.Background(), Request{
		Root:           root,
		RunID:          runID,
		TaskPath:       taskPath,
		ProjectContext: "project context has enough words to be substantive for execution tests",
		Config:         cfg,
		Runner:         runner,
		Git:            git,
		Verifier:       verifier,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted {
		t.Fatalf("result status = %s, want COMPLETED", result.Status)
	}
	if len(runner.requests) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.requests))
	}
	if !git.backupCreated {
		t.Fatal("backup ref was not created")
	}
	if !git.committed {
		t.Fatal("git commit was not called after PASS")
	}
	loaded, err := store.ExecutionSession(result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || loaded.Status != StatusCompleted {
		t.Fatalf("loaded session = %+v, want completed", loaded)
	}
	transitions, err := store.ExecutionTransitions(result.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	if len(transitions) == 0 || transitions[len(transitions)-1].ToStatus != StatusCompleted {
		t.Fatalf("transitions = %+v, want final completed", transitions)
	}
}

func TestEngineDryRunDoesNotInvokeRunnerOrMutateGit(t *testing.T) {
	root := t.TempDir()
	store := openTaskExecStore(t, root)
	defer store.Close()
	runID := createTaskExecRun(t, store)
	taskPath := filepath.Join(root, "task.json")
	writeJSON(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-002",
		Repo:                 ".",
		Title:                "Dry run",
		Objective:            "Show execution prompt without running.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"dry run is non-mutating"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})
	runner := &fakeRunner{}
	git := &fakeGit{head: "before"}

	result, err := Execute(context.Background(), Request{
		Root:           root,
		RunID:          runID,
		TaskPath:       taskPath,
		ProjectContext: "project context has enough words to be substantive for execution tests",
		Config:         DefaultConfig(),
		Runner:         runner,
		Git:            git,
		Verifier:       &fakeVerifier{},
		DryRun:         true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusDryRun {
		t.Fatalf("status = %s, want DRY_RUN", result.Status)
	}
	if len(runner.requests) != 0 {
		t.Fatalf("runner invoked in dry run: %d", len(runner.requests))
	}
	if git.backupCreated || git.committed {
		t.Fatalf("git mutated in dry run: backup=%v commit=%v", git.backupCreated, git.committed)
	}
	prompt, ok := result.Metadata["prompt"].(string)
	if !ok || !strings.Contains(prompt, "Only modify files declared in the task write_scope") {
		t.Fatalf("dry-run prompt did not include default workspace policy:\n%v", result.Metadata["prompt"])
	}
}

func TestEngineAutoTagUsesIncrementalSemverTag(t *testing.T) {
	root := t.TempDir()
	store := openTaskExecStore(t, root)
	defer store.Close()
	runID := createTaskExecRun(t, store)
	taskPath := filepath.Join(root, "task.json")
	writeJSON(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-003",
		Repo:                 ".",
		Kind:                 "dev",
		Title:                "Tag release",
		Objective:            "Create an incremental semver tag when enabled.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"auto_tag increments latest semver patch"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})
	git := &fakeGit{head: "before", latestTag: "v1.2.3"}
	runner := &fakeRunner{result: AgentResult{Status: WorkerPassed, Summary: "implemented task"}}
	verifier := &fakeVerifier{results: []VerificationResult{{Command: "go test ./internal/taskexec", Status: "PASS", LogPath: "pass.log"}}}
	cfg := DefaultConfig()
	cfg.AutoCommit = false
	cfg.AutoTag = true

	if _, err := Execute(context.Background(), Request{
		Root:           root,
		RunID:          runID,
		TaskPath:       taskPath,
		ProjectContext: "project context has enough words to be substantive for execution tests",
		Config:         cfg,
		Runner:         runner,
		Git:            git,
		Verifier:       verifier,
	}); err != nil {
		t.Fatal(err)
	}
	if git.tagName != "v1.2.4" {
		t.Fatalf("tagName = %q, want v1.2.4", git.tagName)
	}
}

type fakeRunner struct {
	requests []AgentRequest
	result   AgentResult
	err      error
}

func (f *fakeRunner) RunTask(ctx context.Context, req AgentRequest) (AgentResult, error) {
	f.requests = append(f.requests, req)
	return f.result, f.err
}

type fakeGit struct {
	head          string
	changed       []string
	latestTag     string
	backupCreated bool
	committed     bool
	pushed        bool
	tagged        bool
	tagName       string
}

func (f *fakeGit) Head(_ context.Context, _ string) (string, error) { return f.head, nil }
func (f *fakeGit) CreateBackupRef(_ context.Context, _ string, _ string, _ string) error {
	f.backupCreated = true
	return nil
}
func (f *fakeGit) ChangedFiles(_ context.Context, _ string) ([]string, error) { return f.changed, nil }
func (f *fakeGit) Diff(_ context.Context, _ string) (string, error)           { return "diff", nil }
func (f *fakeGit) Commit(_ context.Context, _ string, _ string) error {
	f.committed = true
	return nil
}
func (f *fakeGit) Push(_ context.Context, _ string) error {
	f.pushed = true
	return nil
}
func (f *fakeGit) LatestSemverTag(_ context.Context, _ string) (string, error) {
	return f.latestTag, nil
}
func (f *fakeGit) Tag(_ context.Context, _ string, tag string, _ string) error {
	f.tagged = true
	f.tagName = tag
	return nil
}

type fakeVerifier struct {
	results []VerificationResult
	err     error
}

func (f *fakeVerifier) Verify(ctx context.Context, req VerifyRequest) ([]VerificationResult, error) {
	return f.results, f.err
}

func openTaskExecStore(t *testing.T, root string) *db.Store {
	t.Helper()
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	return store
}

func createTaskExecRun(t *testing.T, store *db.Store) int64 {
	t.Helper()
	runID, err := store.CreateRun(&db.Run{Feature: "task execution", Branch: "feat/task-exec", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	return runID
}

func writeJSON(t *testing.T, path string, payload any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
