package cmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestTraceCommandsRecordAgentLifecycleForCurrentRun(t *testing.T) {
	root := setupGatedRun(t)

	start := TraceCmd()
	start.SetArgs([]string{
		"agent-start",
		"--phase", "5",
		"--role", "dev-worker",
		"--agent-type", "cd_dev_worker",
		"--agent-name", "forge worker",
		"--model", "gpt-5.5",
		"--reasoning-effort", "medium",
		"--task", ".codedungeon/tasks/TASK-001.md",
		"--input-summary", "implement telemetry",
	})
	if err := start.Execute(); err != nil {
		t.Fatal(err)
	}

	s := openTestStore(t, root)
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("AgentRuns returned %d rows, want 1", len(agents))
	}
	if agents[0].Status != "RUNNING" || agents[0].AgentType != "cd_dev_worker" {
		t.Fatalf("agent-start did not record running agent: %+v", agents[0])
	}
	s.Close()

	end := TraceCmd()
	end.SetArgs([]string{
		"agent-end",
		"--id", "1",
		"--status", "COMPLETED",
		"--summary", "implementation finished",
		"--artifact", ".codedungeon/tasks/TASK-001.md",
	})
	if err := end.Execute(); err != nil {
		t.Fatal(err)
	}

	s = openTestStore(t, root)
	defer s.Close()
	agents, err = s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if agents[0].Status != "COMPLETED" || agents[0].FinishedAt == 0 || agents[0].OutputSummary == "" {
		t.Fatalf("agent-end did not close agent run: %+v", agents[0])
	}
}

func TestObserveReportIncludesAgentTimelineAndWarnings(t *testing.T) {
	root := setupGatedRun(t)
	s, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		Phase:     "1",
		Role:      "architect",
		AgentType: "cd_architect_planner",
		AgentName: "architecture phase",
		Model:     "gpt-5.5",
	}); err != nil {
		t.Fatal(err)
	}
	closedID, err := s.StartAgentRun(db.AgentRun{
		RunID:     run.ID,
		Phase:     "5",
		Role:      "dev-worker",
		AgentType: "cd_dev_worker",
		AgentName: "forge worker",
		Model:     "gpt-5.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAgentRun(closedID, "COMPLETED", "done", ".codedungeon/tasks/TASK-001.md", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertRunEvent(db.RunEvent{
		RunID:  run.ID,
		Event:  "code_review_integrity_pass",
		Detail: ".codedungeon/code-review/review-result.json",
	}); err != nil {
		t.Fatal(err)
	}
	s.Close()

	report, err := RenderObserveReport(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"# CodeDungeon Observability Report",
		"cd_architect_planner",
		"cd_dev_worker",
		"Telemetry warnings",
		"open agent runs: 1",
		"Run Events",
		"code_review_integrity_pass",
	} {
		if !strings.Contains(report, want) {
			t.Fatalf("observe report missing %q:\n%s", want, report)
		}
	}
}

func TestRunnerAgentTelemetryHelpersRecordRootAgent(t *testing.T) {
	root := setupGatedRun(t)
	s := openTestStore(t, root)
	defer s.Close()
	run, err := s.CurrentRun()
	if err != nil {
		t.Fatal(err)
	}

	agentID, err := recordRunnerAgentStart(s, run.ID, "session-1", "full")
	if err != nil {
		t.Fatal(err)
	}
	if err := recordRunnerAgentEnd(s, run.ID, agentID, "session-1", "COMPLETED", "runner completed"); err != nil {
		t.Fatal(err)
	}

	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("AgentRuns returned %d rows, want 1", len(agents))
	}
	got := agents[0]
	if got.Role != "autonomous-runner" || got.AgentType != "codedungeon-runner" || got.Status != "COMPLETED" {
		t.Fatalf("runner telemetry not recorded correctly: %+v", got)
	}
}

func TestAutonomousChildPromptIncludesTelemetryContract(t *testing.T) {
	prompt := autonomousChildPrompt("full", "ship feature", "feat/ship-feature")
	for _, want := range []string{
		"codedungeon trace agent-start",
		"codedungeon trace agent-end",
		"Record every subagent",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("autonomous child prompt missing %q:\n%s", want, prompt)
		}
	}
}
