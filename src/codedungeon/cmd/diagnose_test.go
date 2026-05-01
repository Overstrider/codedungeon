package cmd

import (
	"os"
	"testing"
)

func TestDiagnoseIdentifiesMissingBinaryOnlyArtifacts(t *testing.T) {
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

	report, err := buildDiagnostics(root)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("diagnostics should fail when core artifacts are missing: %+v", report)
	}
	for _, want := range []string{"database", "project_context", "planning_artifacts"} {
		if !diagnosticHasCheck(report, want) {
			t.Fatalf("diagnostics missing check %q: %+v", want, report.Checks)
		}
	}
	if len(report.NextCommands) == 0 {
		t.Fatalf("diagnostics should suggest next commands: %+v", report)
	}
}

func diagnosticHasCheck(report diagnosticReport, id string) bool {
	for _, check := range report.Checks {
		if check.ID == id {
			return true
		}
	}
	return false
}
