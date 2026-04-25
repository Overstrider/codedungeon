package provider

import (
	"os"
	"path/filepath"
)

type Codex struct{}

func (Codex) Name() string            { return "codex" }
func (Codex) ConfigDir() string       { return ".codex" }
func (Codex) AgentConfigFile() string { return "AGENTS.md" }

func (Codex) BinDir() string      { return filepath.Join(".codex", "bin") }
func (Codex) DBPath() string      { return filepath.Join(".codex", "codedungeon.db") }
func (Codex) CommandsDir() string { return filepath.Join(".codex", "commands") }
func (Codex) AgentsDir() string   { return filepath.Join(".codex", "agents") }
func (Codex) SkillsDir() string   { return filepath.Join(".agents", "skills") }
func (Codex) PhasesDir() string   { return filepath.Join(".codex", "phases") }
func (Codex) TasksDir() string    { return filepath.Join(".codex", "tasks") }
func (Codex) PlanDir() string     { return filepath.Join(".codex", "plan") }
func (Codex) StateDir() string    { return filepath.Join(".codex", "state") }
func (Codex) PlansDir() string    { return filepath.Join(".codex", "plans") }

func (Codex) PluginDir() string            { return "" }
func (Codex) PluginManifest(string) []byte { return nil }
func (Codex) HasPluginSystem() bool        { return false }

func (Codex) HomeGuardPaths() []string {
	home, _ := os.UserHomeDir()
	paths := []string{}
	if home != "" {
		paths = append(paths, filepath.Join(home, ".codex"))
	}
	return paths
}

func (Codex) DefaultModels() ModelConfig {
	return ModelConfig{Reasoning: "gpt-5.2", Fast: "gpt-5.4-mini"}
}

func (Codex) ModelAlternatives() []ModelConfig {
	return []ModelConfig{
		{Reasoning: "gpt-5.2", Fast: "gpt-5.4-mini"},
		{Reasoning: "gpt-5.4", Fast: "gpt-5.4-mini"},
		{Reasoning: "gpt-5.2", Fast: "gpt-5.3-codex-spark"},
	}
}

func (Codex) ReviewCommentMarker() string { return "Codex Adversarial Code Review" }
func (Codex) SupportsThinking() bool      { return false }
