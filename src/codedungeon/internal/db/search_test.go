package db

import (
	"path/filepath"
	"testing"
)

func TestSearchTreatsHyphenatedInputAsLiteralFallback(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertPlanningSession(PlanningSession{
		ID:                   "search-session",
		Mode:                 "FULL",
		Prompt:               "search provider-neutral planning evidence",
		PromptSHA256:         "prompt",
		ProjectContextSHA256: "context",
		HumanGatePolicy:      "never",
		Status:               "COMPLETED",
		OutputDir:            ".codedungeon/task-planning/search-session",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.InsertPlanningBlackboard(PlanningBlackboardEntry{
		SessionID: "search-session",
		Role:      "planner_architect",
		Kind:      "constraint",
		Title:     "Provider neutrality",
		Summary:   "Keep the provider-neutral runner isolated from provider-specific artifacts.",
		FullJSON:  `{"title":"Provider neutrality"}`,
	}); err != nil {
		t.Fatal(err)
	}

	hits, err := s.Search("planning_blackboard", "provider-neutral", 10)
	if err != nil {
		t.Fatalf("hyphenated search returned error: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("hyphenated search returned no hits")
	}
}
