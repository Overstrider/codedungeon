package db

import (
	"path/filepath"
	"testing"
)

func TestStorePersistsQASessionChecksDependenciesAndFindings(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	runID, err := s.CreateRun(&Run{Feature: "qa persistence", Branch: "main", Mode: "FULL", ProjectMode: "SINGLE"})
	if err != nil {
		t.Fatal(err)
	}

	session := QASession{
		ID:          "qa-session-1",
		RunID:       runID,
		ExecutionID: "exec-1",
		Entrypoint:  "workflow",
		Mode:        "auto",
		Status:      "PASS",
		Root:        "E:/repo",
		PlanPath:    ".codedungeon/qa/sessions/qa-session-1/plan.json",
		EvidenceDir: ".codedungeon/qa/sessions/qa-session-1",
	}
	if err := s.UpsertQASession(session); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertQACheck(QACheck{
		ID:         "check-1",
		SessionID:  session.ID,
		Kind:       "command",
		Name:       "go test",
		Status:     "PASS",
		Command:    "go test ./...",
		CWD:        ".",
		ExitCode:   0,
		DurationMs: 42,
		LogPath:    ".codedungeon/qa/sessions/qa-session-1/logs/check-1.log",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertQADependency(QADependency{
		ID:          "dep-1",
		SessionID:   session.ID,
		Name:        "playwright",
		Required:    true,
		Status:      "missing",
		InstallHint: "npm i -D @playwright/test && npx playwright install --with-deps",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.InsertQAFinding(QAFinding{
		ID:           "finding-1",
		SessionID:    session.ID,
		Severity:     "blocking",
		Category:     "dependency_missing",
		Title:        "Playwright missing",
		Detail:       "E2E requires Playwright",
		EvidencePath: ".codedungeon/qa/sessions/qa-session-1/preflight.json",
	}); err != nil {
		t.Fatal(err)
	}

	latest, err := s.LatestQASession(runID)
	if err != nil {
		t.Fatal(err)
	}
	if latest == nil || latest.ID != session.ID || latest.Entrypoint != "workflow" || latest.Status != "PASS" {
		t.Fatalf("latest session = %+v", latest)
	}
	checks, err := s.QAChecks(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 1 || checks[0].Command != "go test ./..." {
		t.Fatalf("checks = %+v", checks)
	}
	deps, err := s.QADependencies(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || !deps[0].Required || deps[0].Status != "missing" {
		t.Fatalf("dependencies = %+v", deps)
	}
	findings, err := s.QAFindings(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 1 || findings[0].Category != "dependency_missing" {
		t.Fatalf("findings = %+v", findings)
	}
}
