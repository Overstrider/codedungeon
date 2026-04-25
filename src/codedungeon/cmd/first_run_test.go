package cmd

import "testing"

func TestFirstRunDecisionPromptsInTTYWhenProjectMissingSetup(t *testing.T) {
	d := DecideFirstRun("codex", true, false, true, true)
	if !d.ShouldSetup {
		t.Fatalf("ShouldSetup = false, want true")
	}
	if d.Command != "codedungeon-codex setup" {
		t.Fatalf("Command = %q, want codedungeon-codex setup", d.Command)
	}
}

func TestFirstRunDecisionDeclineDoesNotSetup(t *testing.T) {
	d := DecideFirstRun("codex", true, false, true, false)
	if d.ShouldSetup {
		t.Fatalf("ShouldSetup = true, want false")
	}
	if d.Command != "codedungeon-codex setup" {
		t.Fatalf("Command = %q, want codedungeon-codex setup", d.Command)
	}
}

func TestFirstRunDecisionNonTTYPrintsYesCommandOnly(t *testing.T) {
	d := DecideFirstRun("codex", true, false, false, false)
	if d.ShouldSetup {
		t.Fatalf("ShouldSetup = true, want false")
	}
	if d.Command != "codedungeon-codex setup --yes" {
		t.Fatalf("Command = %q, want codedungeon-codex setup --yes", d.Command)
	}
}

func TestFirstRunDecisionNoopsOutsideGitOrInitializedProject(t *testing.T) {
	for _, tc := range []struct {
		name        string
		hasGit      bool
		initialized bool
	}{
		{name: "outside git", hasGit: false, initialized: false},
		{name: "initialized", hasGit: true, initialized: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := DecideFirstRun("codex", tc.hasGit, tc.initialized, true, true)
			if d.ShouldSetup || d.Command != "" {
				t.Fatalf("decision = %+v, want no-op", d)
			}
		})
	}
}
