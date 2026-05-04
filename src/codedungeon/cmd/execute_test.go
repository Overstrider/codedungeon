package cmd

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/taskexec"
	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func TestExecuteTaskDryRunCreatesInspectableSession(t *testing.T) {
	root := setupExecuteRun(t)
	taskPath := filepath.Join(root, "task.json")
	writeJSONFile(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-001",
		Repo:                 ".",
		Title:                "Dry run executor",
		Objective:            "Render execution prompt without invoking provider.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"dry-run does not mutate code"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})
	projectContext := filepath.Join(root, "project-context.md")
	if err := os.WriteFile(projectContext, []byte(strings.Repeat("project context for execution dry run. ", 3)), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"task", "--task", taskPath, "--project-context", projectContext, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.LatestExecutionSession(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if session == nil || session.Status != "DRY_RUN" {
		t.Fatalf("session = %+v, want DRY_RUN", session)
	}
	if _, err := os.Stat(filepath.Join(session.OutputDir, "result.json")); err != nil {
		t.Fatalf("result.json missing: %v", err)
	}
}

func TestExecuteStatusRequiresKnownSession(t *testing.T) {
	root := setupExecuteRun(t)
	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"status", "--session", "missing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("execute status succeeded for missing session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("unexpected error: %v", err)
	}
	if root == "" {
		t.Fatal("setup root empty")
	}
}

func TestExecuteTaskRejectsImplicitResume(t *testing.T) {
	root := setupExecuteRun(t)
	taskPath := filepath.Join(root, "task.json")
	writeJSONFile(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-002",
		Repo:                 ".",
		Title:                "No implicit resume",
		Objective:            "Require explicit session id.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"--resume needs id"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"task", "--task", taskPath, "--project-context", strings.Repeat("context words for execution resume guard. ", 3), "--resume"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("execute task accepted --resume without id")
	}
}

func TestExecuteTaskDryRunInitializesEmptyDatabase(t *testing.T) {
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
	taskPath := filepath.Join(root, "task.json")
	writeJSONFile(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-DB-INIT",
		Repo:                 ".",
		Title:                "Init empty DB",
		Objective:            "Allow execute task to initialize an empty CodeDungeon DB.",
		WriteScope:           []string{"README.md"},
		AcceptanceCriteria:   []string{"dry-run succeeds"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"task", "--task", taskPath, "--project-context", strings.Repeat("project context for empty execute database. ", 3), "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".codedungeon", "execute", "sessions", "*", "result.json")); len(matches) != 1 {
		t.Fatalf("expected one dry-run result.json, got %v", matches)
	}
}

func TestExecutePlanDryRunRunsTaskContractsInOrder(t *testing.T) {
	root := setupExecuteRun(t)
	taskDir := filepath.Join(root, "tasks")
	writeJSONFile(t, filepath.Join(taskDir, "TASK-002.json"), taskplanning.TaskSpec{
		ID:                   "TASK-002",
		Repo:                 ".",
		Title:                "Second task",
		Objective:            "Render the second execution prompt.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"second task dry-runs"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})
	writeJSONFile(t, filepath.Join(taskDir, "TASK-001.json"), taskplanning.TaskSpec{
		ID:                   "TASK-001",
		Repo:                 ".",
		Title:                "First task",
		Objective:            "Render the first execution prompt.",
		WriteScope:           []string{"internal/taskexec"},
		AcceptanceCriteria:   []string{"first task dry-runs"},
		VerificationCommands: []string{"go test ./internal/taskexec"},
	})
	projectContext := filepath.Join(root, "project-context.md")
	if err := os.WriteFile(projectContext, []byte(strings.Repeat("project context for execution plan dry run. ", 3)), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"plan", "--tasks", taskDir, "--project-context", projectContext, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("execution sessions = %d, want 2", len(sessions))
	}
	if sessions[0].TaskID != "TASK-001" || sessions[1].TaskID != "TASK-002" {
		t.Fatalf("sessions not created in task order: %+v", sessions)
	}
}

func TestExecutePlanDryRunAcceptsTaskGraph(t *testing.T) {
	root := setupExecuteRun(t)
	graphPath := filepath.Join(root, "task-graph.json")
	writeJSONFile(t, graphPath, taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-001", Repo: ".", Title: "First task", Objective: "Render the first execution prompt.", WriteScope: []string{"internal/taskexec"}, Wave: 1, AcceptanceCriteria: []string{"first task dry-runs"}, VerificationCommands: []string{"go test ./internal/taskexec"}},
			{ID: "TASK-002", Repo: ".", Title: "Second task", Objective: "Render the second execution prompt.", DependsOn: []string{"TASK-001"}, WriteScope: []string{"internal/taskexec"}, Wave: 2, AcceptanceCriteria: []string{"second task dry-runs"}, VerificationCommands: []string{"go test ./internal/taskexec"}},
		},
	})
	projectContext := filepath.Join(root, "project-context.md")
	if err := os.WriteFile(projectContext, []byte(strings.Repeat("project context for execution graph dry run. ", 3)), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"plan", "--task-graph", graphPath, "--project-context", projectContext, "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("execution sessions = %d, want 2", len(sessions))
	}
}

func TestExecutePlanDryRunAcceptsBOMTaskGraph(t *testing.T) {
	root := setupExecuteRun(t)
	graphPath := filepath.Join(root, "task-graph.json")
	graph := `{
  "version": 1,
  "tasks": [{
    "id": "TASK-BOM",
    "repo": ".",
    "title": "BOM graph",
    "objective": "Load task graph JSON saved by PowerShell on Windows.",
    "write_scope": ["README.md"],
    "wave": 1,
    "acceptance_criteria": ["dry-run succeeds"],
    "verification_commands": ["go test ./cmd"]
  }]
}`
	if err := os.WriteFile(graphPath, append([]byte{0xEF, 0xBB, 0xBF}, []byte(graph)...), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"plan", "--task-graph", graphPath, "--project-context", strings.Repeat("project context for bom task graph dry run. ", 3), "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRunRejectsMissingAndConflictingInputs(t *testing.T) {
	setupExecuteRun(t)

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"run", "--dry-run"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "exactly one input") {
		t.Fatalf("missing input error = %v, want exactly one input", err)
	}

	cmd = ExecuteCmd()
	cmd.SetArgs([]string{"run", "--task", "task.json", "--prompt", "do it", "--dry-run"})
	err = cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "exactly one input") {
		t.Fatalf("conflicting input error = %v, want exactly one input", err)
	}
}

func TestExecuteRunInputErrorIncludesExecutionID(t *testing.T) {
	root := setupExecuteRun(t)

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{"run", "--dry-run"})
	out, err := executeCommandInDir(root, cmd)
	if err == nil {
		t.Fatal("execute run succeeded without input")
	}
	var payload struct {
		OK          bool   `json:"ok"`
		Status      string `json:"status"`
		ExecutionID string `json:"execution_id"`
		Error       string `json:"error"`
		Hint        string `json:"hint"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("invalid error JSON %q: %v", out, err)
	}
	if payload.OK {
		t.Fatalf("ok = true, want false")
	}
	if payload.Status != taskexec.StatusFailed {
		t.Fatalf("status = %q, want FAILED", payload.Status)
	}
	if !strings.HasPrefix(payload.ExecutionID, "exec-") {
		t.Fatalf("execution_id = %q, want exec-*", payload.ExecutionID)
	}
	if !strings.Contains(payload.Error, "exactly one input") {
		t.Fatalf("error = %q, want exactly one input", payload.Error)
	}
}

func TestExecuteRunReadyInputsDoNotInvokePromptPlanner(t *testing.T) {
	root := setupExecuteRun(t)
	taskPath := filepath.Join(root, "task.json")
	writeJSONFile(t, taskPath, taskplanning.TaskSpec{
		ID:                   "TASK-READY",
		Repo:                 ".",
		Title:                "Ready task",
		Objective:            "Run a ready task without planning.",
		WriteScope:           []string{"README.md"},
		AcceptanceCriteria:   []string{"dry-run succeeds"},
		VerificationCommands: []string{"go test ./cmd"},
	})

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{
		"run",
		"--task", taskPath,
		"--project-context", strings.Repeat("project context for ready task execution. ", 3),
		"--planner-runner", "files",
		"--planner-input-dir", filepath.Join(root, "missing-planner-fixtures"),
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].TaskID != "TASK-READY" {
		t.Fatalf("sessions = %+v, want only ready task", sessions)
	}

	taskDir := filepath.Join(root, "tasks-ready")
	writeJSONFile(t, filepath.Join(taskDir, "TASK-002.json"), taskplanning.TaskSpec{
		ID:                   "TASK-002",
		Repo:                 ".",
		Title:                "Second ready task",
		Objective:            "Run from a ready task directory.",
		WriteScope:           []string{"docs"},
		AcceptanceCriteria:   []string{"dry-run succeeds"},
		VerificationCommands: []string{"go test ./cmd"},
	})
	writeJSONFile(t, filepath.Join(taskDir, "TASK-001.json"), taskplanning.TaskSpec{
		ID:                   "TASK-001",
		Repo:                 ".",
		Title:                "First ready task",
		Objective:            "Run first from a ready task directory.",
		WriteScope:           []string{"README.md"},
		AcceptanceCriteria:   []string{"dry-run succeeds"},
		VerificationCommands: []string{"go test ./cmd"},
	})

	cmd = ExecuteCmd()
	cmd.SetArgs([]string{
		"run",
		"--tasks", taskDir,
		"--project-context", strings.Repeat("project context for ready tasks execution. ", 3),
		"--planner-runner", "files",
		"--planner-input-dir", filepath.Join(root, "missing-planner-fixtures"),
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	sessions, err = store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 || sessions[1].TaskID != "TASK-001" || sessions[2].TaskID != "TASK-002" {
		t.Fatalf("ready task directory sessions = %+v, want TASK-001 then TASK-002 after ready task", sessions)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".codedungeon", "execute", "task-contracts", "*")); len(matches) != 0 {
		t.Fatalf("ready inputs should not render generated task contracts: %v", matches)
	}
}

func TestExecuteRunTaskGraphRepairsAndRendersContractsUnderRunID(t *testing.T) {
	root := setupExecuteRun(t)
	graphPath := filepath.Join(root, "task-graph.json")
	writeJSONFile(t, graphPath, taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-001", Repo: ".", Title: "First", Objective: "Touch shared scope first.", WriteScope: []string{"README.md"}, Wave: 1, AcceptanceCriteria: []string{"first dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
			{ID: "TASK-002", Repo: ".", Title: "Second", Objective: "Touch shared scope second.", WriteScope: []string{"README.md"}, Wave: 1, AcceptanceCriteria: []string{"second dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
		},
	})
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{
		"run",
		"--task-graph", graphPath,
		"--project-context", strings.Repeat("project context for graph execution repair. ", 3),
		"--planner-runner", "files",
		"--planner-input-dir", filepath.Join(root, "missing-planner-fixtures"),
		"--dry-run",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	matches, _ := filepath.Glob(filepath.Join(root, ".codedungeon", "execute", "task-contracts", strconv.FormatInt(run.ID, 10), "exec-*", "TASK-002.json"))
	if len(matches) != 1 {
		t.Fatalf("expected one rendered TASK-002 contract under run/execution id, got %v", matches)
	}
	contractPath := matches[0]
	var task taskplanning.TaskSpec
	body, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatalf("expected rendered contract under run/execution id: %v", err)
	}
	if err := json.Unmarshal(body, &task); err != nil {
		t.Fatal(err)
	}
	if task.Wave != 2 || !containsString(task.DependsOn, "TASK-001") {
		t.Fatalf("contract was not repaired: %+v", task)
	}
}

func TestExecuteRunTaskGraphRendersContractsUnderExecutionID(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()

	graph := taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-001", Repo: ".", Title: "First", Objective: "Render first task.", WriteScope: []string{"README.md"}, Wave: 1, AcceptanceCriteria: []string{"first dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
			{ID: "TASK-002", Repo: ".", Title: "Second", Objective: "Render second task.", DependsOn: []string{"TASK-001"}, WriteScope: []string{"docs"}, Wave: 2, AcceptanceCriteria: []string{"second dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
		},
	}

	result, err := executeRunFlow(context.Background(), executeRunFlowRequest{
		Root:           root,
		RunID:          run.ID,
		Input:          executeRunInput{Kind: "task-graph", Value: writeTaskGraphFixture(t, root, "task-graph.json", graph)},
		ProjectContext: strings.Repeat("project context for execution id contract rendering. ", 3),
		DryRun:         true,
		Config:         defaultExecuteTestConfig(),
		Executor:       taskexec.FilesRunner{InputDir: root},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.HasPrefix(result.ExecutionID, "exec-") {
		t.Fatalf("execution_id = %q, want exec-*", result.ExecutionID)
	}
	wantDir := filepath.Join(root, ".codedungeon", "execute", "task-contracts", strconv.FormatInt(run.ID, 10), result.ExecutionID)
	if result.ContractsDir != wantDir {
		t.Fatalf("contracts_dir = %q, want %q", result.ContractsDir, wantDir)
	}
	if result.TaskGraphPath != filepath.Join(wantDir, "task-graph.json") {
		t.Fatalf("task_graph_path = %q, want task-graph under execution dir", result.TaskGraphPath)
	}
	if _, err := os.Stat(filepath.Join(wantDir, "TASK-001.json")); err != nil {
		t.Fatalf("execution-scoped TASK-001 contract missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "execute", "task-contracts", strconv.FormatInt(run.ID, 10), "TASK-001.json")); !os.IsNotExist(err) {
		t.Fatalf("contract leaked into run-level directory, err=%v", err)
	}
}

func TestExecuteRunPromptUsesSameExecutionIDForPlanningAndContracts(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	plannerDir := filepath.Join(root, "planner-fixtures")
	writePlannerGraphFixture(t, plannerDir)

	result, err := executeRunFlow(context.Background(), executeRunFlowRequest{
		Root:           root,
		RunID:          run.ID,
		Input:          executeRunInput{Kind: "prompt", Value: "Split and execute this prompt."},
		ProjectContext: strings.Repeat("project context for prompt planning execution id. ", 3),
		DryRun:         true,
		Config:         defaultExecuteTestConfig(),
		Executor:       taskexec.FilesRunner{InputDir: root},
		Planner:        taskplanning.PlannerSplitterFilesRunner{InputDir: plannerDir},
	})
	if err != nil {
		t.Fatal(err)
	}

	if result.ExecutionID == "" {
		t.Fatal("execution_id missing")
	}
	runLabel := strconv.FormatInt(run.ID, 10)
	plannerOutput := filepath.Join(root, ".codedungeon", "execute", "prompt-planning", runLabel, result.ExecutionID, "planner-splitter-output.json")
	if _, err := os.Stat(plannerOutput); err != nil {
		t.Fatalf("planner output missing under execution id: %v", err)
	}
	if !strings.Contains(result.ContractsDir, filepath.Join("task-contracts", runLabel, result.ExecutionID)) {
		t.Fatalf("contracts_dir = %q, want same execution id as planning", result.ContractsDir)
	}
}

func TestExecuteRunCreatesSeparateArtifactDirsForSameRun(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	firstGraph := taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-OLD", Repo: ".", Title: "Old", Objective: "Old task.", WriteScope: []string{"README.md"}, Wave: 1, AcceptanceCriteria: []string{"old dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
		},
	}
	secondGraph := taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-NEW", Repo: ".", Title: "New", Objective: "New task.", WriteScope: []string{"docs"}, Wave: 1, AcceptanceCriteria: []string{"new dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
		},
	}

	first, err := executeRunFlow(context.Background(), executeRunFlowRequest{
		Root:           root,
		RunID:          run.ID,
		Input:          executeRunInput{Kind: "task-graph", Value: writeTaskGraphFixture(t, root, "first-task-graph.json", firstGraph)},
		ProjectContext: strings.Repeat("project context for first isolated execution. ", 3),
		DryRun:         true,
		Config:         defaultExecuteTestConfig(),
		Executor:       taskexec.FilesRunner{InputDir: root},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := executeRunFlow(context.Background(), executeRunFlowRequest{
		Root:           root,
		RunID:          run.ID,
		Input:          executeRunInput{Kind: "task-graph", Value: writeTaskGraphFixture(t, root, "second-task-graph.json", secondGraph)},
		ProjectContext: strings.Repeat("project context for second isolated execution. ", 3),
		DryRun:         true,
		Config:         defaultExecuteTestConfig(),
		Executor:       taskexec.FilesRunner{InputDir: root},
	})
	if err != nil {
		t.Fatal(err)
	}

	if first.ExecutionID == "" || second.ExecutionID == "" || first.ExecutionID == second.ExecutionID {
		t.Fatalf("execution ids = %q and %q, want distinct non-empty ids", first.ExecutionID, second.ExecutionID)
	}
	if _, err := os.Stat(filepath.Join(first.ContractsDir, "TASK-OLD.json")); err != nil {
		t.Fatalf("first contract missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(first.ContractsDir, "TASK-NEW.json")); !os.IsNotExist(err) {
		t.Fatalf("second contract leaked into first dir, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(second.ContractsDir, "TASK-NEW.json")); err != nil {
		t.Fatalf("second contract missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(second.ContractsDir, "TASK-OLD.json")); !os.IsNotExist(err) {
		t.Fatalf("first contract leaked into second dir, err=%v", err)
	}
}

func TestExecuteRunRejectsContractFilenameCollisions(t *testing.T) {
	root := setupExecuteRun(t)
	store := openTestStore(t, root)
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	store.Close()
	graph := taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK/A", Repo: ".", Title: "Slash task", Objective: "Render slash task.", WriteScope: []string{"README.md"}, Wave: 1, AcceptanceCriteria: []string{"slash dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
			{ID: "TASK_A", Repo: ".", Title: "Underscore task", Objective: "Render underscore task.", DependsOn: []string{"TASK/A"}, WriteScope: []string{"docs"}, Wave: 2, AcceptanceCriteria: []string{"underscore dry-run"}, VerificationCommands: []string{"go test ./cmd"}},
		},
	}

	result, err := executeRunFlow(context.Background(), executeRunFlowRequest{
		Root:           root,
		RunID:          run.ID,
		Input:          executeRunInput{Kind: "task-graph", Value: writeTaskGraphFixture(t, root, "task-graph.json", graph)},
		ProjectContext: strings.Repeat("project context for contract collision detection. ", 3),
		DryRun:         true,
		Config:         defaultExecuteTestConfig(),
		Executor:       taskexec.FilesRunner{InputDir: root},
	})
	if err == nil {
		t.Fatalf("execute run succeeded despite colliding contract filenames: %+v", result)
	}
	if !strings.Contains(err.Error(), "contract filename collision") {
		t.Fatalf("error = %v, want contract filename collision", err)
	}
}

func TestExecuteRunPromptNeedsUserInputDoesNotExecute(t *testing.T) {
	root := setupExecuteRun(t)
	promptPath := filepath.Join(root, "prompt.txt")
	writeFile(t, promptPath, "Add a feature but ask a material question first.")
	plannerDir := filepath.Join(root, "planner-fixtures")
	writeFile(t, filepath.Join(plannerDir, "planner_splitter.json"), `{
  "needs_user_input": true,
  "questions": [{"question":"Which storage backend should be used?","impact":"Changes write scope and task order.","material":true}],
  "summary": "The prompt is materially ambiguous.",
  "risks": [{"title":"Wrong storage","impact":"The graph may target the wrong persistence layer.","mitigation":"Ask first.","severity":"P1"}]
}`)

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{
		"run",
		"--prompt-file", promptPath,
		"--project-context", strings.Repeat("project context for ambiguous prompt execution. ", 3),
		"--planner-runner", "files",
		"--planner-input-dir", plannerDir,
		"--runner", "files",
		"--input-dir", filepath.Join(root, "missing-execution-fixtures"),
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("execution sessions = %+v, want none", sessions)
	}
}

func TestExecuteRunPromptPlansContractsAndExecutesTasksInOrder(t *testing.T) {
	root := setupExecuteRun(t)
	writeFile(t, filepath.Join(root, "README.md"), "initial\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init")
	writeJSONFile(t, filepath.Join(root, ".ralphrc"), map[string]any{"auto_commit": false})
	plannerDir := filepath.Join(root, "planner-fixtures")
	writePlannerGraphFixture(t, plannerDir)
	execDir := filepath.Join(root, "exec-fixtures")
	writeJSONFile(t, filepath.Join(execDir, "execution-result.json"), map[string]any{
		"status":        "PASS",
		"summary":       "fixture worker passed",
		"changed_files": []string{},
	})

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{
		"run",
		"--prompt", "Add autonomous execute run",
		"--project-context", strings.Repeat("project context for planner execution e2e. ", 3),
		"--planner-runner", "files",
		"--planner-input-dir", plannerDir,
		"--runner", "files",
		"--input-dir", execDir,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 2 {
		t.Fatalf("execution sessions = %d, want 2", len(sessions))
	}
	if sessions[0].TaskID != "TASK-001" || sessions[1].TaskID != "TASK-002" {
		t.Fatalf("sessions not in task order: %+v", sessions)
	}
	matches, _ := filepath.Glob(filepath.Join(root, ".codedungeon", "execute", "task-contracts", strconv.FormatInt(run.ID, 10), "exec-*", "TASK-001.json"))
	if len(matches) != 1 {
		t.Fatalf("TASK-001 contract missing under run/execution id: %v", matches)
	}
	contractsDir := filepath.Dir(matches[0])
	if _, err := os.Stat(filepath.Join(contractsDir, "TASK-002.json")); err != nil {
		t.Fatalf("TASK-002 contract missing in same execution dir: %v", err)
	}
}

func TestExecuteRunPromptInvalidJSONFailsBeforeExecution(t *testing.T) {
	root := setupExecuteRun(t)
	plannerDir := filepath.Join(root, "planner-fixtures")
	writeFile(t, filepath.Join(plannerDir, "planner_splitter.json"), "not json")

	cmd := ExecuteCmd()
	cmd.SetArgs([]string{
		"run",
		"--prompt", "Add invalid planner handling",
		"--project-context", strings.Repeat("project context for invalid planner JSON. ", 3),
		"--planner-runner", "files",
		"--planner-input-dir", plannerDir,
		"--runner", "files",
		"--input-dir", filepath.Join(root, "missing-execution-fixtures"),
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid planner JSON") {
		t.Fatalf("error = %v, want invalid planner JSON", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(root, ".codedungeon", "execute", "task-contracts", "*")); len(matches) != 0 {
		t.Fatalf("contracts should not be rendered for invalid planner JSON: %v", matches)
	}
	store := openTestStore(t, root)
	defer store.Close()
	run, err := store.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := store.ExecutionSessions(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Fatalf("execution sessions = %+v, want none", sessions)
	}
}

func setupExecuteRun(t *testing.T) string {
	t.Helper()
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
	if _, err := store.CreateRun(&db.Run{Feature: "execute cli", Branch: "feat/execute-cli", Mode: "FULL", ProjectMode: "SINGLE"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	return root
}

func writePlannerGraphFixture(t *testing.T, dir string) {
	t.Helper()
	writeFile(t, filepath.Join(dir, "planner_splitter.json"), `{
  "needs_user_input": false,
  "questions": [],
  "summary": "Planner produced two ordered executable tasks.",
  "risks": [{"title":"Fixture risk","impact":"Verification must run.","mitigation":"Each task declares a command.","severity":"P2"}],
  "task_graph": {
    "version": 1,
    "tasks": [
      {
        "id": "TASK-001",
        "repo": ".",
        "kind": "dev",
        "title": "First planned task",
        "objective": "Execute the first planned task.",
        "write_scope": ["README.md"],
        "wave": 1,
        "acceptance_criteria": ["first task passes"],
        "verification_commands": ["echo ok"],
        "risk_notes": ["fixture"]
      },
      {
        "id": "TASK-002",
        "repo": ".",
        "kind": "dev",
        "title": "Second planned task",
        "objective": "Execute the second planned task after the first.",
        "write_scope": ["docs"],
        "depends_on": ["TASK-001"],
        "wave": 2,
        "acceptance_criteria": ["second task passes"],
        "verification_commands": ["echo ok"],
        "risk_notes": ["fixture"]
      }
    ]
  }
}`)
}

func writeExecuteJSON(t *testing.T, path string, payload any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(body, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTaskGraphFixture(t *testing.T, root, name string, graph taskplanning.TaskGraph) string {
	t.Helper()
	path := filepath.Join(root, name)
	writeJSONFile(t, path, graph)
	return path
}

func defaultExecuteTestConfig() taskexec.Config {
	cfg := taskexec.DefaultConfig()
	cfg.Runner = "files"
	cfg.AutoCommit = false
	cfg.AutoPush = false
	cfg.AutoTag = false
	cfg.MaxIterations = 1
	return cfg
}
