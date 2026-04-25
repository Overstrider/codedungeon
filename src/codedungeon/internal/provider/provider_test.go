package provider

import (
	"os"
	"path/filepath"
	"testing"
)

func resetProviderForTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		mu.Lock()
		current = nil
		DefaultProvider = ""
		mu.Unlock()
	})
	mu.Lock()
	current = nil
	mu.Unlock()
}

func TestDetectDefaultsToClaude(t *testing.T) {
	t.Setenv("CODEDUNGEON_PROVIDER", "")
	DefaultProvider = ""
	resetProviderForTest(t)

	p := Detect()

	if p.Name() != "claude" {
		t.Fatalf("provider = %q, want claude", p.Name())
	}
	if p.AgentConfigFile() != "CLAUDE.md" {
		t.Fatalf("agent config = %q, want CLAUDE.md", p.AgentConfigFile())
	}
	if p.CommandsDir() != filepath.Join(".claude", "commands") {
		t.Fatalf("commands dir = %q", p.CommandsDir())
	}
	if !p.HasPluginSystem() {
		t.Fatalf("claude should keep plugin system enabled")
	}
}

func TestDetectUsesBuildTimeDefaultProvider(t *testing.T) {
	t.Setenv("CODEDUNGEON_PROVIDER", "")
	DefaultProvider = "codex"
	resetProviderForTest(t)

	p := Detect()

	if p.Name() != "codex" {
		t.Fatalf("provider = %q, want codex", p.Name())
	}
}

func TestDetectEnvOverridesBuildTimeDefaultProvider(t *testing.T) {
	t.Setenv("CODEDUNGEON_PROVIDER", "claude")
	DefaultProvider = "codex"
	resetProviderForTest(t)

	p := Detect()

	if p.Name() != "claude" {
		t.Fatalf("provider = %q, want claude", p.Name())
	}
}

func TestDetectClaudeLegacyAliasesNormalizeToClaude(t *testing.T) {
	for _, alias := range []string{"claude-code", "claude-ce"} {
		t.Run(alias, func(t *testing.T) {
			t.Setenv("CODEDUNGEON_PROVIDER", alias)
			resetProviderForTest(t)

			p := Detect()

			if p.Name() != "claude" {
				t.Fatalf("provider = %q, want claude", p.Name())
			}
		})
	}
}

func TestDetectCodexProvider(t *testing.T) {
	t.Setenv("CODEDUNGEON_PROVIDER", "codex")
	resetProviderForTest(t)

	p := Detect()

	if p.Name() != "codex" {
		t.Fatalf("provider = %q, want codex", p.Name())
	}
	if p.ConfigDir() != ".codex" {
		t.Fatalf("config dir = %q, want .codex", p.ConfigDir())
	}
	if p.AgentConfigFile() != "AGENTS.md" {
		t.Fatalf("agent config = %q, want AGENTS.md", p.AgentConfigFile())
	}
	if p.CommandsDir() != filepath.Join(".codex", "commands") {
		t.Fatalf("commands dir = %q", p.CommandsDir())
	}
	if p.AgentsDir() != filepath.Join(".codex", "agents") {
		t.Fatalf("agents dir = %q", p.AgentsDir())
	}
	if p.SkillsDir() != filepath.Join(".agents", "skills") {
		t.Fatalf("skills dir = %q", p.SkillsDir())
	}
	if p.HasPluginSystem() {
		t.Fatalf("codex provider should not use Claude plugin install flow")
	}
	if p.SupportsThinking() {
		t.Fatalf("codex spawn prompts should not emit Claude max_thinking_tokens")
	}
}

func TestCodexHomeGuardPaths(t *testing.T) {
	t.Setenv("CODEDUNGEON_PROVIDER", "codex")
	resetProviderForTest(t)

	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home unavailable")
	}

	guards := Detect().HomeGuardPaths()
	want := filepath.Join(home, ".codex")
	for _, got := range guards {
		if got == want {
			return
		}
	}
	t.Fatalf("codex guards = %#v, want %q", guards, want)
}

func TestCodexDefaultModelsIncludeReasoningEfforts(t *testing.T) {
	cfg := Codex{}.DefaultModels()
	if cfg.Reasoning != "gpt-5.5" || cfg.ReasoningEffort != "xhigh" {
		t.Fatalf("codex reasoning defaults = %q/%q, want gpt-5.5/xhigh", cfg.Reasoning, cfg.ReasoningEffort)
	}
	if cfg.Fast != "gpt-5.5" || cfg.FastEffort != "medium" {
		t.Fatalf("codex fast defaults = %q/%q, want gpt-5.5/medium", cfg.Fast, cfg.FastEffort)
	}
}

func TestClaudeDefaultModelsDoNotSetReasoningEfforts(t *testing.T) {
	cfg := Claude{}.DefaultModels()
	if cfg.ReasoningEffort != "" || cfg.FastEffort != "" {
		t.Fatalf("claude efforts = %q/%q, want empty", cfg.ReasoningEffort, cfg.FastEffort)
	}
}
