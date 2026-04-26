package prompts

import (
	"strings"
	"testing"
)

func TestClaudeArtifactsSplitNativeAndRuntimeInstallPaths(t *testing.T) {
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
		if strings.HasPrefix(a.RelPath, "phases/") || strings.HasPrefix(a.RelPath, "commands/") {
			if !strings.HasPrefix(a.InstallPath, ".codedungeon/") {
				t.Fatalf("editable install path %q should live under .codedungeon/", a.InstallPath)
			}
			continue
		}
		if strings.HasPrefix(a.RelPath, "command-wrappers/") {
			if !strings.HasPrefix(a.InstallPath, ".claude/commands/") || a.Kind != "command-wrapper" {
				t.Fatalf("wrapper install path/kind = %q/%q, want .claude/commands + command-wrapper", a.InstallPath, a.Kind)
			}
			continue
		}
		if !strings.HasPrefix(a.InstallPath, ".claude/") {
			t.Fatalf("native install path %q should stay under .claude/", a.InstallPath)
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

func TestClaudeSpawnArtifactsRequireDangerouslySkipPermissions(t *testing.T) {
	arts, err := ArtifactsFor("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range arts {
		if !isClaudeSpawnSurface(a.RelPath) {
			continue
		}
		body := string(a.Content)
		if !mentionsClaudeSpawn(body) {
			continue
		}
		if !strings.Contains(body, "--dangerously-skip-permissions") {
			t.Fatalf("%s mentions Claude spawn flow but does not require --dangerously-skip-permissions", a.RelPath)
		}
	}
}

func isClaudeSpawnSurface(rel string) bool {
	return strings.HasPrefix(rel, "commands/") ||
		strings.HasPrefix(rel, "phases/") ||
		rel == "skills/summoning-circle-spawn/SKILL.md"
}

func mentionsClaudeSpawn(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "spawn") ||
		strings.Contains(body, "Task tool") ||
		strings.Contains(body, "Task calls") ||
		strings.Contains(body, "general-purpose")
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
		".codedungeon/commands/main-quest.md":           false,
		".codedungeon/commands/side-quest.md":           false,
		".codedungeon/commands/one-shot.md":             false,
		".codedungeon/phases/forge-execution.md":        false,
		".agents/skills/codedungeon-flow/SKILL.md":      false,
		".agents/skills/backend-specialist/SKILL.md":    false,
		".agents/skills/main-quest/SKILL.md":            false,
		".agents/skills/side-quest/SKILL.md":            false,
		".agents/skills/one-shot/SKILL.md":              false,
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
		if strings.HasPrefix(a.RelPath, "phases/") && !strings.HasPrefix(a.InstallPath, ".codedungeon/phases/") {
			t.Fatalf("codex phases must install under .codedungeon/phases, got %q", a.InstallPath)
		}
		if strings.HasPrefix(a.RelPath, "commands/") && !strings.HasPrefix(a.InstallPath, ".codedungeon/commands/") {
			t.Fatalf("codex commands must install under .codedungeon/commands, got %q", a.InstallPath)
		}
		if strings.HasPrefix(a.InstallPath, ".codex/commands/") {
			t.Fatalf("codex commands must not install under .codex/commands, got %q", a.InstallPath)
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
		for _, required := range []string{"Role:", "Working mode:", "Return:"} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing compact instruction section %q", a.RelPath, required)
			}
		}
		for _, forbidden := range []string{
			"max_thinking_tokens",
			"--dangerously-skip-permissions",
			"model = ",
			"model_reasoning_effort",
			"sandbox_mode",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains provider/runtime setting %q that should stay outside Codex agent TOML", a.RelPath, forbidden)
			}
		}
	}
	if count == 0 {
		t.Fatal("expected codex agents")
	}
}

func TestCodexConfigEnablesCustomAgentSpawning(t *testing.T) {
	raw, err := GetRawFor("codex", "config.toml")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, required := range []string{
		"[features]",
		"multi_agent_v2 = true",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("codex config missing %q:\n%s", required, body)
		}
	}
	for _, forbidden := range []string{
		"max_threads",
		"max_depth",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("codex config contains %q, which current Codex rejects with multi_agent_v2 enabled:\n%s", forbidden, body)
		}
	}
}

func TestCodexCommandSkillsArePrimaryWorkflowSurface(t *testing.T) {
	for _, name := range []string{
		"main-quest",
		"side-quest",
		"one-shot",
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
		"skills/main-quest/SKILL.md",
		"commands/main-quest.md",
		"skills/one-shot/SKILL.md",
		"commands/one-shot.md",
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
