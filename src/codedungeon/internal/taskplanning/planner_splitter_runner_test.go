package taskplanning

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPlannerSplitterRunnerUsesEphemeralReadOnlyStructuredCodexExec(t *testing.T) {
	outDir := t.TempDir()
	var captured PlannerCommandInvocation
	runner := PlannerSplitterRunner{
		WorkDir:         "C:\\repo",
		Model:           "gpt-5.5",
		ReasoningEffort: "high",
		Timeout:         time.Minute,
		Ephemeral:       true,
		CommandRunner: func(ctx context.Context, inv PlannerCommandInvocation) error {
			captured = inv
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("planner command context has no timeout deadline")
			}
			lastMessage := argValue(inv.Args, "--output-last-message")
			if lastMessage == "" {
				t.Fatal("--output-last-message missing")
			}
			return os.WriteFile(lastMessage, []byte(validPlannerSplitterOutputJSON()), 0o644)
		},
	}

	result, err := runner.Plan(context.Background(), PromptPlannerRequest{
		Prompt:          "Add a new execute mode",
		ProjectContext:  strings.Repeat("project context for planner splitter. ", 3),
		WorkspacePolicy: "Only inspect source and produce a graph.",
		OutputDir:       outDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskGraph == nil || len(result.TaskGraph.Tasks) != 1 {
		t.Fatalf("planner graph = %+v, want one task", result.TaskGraph)
	}
	if captured.Name != "codex" || len(captured.Args) == 0 || captured.Args[0] != "exec" {
		t.Fatalf("command = %s %v, want codex exec", captured.Name, captured.Args)
	}
	for _, want := range []string{"--ephemeral", "--output-schema", "--output-last-message", "-"} {
		if !containsString(captured.Args, want) {
			t.Fatalf("planner args missing %q: %v", want, captured.Args)
		}
	}
	if sandbox := argValue(captured.Args, "--sandbox"); sandbox != "read-only" {
		t.Fatalf("--sandbox = %q, want read-only; args=%v", sandbox, captured.Args)
	}
	if model := argValue(captured.Args, "--model"); model != "gpt-5.5" {
		t.Fatalf("--model = %q, want gpt-5.5; args=%v", model, captured.Args)
	}
	if !containsString(captured.Args, `model_reasoning_effort="high"`) {
		t.Fatalf("reasoning effort config missing from args: %v", captured.Args)
	}
	if containsString(captured.Args, "resume") {
		t.Fatalf("planner must not use resume: %v", captured.Args)
	}
	if !strings.Contains(captured.Stdin, "CodeDungeon planner+splitter") ||
		!strings.Contains(captured.Stdin, "Do not edit files") ||
		!strings.Contains(captured.Stdin, "User prompt:") {
		t.Fatalf("planner prompt missing role/rules:\n%s", captured.Stdin)
	}
	if _, err := os.Stat(argValue(captured.Args, "--output-schema")); err != nil {
		t.Fatalf("output schema was not written: %v", err)
	}
}

func TestPlannerSplitterRunnerUsesConfiguredFallbackWhenModelCatalogMissesPrimary(t *testing.T) {
	outDir := t.TempDir()
	var args []string
	runner := PlannerSplitterRunner{
		WorkDir:         ".",
		Model:           "gpt-5.5",
		FallbackModels:  []string{"gpt-5.4"},
		AvailableModels: []string{"gpt-5.4"},
		Timeout:         time.Minute,
		Ephemeral:       true,
		CommandRunner: func(_ context.Context, inv PlannerCommandInvocation) error {
			args = inv.Args
			return os.WriteFile(argValue(inv.Args, "--output-last-message"), []byte(validPlannerSplitterOutputJSON()), 0o644)
		},
	}

	if _, err := runner.Plan(context.Background(), PromptPlannerRequest{
		Prompt:         "Add planner fallback",
		ProjectContext: strings.Repeat("project context for planner fallback. ", 3),
		OutputDir:      outDir,
	}); err != nil {
		t.Fatal(err)
	}
	if model := argValue(args, "--model"); model != "gpt-5.4" {
		t.Fatalf("--model = %q, want fallback gpt-5.4; args=%v", model, args)
	}
}

func TestPlannerSplitterRunnerRejectsInvalidJSONWithoutGraphRepair(t *testing.T) {
	outDir := t.TempDir()
	runner := PlannerSplitterRunner{
		WorkDir: ".",
		Model:   "gpt-5.5",
		Timeout: time.Minute,
		CommandRunner: func(_ context.Context, inv PlannerCommandInvocation) error {
			return os.WriteFile(argValue(inv.Args, "--output-last-message"), []byte("not json"), 0o644)
		},
	}

	_, err := runner.Plan(context.Background(), PromptPlannerRequest{
		Prompt:         "Add invalid JSON handling",
		ProjectContext: strings.Repeat("project context for invalid JSON handling. ", 3),
		OutputDir:      outDir,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid planner JSON") {
		t.Fatalf("error = %v, want invalid planner JSON", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(outDir, "tasks", "*.json")); len(matches) != 0 {
		t.Fatalf("planner runner should not render or repair invalid JSON, got %v", matches)
	}
}

func validPlannerSplitterOutputJSON() string {
	return `{
  "needs_user_input": false,
  "questions": [],
  "summary": "Planner produced one executable task graph.",
  "risks": [{"title":"Verification risk","impact":"The worker must run tests.","mitigation":"Task includes verification.","severity":"P2"}],
  "task_graph": {
    "version": 1,
    "tasks": [{
      "id": "TASK-001",
      "repo": ".",
      "kind": "dev",
      "title": "Implement execute run",
      "objective": "Add the execute run orchestration.",
      "write_scope": ["cmd/execute.go"],
      "wave": 1,
      "acceptance_criteria": ["execute run emits JSON"],
      "verification_commands": ["go test ./cmd"],
      "risk_notes": ["CLI behavior changes"]
    }]
  }
}`
}

func argValue(args []string, flag string) string {
	for i, arg := range args {
		if arg == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
