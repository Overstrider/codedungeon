package prompts

import (
	"strings"
	"testing"
)

func TestClaudeArtifactsKeepClaudeInstallPaths(t *testing.T) {
	arts, err := ArtifactsFor("claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) == 0 {
		t.Fatal("expected claude artifacts")
	}
	for _, a := range arts {
		if a.Provider != "claude" {
			t.Fatalf("provider = %q, want claude", a.Provider)
		}
		if a.PackID != "codedungeon-claude" {
			t.Fatalf("pack id = %q, want codedungeon-claude", a.PackID)
		}
		if !strings.HasPrefix(a.InstallPath, ".claude/") {
			t.Fatalf("install path %q should stay under .claude/", a.InstallPath)
		}
	}
}

func TestClaudeLegacyPromptAliasesUseCanonicalPack(t *testing.T) {
	for _, alias := range []string{"claude-code", "claude-ce"} {
		arts, err := ArtifactsFor(alias)
		if err != nil {
			t.Fatal(err)
		}
		if len(arts) == 0 {
			t.Fatalf("expected artifacts for alias %s", alias)
		}
		if arts[0].Provider != "claude" {
			t.Fatalf("alias %s provider = %q, want claude", alias, arts[0].Provider)
		}
	}
}

func TestCodexArtifactsAreProviderNative(t *testing.T) {
	arts, err := ArtifactsFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"AGENTS.md":                                     false,
		".codex/config.toml":                            false,
		".codex/agents/cd_dev_worker.toml":              false,
		".codex/commands/codedungeon-dev-cycle.md":      false,
		".codex/phases/forge-execution.md":              false,
		".agents/skills/codedungeon-flow/SKILL.md":      false,
		".agents/skills/backend-specialist/SKILL.md":    false,
		".agents/skills/codedungeon-dev-cycle/SKILL.md": false,
		".agents/skills/minidungeon/SKILL.md":           false,
		".agents/skills/code-review/SKILL.md":           false,
		".agents/skills/codedungeon-test-loop/SKILL.md": false,
		".agents/skills/cleanup-tasks/SKILL.md":         false,
	}
	for _, a := range arts {
		if a.Provider != "codex" {
			t.Fatalf("provider = %q, want codex", a.Provider)
		}
		if strings.HasPrefix(a.InstallPath, ".codex/skills/") {
			t.Fatalf("codex skills must install under .agents/skills, got %q", a.InstallPath)
		}
		if _, ok := want[a.InstallPath]; ok {
			want[a.InstallPath] = true
		}
	}
	for path, seen := range want {
		if !seen {
			t.Fatalf("missing codex artifact install path %q", path)
		}
	}
}

func TestCodexAgentsUseOfficialTomlSchema(t *testing.T) {
	arts, err := ArtifactsFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, a := range arts {
		if a.Kind != "agent" {
			continue
		}
		count++
		body := string(a.Content)
		if strings.HasPrefix(body, "\ufeff") {
			t.Fatalf("%s starts with UTF-8 BOM; Codex agent TOML should be plain UTF-8", a.RelPath)
		}
		for _, required := range []string{"name = ", "description = ", "developer_instructions = "} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing %q", a.RelPath, required)
			}
		}
		if strings.Contains(body, "\nprompt = ") {
			t.Fatalf("%s uses prompt field; Codex custom agents require developer_instructions", a.RelPath)
		}
	}
	if count == 0 {
		t.Fatal("expected codex agents")
	}
}

func TestCodexCommandSkillsArePrimaryWorkflowSurface(t *testing.T) {
	for _, name := range []string{
		"codedungeon-dev-cycle",
		"minidungeon",
		"code-review",
		"codedungeon-test-loop",
		"cleanup-tasks",
	} {
		raw, err := GetRawFor("codex", "skills/"+name+"/SKILL.md")
		if err != nil {
			t.Fatalf("missing command skill %s: %v", name, err)
		}
		body := string(raw)
		if !strings.Contains(body, "name: "+name) {
			t.Fatalf("skill %s missing matching frontmatter name", name)
		}
		if strings.Contains(body, "slash command") {
			t.Fatalf("skill %s should not describe itself as a slash command", name)
		}
	}
}

func TestCodexPromptListIsNamespaced(t *testing.T) {
	names, err := ListFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("expected codex prompt names")
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "codex:") {
			t.Fatalf("codex prompt name %q should be namespaced", name)
		}
	}
	body, err := GetFor("codex", "codex:caveman-ultra")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "compact") {
		t.Fatalf("unexpected codex caveman prompt body: %q", body)
	}
}

func TestCodexPromptListExcludesInstallOnlyArtifacts(t *testing.T) {
	names, err := ListFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		switch name {
		case "codex:config", "codex:AGENTS":
			t.Fatalf("install-only artifact %q should not be seeded as a prompt", name)
		}
	}
}

func TestCodexWorkflowPromptsUseProjectLocalBinary(t *testing.T) {
	for _, rel := range []string{
		"AGENTS.md",
		"skills/codedungeon-cli/SKILL.md",
		"skills/codedungeon-dev-cycle/SKILL.md",
		"commands/codedungeon-dev-cycle.md",
	} {
		raw, err := GetRawFor("codex", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(raw)
		if !strings.Contains(body, "./.codex/bin/codedungeon") {
			t.Fatalf("%s should tell Codex to use the project-local binary", rel)
		}
	}
}
