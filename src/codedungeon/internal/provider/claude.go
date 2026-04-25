package provider

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Claude struct{}

func (Claude) Name() string            { return "claude" }
func (Claude) ConfigDir() string       { return ".claude" }
func (Claude) AgentConfigFile() string { return "CLAUDE.md" }

func (Claude) BinDir() string      { return filepath.Join(".claude", "bin") }
func (Claude) DBPath() string      { return filepath.Join(".claude", "codedungeon.db") }
func (Claude) CommandsDir() string { return filepath.Join(".claude", "commands") }
func (Claude) AgentsDir() string   { return filepath.Join(".claude", "agents") }
func (Claude) SkillsDir() string   { return filepath.Join(".claude", "skills") }
func (Claude) PhasesDir() string   { return filepath.Join(".claude", "phases") }
func (Claude) TasksDir() string    { return filepath.Join(".claude", "tasks") }
func (Claude) PlanDir() string     { return filepath.Join(".claude", "plan") }
func (Claude) StateDir() string    { return filepath.Join(".claude", "state") }
func (Claude) PlansDir() string    { return filepath.Join(".claude", "plans") }

func (Claude) PluginDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "plugins", "local", "codedungeon")
}

func (Claude) HasPluginSystem() bool { return true }

func (Claude) PluginManifest(version string) []byte {
	m := map[string]any{
		"name":        "codedungeon",
		"version":     version,
		"description": "Deterministic Go CLI for autonomous dev pipelines. SQLite (FTS5) state, embedded prompts, project-scoped.",
		"author":      map[string]string{"name": "loldinis"},
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	b = append(b, '\n')
	return b
}

func (Claude) HomeGuardPaths() []string {
	home, _ := os.UserHomeDir()
	paths := []string{"/root/.claude"}
	if home != "" {
		paths = append([]string{filepath.Join(home, ".claude")}, paths...)
	}
	return paths
}

func (Claude) DefaultModels() ModelConfig {
	return ModelConfig{Reasoning: "claude-opus-4-7", Fast: "claude-sonnet-4-6"}
}

func (Claude) ModelAlternatives() []ModelConfig {
	return []ModelConfig{
		{Reasoning: "claude-opus-4-7", Fast: "claude-sonnet-4-6"},
		{Reasoning: "claude-opus-4-7", Fast: "claude-haiku-4-5"},
		{Reasoning: "claude-sonnet-4-6", Fast: "claude-haiku-4-5"},
	}
}

func (Claude) ReviewCommentMarker() string { return "Claude Adversarial Code Review" }
func (Claude) SupportsThinking() bool      { return true }
