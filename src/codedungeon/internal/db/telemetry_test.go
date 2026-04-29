package db

import (
	"path/filepath"
	"testing"
)

func TestAgentTelemetryRecordsLifecycleAndEvents(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	runID, err := s.CreateRun(&Run{
		Feature:     "trace agents",
		Branch:      "feat/trace-agents",
		Mode:        "FULL",
		ProjectMode: "SINGLE",
	})
	if err != nil {
		t.Fatal(err)
	}

	agentID, err := s.StartAgentRun(AgentRun{
		RunID:           runID,
		SessionID:       "session-1",
		Phase:           "5",
		Role:            "dev-worker",
		AgentType:       "cd_dev_worker",
		AgentName:       "forge worker",
		Model:           "gpt-5.5",
		ReasoningEffort: "medium",
		TaskPath:        ".codedungeon/tasks/TASK-001.md",
		InputSummary:    "implement telemetry",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertAgentEvent(AgentEvent{
		RunID:      runID,
		AgentRunID: agentID,
		SessionID:  "session-1",
		Phase:      "5",
		Event:      "artifact",
		Detail:     ".codedungeon/tasks/TASK-001.md",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishAgentRun(agentID, "COMPLETED", "wrote implementation", ".codedungeon/tasks/TASK-001.md", ""); err != nil {
		t.Fatal(err)
	}

	agents, err := s.AgentRuns(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(agents) != 1 {
		t.Fatalf("AgentRuns returned %d rows, want 1", len(agents))
	}
	got := agents[0]
	if got.ID != agentID || got.Status != "COMPLETED" || got.FinishedAt == 0 {
		t.Fatalf("agent lifecycle not persisted: %+v", got)
	}
	if got.Phase != "5" || got.AgentType != "cd_dev_worker" || got.Model != "gpt-5.5" || got.ArtifactPath == "" {
		t.Fatalf("agent metadata not persisted: %+v", got)
	}

	events, err := s.AgentEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].AgentRunID != agentID || events[0].Event != "artifact" {
		t.Fatalf("agent events not persisted: %+v", events)
	}

	if _, err := s.InsertRunEvent(RunEvent{
		RunID:     runID,
		SessionID: "session-1",
		Event:     "code_review_integrity_pass",
		Detail:    ".codedungeon/code-review/review-result.json",
	}); err != nil {
		t.Fatal(err)
	}
	runEvents, err := s.RunEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	if len(runEvents) != 1 || runEvents[0].Event != "code_review_integrity_pass" || runEvents[0].SessionID != "session-1" {
		t.Fatalf("run events not persisted: %+v", runEvents)
	}
}

func TestFinishAgentRunRejectsUnknownID(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	if err := s.FinishAgentRun(404, "COMPLETED", "missing", "", ""); err == nil {
		t.Fatal("FinishAgentRun accepted an unknown agent run id")
	}
}
