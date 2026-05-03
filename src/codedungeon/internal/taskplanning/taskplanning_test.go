package taskplanning

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loldinis/codedungeon/internal/tooladapter"
)

func TestValidateTaskGraphRejectsCyclesAndUnsafeParallelWriteScopes(t *testing.T) {
	valid := TaskGraph{
		Version: 1,
		Tasks: []TaskSpec{
			{ID: "TASK-001", Repo: "api", Title: "Add schema", Objective: "Create storage", WriteScope: []string{"db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"schema exists"}, VerificationCommands: []string{"go test ./..."}},
			{ID: "TASK-002", Repo: "api", Title: "Use schema", Objective: "Wire storage", DependsOn: []string{"TASK-001"}, WriteScope: []string{"internal/store.go"}, Wave: 2, AcceptanceCriteria: []string{"store uses schema"}, VerificationCommands: []string{"go test ./..."}},
		},
	}
	if err := ValidateTaskGraph(valid); err != nil {
		t.Fatalf("valid graph rejected: %v", err)
	}

	cyclic := TaskGraph{
		Version: 1,
		Tasks: []TaskSpec{
			{ID: "TASK-001", Repo: "api", Title: "Add schema", Objective: "Create storage", DependsOn: []string{"TASK-002"}, WriteScope: []string{"db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"schema exists"}, VerificationCommands: []string{"go test ./..."}},
			{ID: "TASK-002", Repo: "api", Title: "Use schema", Objective: "Wire storage", DependsOn: []string{"TASK-001"}, WriteScope: []string{"internal/store.go"}, Wave: 2, AcceptanceCriteria: []string{"store uses schema"}, VerificationCommands: []string{"go test ./..."}},
		},
	}
	if err := ValidateTaskGraph(cyclic); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("cycle not rejected clearly: %v", err)
	}

	unsafeParallel := TaskGraph{
		Version: 1,
		Tasks: []TaskSpec{
			{ID: "TASK-001", Repo: "api", Title: "Add schema", Objective: "Create storage", WriteScope: []string{"db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"schema exists"}, VerificationCommands: []string{"go test ./..."}},
			{ID: "TASK-002", Repo: "api", Title: "Use schema", Objective: "Wire storage", WriteScope: []string{"db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"store uses schema"}, VerificationCommands: []string{"go test ./..."}},
		},
	}
	if err := ValidateTaskGraph(unsafeParallel); err == nil || !strings.Contains(err.Error(), "parallel write scope") {
		t.Fatalf("parallel write-scope conflict not rejected clearly: %v", err)
	}
}

func TestRepairTaskGraphSerializesConflictingWriteScopes(t *testing.T) {
	graph := TaskGraph{
		Version: 1,
		Tasks: []TaskSpec{
			{ID: "TASK-001", Repo: "api", Title: "Add schema", Objective: "Create storage", WriteScope: []string{"db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"schema exists"}, VerificationCommands: []string{"go test ./..."}},
			{ID: "TASK-002", Repo: "api", Title: "Use schema", Objective: "Wire storage", WriteScope: []string{"db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"store uses schema"}, VerificationCommands: []string{"go test ./..."}},
		},
	}

	repaired, actions, err := RepairTaskGraph(graph)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTaskGraph(repaired); err != nil {
		t.Fatalf("repaired graph is invalid: %v", err)
	}
	if len(actions) == 0 {
		t.Fatal("repair actions were not reported")
	}
	tasks := map[string]TaskSpec{}
	for _, task := range repaired.Tasks {
		tasks[task.ID] = task
	}
	if tasks["TASK-001"].Wave != 1 {
		t.Fatalf("TASK-001 wave = %d, want 1", tasks["TASK-001"].Wave)
	}
	if tasks["TASK-002"].Wave != 2 {
		t.Fatalf("TASK-002 wave = %d, want 2", tasks["TASK-002"].Wave)
	}
	if !containsString(tasks["TASK-002"].DependsOn, "TASK-001") {
		t.Fatalf("TASK-002 deps = %v, want TASK-001", tasks["TASK-002"].DependsOn)
	}
}

func TestExecuteAutoRepairPersistsRepairedTaskGraph(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "planning")
	conflictingGraph := TaskGraph{
		Version: 1,
		Tasks: []TaskSpec{
			{ID: "TASK-001", Repo: "codedungeon", Kind: "dev", Title: "Add schema", Objective: "Persist planning state.", WriteScope: []string{"internal/db/schema.sql"}, Wave: 1, ParallelGroup: "storage", OwnerRole: "backend", AcceptanceCriteria: []string{"schema migrated"}, VerificationCommands: []string{"go test ./internal/db"}},
			{ID: "TASK-002", Repo: "codedungeon", Kind: "dev", Title: "Update schema usage", Objective: "Use planning state.", WriteScope: []string{"internal/db/schema.sql"}, Wave: 1, ParallelGroup: "storage", OwnerRole: "backend", AcceptanceCriteria: []string{"schema used"}, VerificationCommands: []string{"go test ./internal/db"}},
		},
	}
	runner := &fakeRunner{outputs: map[string]AgentOutput{
		"planner_architect": {
			Role: "planner_architect", Provider: "test", Model: "fake", SessionID: "agent-1", Confidence: 0.82,
			Summary:   "Use SQLite-backed planning.",
			Proposals: []Proposal{{Title: "SQLite module", Summary: "Keep state in project DB."}},
		},
		"planning_evaluator": {
			Role: "planning_evaluator", Provider: "test", Model: "fake", SessionID: "agent-eval", Confidence: 0.9,
			Verdict: "PASS", Score: 0.91, Summary: "Plan covers prompt and has testable tasks.",
		},
		"task_splitter": {
			Role: "task_splitter", Provider: "test", Model: "fake", SessionID: "agent-split", Confidence: 0.88,
			Summary: "Generated conflicting graph for repair.", TaskGraph: &conflictingGraph,
		},
	}}

	result, err := Execute(context.Background(), Request{
		Prompt:          "Add swarm task planning",
		Mode:            "full",
		ProjectContext:  strings.Repeat("Project context with CLI, SQLite, provider adapters. ", 2),
		OutputDir:       outDir,
		Roles:           []string{"planner_architect"},
		HumanGatePolicy: HumanGateMaterialAmbiguity,
		ProjectRules:    ProjectRulesEnvelope{Status: "stale", Digest: "digest", Read: "yes"},
		AutoRepair:      true,
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if result.TaskGraph == nil {
		t.Fatal("missing repaired task graph")
	}
	if result.TaskGraph.Tasks[1].Wave != 2 || !containsString(result.TaskGraph.Tasks[1].DependsOn, "TASK-001") {
		t.Fatalf("graph was not repaired: %+v", result.TaskGraph.Tasks)
	}
	if result.Metadata["auto_repair_actions"] == nil {
		t.Fatalf("repair metadata missing: %+v", result.Metadata)
	}
}

func TestExecuteStopsBeforeSplitWhenEvaluatorNeedsUserInput(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "planning")
	runner := &fakeRunner{outputs: map[string]AgentOutput{
		"planner_architect": {
			Role: "planner_architect", Provider: "test", Model: "fake", SessionID: "agent-1", Confidence: 0.82,
			Summary:   "Proposed storage-first implementation.",
			Proposals: []Proposal{{Title: "Add storage", Summary: "Create persistent state before handlers."}},
		},
		"planning_evaluator": {
			Role: "planning_evaluator", Provider: "test", Model: "fake", SessionID: "agent-eval", Confidence: 0.9,
			Verdict:   "NEEDS_USER_INPUT",
			Summary:   "Architecture choice changes the task graph.",
			Questions: []Question{{Question: "Should state live in SQLite or JSON files?", Impact: "Changes schema and task ordering.", Material: true}},
		},
	}}

	result, err := Execute(context.Background(), Request{
		Prompt:          "Add persisted planning state",
		Mode:            "full",
		ProjectContext:  strings.Repeat("Project context with CLI, SQLite, provider adapters. ", 2),
		OutputDir:       outDir,
		Roles:           []string{"planner_architect"},
		HumanGatePolicy: HumanGateMaterialAmbiguity,
		ProjectRules:    ProjectRulesEnvelope{Status: "stale", Digest: "digest", Read: "yes"},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusNeedsUserInput || !result.NeedsUserInput {
		t.Fatalf("status = %s needs_user_input=%v", result.Status, result.NeedsUserInput)
	}
	if runner.called["task_splitter"] {
		t.Fatalf("task_splitter was called despite material user question")
	}
	for _, rel := range []string{"planning-request.json", "blackboard.jsonl", "evaluation.json", "planning-result.json"} {
		if _, err := os.Stat(filepath.Join(result.OutputDir, rel)); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
}

func TestExecuteBuildsValidatedTaskGraphAndRenderedTasks(t *testing.T) {
	outDir := filepath.Join(t.TempDir(), "planning")
	graph := TaskGraph{
		Version: 1,
		Tasks: []TaskSpec{
			{ID: "TASK-001", Repo: "codedungeon", Kind: "dev", Title: "Add planning schema", Objective: "Persist planning sessions.", WriteScope: []string{"internal/db/schema.sql"}, Wave: 1, ParallelGroup: "schema", OwnerRole: "backend", AcceptanceCriteria: []string{"planning tables exist"}, VerificationCommands: []string{"go test ./internal/db"}},
			{ID: "TASK-002", Repo: "codedungeon", Kind: "dev", Title: "Add CLI command", Objective: "Expose planning run.", DependsOn: []string{"TASK-001"}, WriteScope: []string{"cmd/plan.go"}, Wave: 2, ParallelGroup: "cli", OwnerRole: "backend", AcceptanceCriteria: []string{"plan run emits JSON"}, VerificationCommands: []string{"go test ./cmd"}},
		},
	}
	runner := &fakeRunner{outputs: map[string]AgentOutput{
		"planner_architect": {
			Role: "planner_architect", Provider: "test", Model: "fake", SessionID: "agent-1", Confidence: 0.82,
			Summary:   "Use SQLite-backed module.",
			Proposals: []Proposal{{Title: "SQLite module", Summary: "Keep state in project DB."}},
		},
		"planning_evaluator": {
			Role: "planning_evaluator", Provider: "test", Model: "fake", SessionID: "agent-eval", Confidence: 0.9,
			Verdict: "PASS", Score: 0.91, Summary: "Plan covers prompt and has testable tasks.",
		},
		"task_splitter": {
			Role: "task_splitter", Provider: "test", Model: "fake", SessionID: "agent-split", Confidence: 0.88,
			Summary: "Generated two-wave graph.", TaskGraph: &graph,
		},
	}}

	result, err := Execute(context.Background(), Request{
		Prompt:          "Add swarm task planning",
		Mode:            "full",
		ProjectContext:  strings.Repeat("Project context with CLI, SQLite, provider adapters. ", 2),
		OutputDir:       outDir,
		Roles:           []string{"planner_architect"},
		HumanGatePolicy: HumanGateMaterialAmbiguity,
		ProjectRules:    ProjectRulesEnvelope{Status: "stale", Digest: "digest", Read: "yes"},
	}, runner)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || result.TaskGraph == nil || len(result.TaskGraph.Tasks) != 2 {
		t.Fatalf("unexpected result: status=%s graph=%+v", result.Status, result.TaskGraph)
	}
	for _, rel := range []string{"task-graph.json", "MASTER.md", filepath.Join("tasks", "codedungeon", "TASK-001.md"), filepath.Join("tasks", "codedungeon", "TASK-001.json"), filepath.Join("tasks", "codedungeon", "TASK-002.md"), filepath.Join("tasks", "codedungeon", "TASK-002.json")} {
		if _, err := os.Stat(filepath.Join(result.OutputDir, rel)); err != nil {
			t.Fatalf("missing artifact %s: %v", rel, err)
		}
	}
}

func TestExecuteFailsWhenEvaluatorRejectsPlanningBeforeSplit(t *testing.T) {
	runner := &fakeRunner{outputs: map[string]AgentOutput{
		"planner_architect": {
			Role: "planner_architect", Provider: "test", Model: "fake", SessionID: "agent-1", Confidence: 0.82,
			Summary: "Proposed implementation with known gaps.",
		},
		"planning_evaluator": {
			Role: "planning_evaluator", Provider: "test", Model: "fake", SessionID: "agent-eval", Confidence: 0.9,
			Verdict: "FAIL", Summary: "Planner missed the database migration and verification strategy.",
		},
	}}
	_, err := Execute(context.Background(), Request{
		Prompt:          "Add persisted planning state",
		Mode:            "full",
		ProjectContext:  strings.Repeat("Project context with CLI, SQLite, provider adapters. ", 2),
		OutputDir:       filepath.Join(t.TempDir(), "planning"),
		Roles:           []string{"planner_architect"},
		HumanGatePolicy: HumanGateMaterialAmbiguity,
		ProjectRules:    ProjectRulesEnvelope{Status: "stale", Digest: "digest", Read: "yes"},
	}, runner)
	if err == nil || !strings.Contains(err.Error(), "planning evaluator failed") {
		t.Fatalf("expected evaluator failure, got %v", err)
	}
	if runner.called["task_splitter"] {
		t.Fatalf("task_splitter was called after evaluator failure")
	}
}

func TestCodexRunnerFailsFastWhenNetworkSandboxIsActive(t *testing.T) {
	t.Setenv("CODEX_SANDBOX_NETWORK_DISABLED", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	outputPath := filepath.Join(t.TempDir(), "agent-output.json")

	err := (CodexRunner{WorkDir: t.TempDir()}).RunPlanningAgent(ctx, AgentRequest{
		Role:       "planner_architect",
		SessionID:  "sandbox-test",
		Round:      1,
		OutputPath: outputPath,
		ProjectRules: ProjectRulesEnvelope{
			Status: "stale",
			Digest: "digest",
			Read:   "yes",
		},
	})
	if err == nil {
		t.Fatal("expected sandbox fail-fast error")
	}
	if !strings.Contains(err.Error(), "codex-runner-requires-unsandboxed-exec") ||
		!strings.Contains(err.Error(), "CODEX_SANDBOX_NETWORK_DISABLED=1") {
		t.Fatalf("sandbox error was not actionable: %v", err)
	}
	if _, statErr := os.Stat(outputPath); !os.IsNotExist(statErr) {
		t.Fatalf("runner should not create output when sandbox preflight fails, stat err=%v", statErr)
	}
}

func TestClaudeRunnerPinsProjectLocalSonnetModel(t *testing.T) {
	var captured tooladapter.Command
	runner := ClaudeRunner{
		WorkDir: "repo",
		Model:   "claude-sonnet-4-6",
		Runner: commandRunnerFunc(func(_ context.Context, cmd tooladapter.Command) (tooladapter.CommandResult, error) {
			captured = cmd
			return tooladapter.CommandResult{}, nil
		}),
	}

	err := runner.RunPlanningAgent(context.Background(), AgentRequest{
		Role:          "planner_architect",
		SessionID:     "phase-4",
		Round:         1,
		ContextPacket: "project context",
		OutputPath:    "out/planner_architect.json",
		ProjectRules:  ProjectRulesEnvelope{Status: "approved", Digest: "digest", Read: "yes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if captured.Name != "claude" {
		t.Fatalf("command name = %q, want claude", captured.Name)
	}
	for _, want := range []string{
		"--setting-sources", "project,local",
		"--strict-mcp-config",
		"--dangerously-skip-permissions",
		"--model", "claude-sonnet-4-6",
	} {
		if !containsString(captured.Args, want) {
			t.Fatalf("claude planning args missing %q: %+v", want, captured.Args)
		}
	}
	if containsString(captured.Args, "--fallback-model") {
		t.Fatalf("claude planning args must not include fallback model: %+v", captured.Args)
	}
	for _, want := range []string{
		"CLAUDE_CODE_SUBAGENT_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_OPUS_MODEL=claude-sonnet-4-6",
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=claude-sonnet-4-6",
	} {
		if !containsString(captured.Env, want) {
			t.Fatalf("claude planning env missing %q: %+v", want, captured.Env)
		}
	}
	if !strings.Contains(captured.Stdin, `"provider": "claude"`) ||
		!strings.Contains(captured.Stdin, `"model": "claude-sonnet-4-6"`) {
		t.Fatalf("planning prompt did not require Claude/Sonnet metadata:\n%s", captured.Stdin)
	}
}

type commandRunnerFunc func(context.Context, tooladapter.Command) (tooladapter.CommandResult, error)

func (f commandRunnerFunc) Run(ctx context.Context, cmd tooladapter.Command) (tooladapter.CommandResult, error) {
	return f(ctx, cmd)
}

type fakeRunner struct {
	outputs map[string]AgentOutput
	called  map[string]bool
}

func (r *fakeRunner) RunPlanningAgent(_ context.Context, req AgentRequest) error {
	if r.called == nil {
		r.called = map[string]bool{}
	}
	r.called[req.Role] = true
	out, ok := r.outputs[req.Role]
	if !ok {
		out = AgentOutput{
			Role: req.Role, Provider: "test", Model: "fake", SessionID: "agent-" + req.Role,
			Confidence: 0.75, Summary: req.Role + " completed without findings.",
		}
	}
	return writeJSONFile(req.OutputPath, out)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
