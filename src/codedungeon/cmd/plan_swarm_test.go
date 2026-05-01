package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func TestPlanRunCommandPersistsPlanningSessionAndTasks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
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
	runID, err := store.CreateRun(&db.Run{Feature: "Swarm planning test", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	projectContext := filepath.Join(root, "project-context.md")
	writeFile(t, projectContext, strings.Repeat("Project context for CodeDungeon task planning with SQLite and provider adapters. ", 2))
	inputDir := filepath.Join(root, "fixtures")
	writePlanningFixtures(t, inputDir)
	outDir := filepath.Join(root, ".codedungeon", "task-planning")

	cmd := PlanCmd()
	cmd.SetArgs([]string{
		"run",
		"--prompt", "Add task planning swarm",
		"--mode", "full",
		"--project-context", projectContext,
		"--project-rules-status", "stale",
		"--project-rules-digest", "rules-digest",
		"--project-rules-read", "yes",
		"--runner", "files",
		"--input-dir", inputDir,
		"--out", outDir,
		"--legacy-phase4",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	store, err = db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var sessions int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM planning_sessions WHERE run_id=? AND status='COMPLETED'`, runID).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 {
		t.Fatalf("planning sessions = %d, want 1", sessions)
	}
	var tasks int
	if err := store.DB.QueryRow(`SELECT COUNT(1) FROM tasks WHERE run_id=?`, runID).Scan(&tasks); err != nil {
		t.Fatal(err)
	}
	if tasks != 2 {
		t.Fatalf("exported tasks = %d, want 2", tasks)
	}
	if _, err := os.Stat(filepath.Join(outDir, "MASTER.md")); err != nil {
		t.Fatalf("MASTER.md not rendered: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "plan", "MASTER.md")); err != nil {
		t.Fatalf("legacy MASTER.md not mirrored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "tasks", "swarm-planning-test", "codedungeon", "PLAN.md")); err != nil {
		t.Fatalf("legacy PLAN.md not mirrored: %v", err)
	}
	report, err := RenderObserveReport(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## Task Planning", "Status: COMPLETED", "Planning agents: 6"} {
		if !strings.Contains(report, want) {
			t.Fatalf("observe report missing %q:\n%s", want, report)
		}
	}
}

func TestPlanRunAutoRepairAndPromotePublishesBinaryReadyArtifacts(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
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
	if _, err := store.CreateRun(&db.Run{Feature: "Binary ready planning", Mode: "FULL", ProjectMode: "SINGLE"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	projectContext := filepath.Join(root, "project-context.md")
	writeFile(t, projectContext, strings.Repeat("Project context for CodeDungeon binary planning with SQLite and provider adapters. ", 2))
	inputDir := filepath.Join(root, "fixtures")
	writePlanningFixtures(t, inputDir)
	writePlanningJSON(t, filepath.Join(inputDir, "task_splitter.json"), taskplanning.AgentOutput{
		Role: "task_splitter", Provider: "test", Model: "fake", SessionID: "session-splitter",
		Confidence: 0.88, Summary: "Split into two repairable tasks.",
		TaskGraph: &taskplanning.TaskGraph{
			Version: 1,
			Tasks: []taskplanning.TaskSpec{
				{ID: "TASK-001", Repo: "codedungeon", Kind: "dev", Title: "Add schema", Objective: "Persist planning state.", WriteScope: []string{"internal/db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"schema migrated"}, VerificationCommands: []string{"go test ./internal/db"}},
				{ID: "TASK-002", Repo: "codedungeon", Kind: "dev", Title: "Use schema", Objective: "Use planning state.", WriteScope: []string{"internal/db/schema.sql"}, Wave: 1, AcceptanceCriteria: []string{"schema used"}, VerificationCommands: []string{"go test ./internal/db"}},
			},
		},
	})
	outDir := filepath.Join(root, ".codedungeon", "task-planning", "binary-ready")

	cmd := PlanCmd()
	cmd.SetArgs([]string{
		"run",
		"--prompt", "Binary ready planning",
		"--mode", "full",
		"--project-context", projectContext,
		"--project-rules-status", "stale",
		"--project-rules-digest", "rules-digest",
		"--project-rules-read", "yes",
		"--runner", "files",
		"--input-dir", inputDir,
		"--out", outDir,
		"--auto-repair",
		"--promote",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "plan", "MASTER.md")); err != nil {
		t.Fatalf("canonical MASTER.md not promoted: %v", err)
	}
	planPath := filepath.Join(root, ".codedungeon", "plan", "PLAN.md")
	if _, err := os.Stat(planPath); err != nil {
		t.Fatalf("canonical PLAN.md not promoted: %v", err)
	}
	taskPath := filepath.Join(root, ".codedungeon", "tasks", "task-001-add-schema.md")
	if _, err := os.Stat(taskPath); err != nil {
		t.Fatalf("canonical task file not promoted: %v", err)
	}
	graphBody, err := os.ReadFile(filepath.Join(outDir, "task-graph.json"))
	if err != nil {
		t.Fatal(err)
	}
	var graph taskplanning.TaskGraph
	if err := json.Unmarshal(graphBody, &graph); err != nil {
		t.Fatal(err)
	}
	if graph.Tasks[1].Wave != 2 || !containsString(graph.Tasks[1].DependsOn, "TASK-001") {
		t.Fatalf("promoted run did not repair graph: %+v", graph.Tasks)
	}
}

func TestPlanValidateCommandRejectsInvalidGraph(t *testing.T) {
	root := t.TempDir()
	graphPath := filepath.Join(root, "task-graph.json")
	writePlanningJSON(t, graphPath, taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-001", Repo: "repo", Title: "A", Objective: "A", DependsOn: []string{"TASK-002"}, WriteScope: []string{"a.go"}, Wave: 1, AcceptanceCriteria: []string{"a"}, VerificationCommands: []string{"go test ./..."}},
			{ID: "TASK-002", Repo: "repo", Title: "B", Objective: "B", DependsOn: []string{"TASK-001"}, WriteScope: []string{"b.go"}, Wave: 2, AcceptanceCriteria: []string{"b"}, VerificationCommands: []string{"go test ./..."}},
		},
	})

	cmd := PlanCmd()
	cmd.SetArgs([]string{"validate", "--task-graph", graphPath})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle validation error, got %v", err)
	}
}

func writePlanningFixtures(t *testing.T, dir string) {
	t.Helper()
	common := taskplanning.ProjectRulesEnvelope{Status: "stale", Digest: "rules-digest", Read: "yes"}
	for _, role := range []string{"planner_architect", "domain_planner", "qa_planner", "risk_planner"} {
		writePlanningJSON(t, filepath.Join(dir, role+".json"), taskplanning.AgentOutput{
			Role: role, Provider: "test", Model: "fake", SessionID: "session-" + role,
			Confidence: 0.8, Summary: role + " produced a concrete proposal.",
			ProjectRules: common,
			Proposals:    []taskplanning.Proposal{{Title: role + " proposal", Summary: "Concrete planning input."}},
		})
	}
	writePlanningJSON(t, filepath.Join(dir, "planning_evaluator.json"), taskplanning.AgentOutput{
		Role: "planning_evaluator", Provider: "test", Model: "fake", SessionID: "session-evaluator",
		Confidence: 0.9, Verdict: "PASS", Score: 0.92, Summary: "Planning is sufficient and has no material ambiguity.",
		ProjectRules: common,
	})
	writePlanningJSON(t, filepath.Join(dir, "task_splitter.json"), taskplanning.AgentOutput{
		Role: "task_splitter", Provider: "test", Model: "fake", SessionID: "session-splitter",
		Confidence: 0.88, Summary: "Split into two sequential tasks.", ProjectRules: common,
		TaskGraph: &taskplanning.TaskGraph{
			Version: 1,
			Tasks: []taskplanning.TaskSpec{
				{ID: "TASK-001", Repo: "codedungeon", Kind: "dev", Title: "Add schema", Objective: "Persist planning state.", WriteScope: []string{"internal/db/schema.sql"}, Wave: 1, ParallelGroup: "schema", OwnerRole: "backend", AcceptanceCriteria: []string{"schema migrated"}, VerificationCommands: []string{"go test ./internal/db"}},
				{ID: "TASK-002", Repo: "codedungeon", Kind: "dev", Title: "Add command", Objective: "Expose planning command.", DependsOn: []string{"TASK-001"}, WriteScope: []string{"cmd/plan.go"}, Wave: 2, ParallelGroup: "cli", OwnerRole: "backend", AcceptanceCriteria: []string{"command emits result"}, VerificationCommands: []string{"go test ./cmd"}},
			},
		},
	})
}

func writePlanningJSON(t *testing.T, path string, payload any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
