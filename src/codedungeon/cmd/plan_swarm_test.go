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

func TestObservePlanningStatusShowsRecoveredCompletion(t *testing.T) {
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
	runID, err := store.CreateRun(&db.Run{Feature: "Recovered planning", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, ".codedungeon", "task-planning", "recovered")
	if err := store.UpsertPlanningSession(db.PlanningSession{
		ID:              "plan-recovered",
		RunID:           runID,
		Mode:            "FULL",
		Prompt:          "Recovered planning",
		HumanGatePolicy: "ask",
		Status:          taskplanning.StatusFailed,
		OutputDir:       outDir,
		FailureMessage:  "first provider attempt failed",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRunEvent(db.RunEvent{RunID: runID, Event: "planning_failed", Detail: "first provider attempt failed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertPlanningEvaluation(db.PlanningEvaluation{
		SessionID: "plan-recovered",
		RunID:     runID,
		Verdict:   "PASS",
		Score:     0.95,
		FullJSON:  `{"verdict":"PASS"}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertPlanningTaskGraph(db.PlanningTaskGraph{
		SessionID: "plan-recovered",
		RunID:     runID,
		Version:   1,
		Status:    taskplanning.StatusCompleted,
		GraphJSON: `{"version":1,"tasks":[]}`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.InsertRunEvent(db.RunEvent{RunID: runID, Event: "planning_completed", Detail: "plan-recovered"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	report, err := RenderObserveReport(root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(report, "Status: COMPLETED (recovered from prior failure)") {
		t.Fatalf("observe report should show effective recovered planning status:\n%s", report)
	}
}

func TestEffectivePlanningStatusDoesNotRecoverAfterLaterFailure(t *testing.T) {
	status := effectivePlanningStatus(
		&db.PlanningSession{ID: "plan-recovered", Status: taskplanning.StatusFailed},
		[]db.PlanningEvaluation{{Verdict: "PASS", Score: 0.95}},
		[]db.PlanningTaskGraph{{Version: 1, Status: taskplanning.StatusCompleted}},
		[]db.RunEvent{
			{ID: 1, Event: "planning_failed", Detail: "first attempt failed"},
			{ID: 2, Event: "planning_recovered", Detail: "plan-recovered"},
			{ID: 3, Event: "planning_completed", Detail: "plan-recovered"},
			{ID: 4, Event: "planning_failed", Detail: "later attempt failed"},
		},
	)
	if status != taskplanning.StatusFailed {
		t.Fatalf("status = %q, want latest failure to remain visible", status)
	}
}

func TestDefaultPlanningRunnerNameFollowsProvider(t *testing.T) {
	cases := map[string]string{
		"claude":     "claude",
		"claude-ce":  "claude",
		"codex":      "codex",
		"codex-cli":  "codex",
		"unexpected": "codex",
		"":           "codex",
	}
	for providerName, want := range cases {
		if got := defaultPlanningRunnerName(providerName); got != want {
			t.Fatalf("defaultPlanningRunnerName(%q) = %q, want %q", providerName, got, want)
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

func TestPlanRunPromoteAllReposForMultiRepoGraph(t *testing.T) {
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
	if _, err := store.CreateRun(&db.Run{Feature: "Multi Repo Checkout", Mode: "FULL", ProjectMode: "MULTI"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	projectContext := filepath.Join(root, "project-context.md")
	writeFile(t, projectContext, strings.Repeat("Project context for backend portal app CodeDungeon planning with shared custody. ", 2))
	inputDir := filepath.Join(root, "fixtures")
	writePlanningFixtures(t, inputDir)
	writeMultiRepoTaskSplitterFixture(t, inputDir)
	outDir := filepath.Join(root, ".codedungeon", "task-planning", "multi-repo-checkout")
	writeFile(t, filepath.Join(root, ".codedungeon", "tasks", "task-999-existing-flat.md"), "keep me\n")

	cmd := PlanCmd()
	cmd.SetArgs([]string{
		"run",
		"--prompt", "Multi Repo Checkout",
		"--mode", "full",
		"--project-context", projectContext,
		"--project-rules-status", "approved",
		"--project-rules-digest", "rules-digest",
		"--project-rules-read", "yes",
		"--runner", "files",
		"--input-dir", inputDir,
		"--out", outDir,
		"--promote",
	})
	out := captureStdout(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatal(err)
		}
	})
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal plan run output: %v\n%s", err, out)
	}
	if payload["promotion_mode"] != "multi_repo_all" {
		t.Fatalf("promotion_mode = %v, want multi_repo_all\n%s", payload["promotion_mode"], out)
	}
	if len(payload["promoted_repos"].([]any)) != 3 {
		t.Fatalf("promoted_repos = %v, want backend portal app", payload["promoted_repos"])
	}

	assertFileExists(t, filepath.Join(root, ".codedungeon", "plan", "MASTER.md"))
	for _, tc := range []struct {
		repo string
		task string
	}{
		{"backend", "TASK-001.md"},
		{"portal", "TASK-002.md"},
		{"app", "TASK-003.md"},
	} {
		assertFileExists(t, filepath.Join(root, ".codedungeon", "tasks", "multi-repo-checkout", tc.repo, "PLAN.md"))
		assertFileExists(t, filepath.Join(root, ".codedungeon", "tasks", "multi-repo-checkout", tc.repo, tc.task))
	}
	if _, err := os.Stat(filepath.Join(root, ".codedungeon", "plan", "PLAN.md")); err == nil {
		t.Fatalf("multi-repo promotion must not overwrite flat PLAN.md")
	}
	assertFileExists(t, filepath.Join(root, ".codedungeon", "tasks", "task-999-existing-flat.md"))
}

func TestPlanPromoteMultiRepoFeatureAndExplicitRepoModes(t *testing.T) {
	graph := taskplanning.TaskGraph{
		Version: 1,
		Tasks: []taskplanning.TaskSpec{
			{ID: "TASK-001", Repo: "backend", Kind: "dev", Title: "Backend API", Objective: "Update backend.", WriteScope: []string{"backend/api.go"}, Wave: 1, AcceptanceCriteria: []string{"backend works"}, VerificationCommands: []string{"go test ./..."}},
			{ID: "TASK-002", Repo: "portal", Kind: "dev", Title: "Portal UI", Objective: "Update portal.", WriteScope: []string{"portal/app.tsx"}, Wave: 1, AcceptanceCriteria: []string{"portal works"}, VerificationCommands: []string{"npm test"}},
			{ID: "TASK-003", Repo: "app", Kind: "dev", Title: "Mobile App", Objective: "Update app.", WriteScope: []string{"app/Main.kt"}, Wave: 1, AcceptanceCriteria: []string{"app works"}, VerificationCommands: []string{"./gradlew test"}},
		},
	}

	t.Run("all repos with feature", func(t *testing.T) {
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
		outDir := filepath.Join(root, "multi-output")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := taskplanning.RenderArtifacts(outDir, graph, taskplanning.ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"}); err != nil {
			t.Fatal(err)
		}

		cmd := PlanCmd()
		cmd.SetArgs([]string{"promote", "--from", outDir, "--feature", "Tetoz Multi Repo"})
		out := captureStdout(t, func() {
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("unmarshal promote output: %v\n%s", err, out)
		}
		if payload["promotion_mode"] != "multi_repo_all" {
			t.Fatalf("promotion_mode = %v, want multi_repo_all", payload["promotion_mode"])
		}
		assertFileExists(t, filepath.Join(root, ".codedungeon", "tasks", "tetoz-multi-repo", "backend", "PLAN.md"))
		assertFileExists(t, filepath.Join(root, ".codedungeon", "tasks", "tetoz-multi-repo", "portal", "TASK-002.md"))
		if _, err := os.Stat(filepath.Join(root, ".codedungeon", "plan", "PLAN.md")); err == nil {
			t.Fatalf("multi-repo promote must not write flat PLAN.md")
		}
	})

	t.Run("explicit repo keeps single-repo promotion", func(t *testing.T) {
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
		outDir := filepath.Join(root, "multi-output")
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := taskplanning.RenderArtifacts(outDir, graph, taskplanning.ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"}); err != nil {
			t.Fatal(err)
		}

		cmd := PlanCmd()
		cmd.SetArgs([]string{"promote", "--from", outDir, "--repo", "backend"})
		out := captureStdout(t, func() {
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
		})
		var payload map[string]any
		if err := json.Unmarshal([]byte(out), &payload); err != nil {
			t.Fatalf("unmarshal promote output: %v\n%s", err, out)
		}
		if payload["promotion_mode"] != "single_repo" {
			t.Fatalf("promotion_mode = %v, want single_repo", payload["promotion_mode"])
		}
		assertFileExists(t, filepath.Join(root, ".codedungeon", "plan", "PLAN.md"))
		assertFileExists(t, filepath.Join(root, ".codedungeon", "tasks", "task-001-backend-api.md"))
		if _, err := os.Stat(filepath.Join(root, ".codedungeon", "tasks", "tetoz-multi-repo", "portal", "PLAN.md")); err == nil {
			t.Fatalf("explicit repo promotion should not promote all repos")
		}
	})
}

func TestPlanPromoteFailureEmitsCustodyStatus(t *testing.T) {
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

	cmd := PlanCmd()
	cmd.SetArgs([]string{"promote", "--from", filepath.Join(root, "missing-output"), "--feature", "broken feature"})
	var execErr error
	out := captureStdout(t, func() {
		execErr = cmd.Execute()
	})
	if execErr == nil {
		t.Fatal("expected promotion failure")
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("unmarshal failure output: %v\n%s", err, out)
	}
	if payload["custody_status"] != "NO_CODEDUNGEON_DELIVERY_CREATED" {
		t.Fatalf("custody_status = %v, want NO_CODEDUNGEON_DELIVERY_CREATED", payload["custody_status"])
	}
	commands, ok := payload["recovery_commands"].([]any)
	if !ok || len(commands) == 0 || !strings.Contains(commands[0].(string), "codedungeon plan promote --from") {
		t.Fatalf("recovery_commands = %#v, want exact promote command", payload["recovery_commands"])
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

func writeMultiRepoTaskSplitterFixture(t *testing.T, dir string) {
	t.Helper()
	common := taskplanning.ProjectRulesEnvelope{Status: "approved", Digest: "rules-digest", Read: "yes"}
	writePlanningJSON(t, filepath.Join(dir, "task_splitter.json"), taskplanning.AgentOutput{
		Role: "task_splitter", Provider: "test", Model: "fake", SessionID: "session-splitter",
		Confidence: 0.88, Summary: "Split into three repos.", ProjectRules: common,
		TaskGraph: &taskplanning.TaskGraph{
			Version: 1,
			Tasks: []taskplanning.TaskSpec{
				{ID: "TASK-001", Repo: "backend", Kind: "dev", Title: "Backend API", Objective: "Update backend API.", WriteScope: []string{"backend/api.go"}, Wave: 1, AcceptanceCriteria: []string{"backend works"}, VerificationCommands: []string{"go test ./..."}},
				{ID: "TASK-002", Repo: "portal", Kind: "dev", Title: "Portal UI", Objective: "Update portal UI.", WriteScope: []string{"portal/app.tsx"}, Wave: 1, AcceptanceCriteria: []string{"portal works"}, VerificationCommands: []string{"npm test"}},
				{ID: "TASK-003", Repo: "app", Kind: "dev", Title: "Mobile App", Objective: "Update mobile app.", WriteScope: []string{"app/Main.kt"}, Wave: 1, AcceptanceCriteria: []string{"app works"}, VerificationCommands: []string{"./gradlew test"}},
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
