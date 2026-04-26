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
func (Codex) DBPath() string      { return filepath.Join(".codedungeon", "codedungeon.db") }
func (Codex) CommandsDir() string { return filepath.Join(".codedungeon", "commands") }
func (Codex) AgentsDir() string   { return filepath.Join(".codex", "agents") }
func (Codex) SkillsDir() string   { return filepath.Join(".agents", "skills") }
func (Codex) PhasesDir() string   { return filepath.Join(".codedungeon", "phases") }
func (Codex) TasksDir() string    { return filepath.Join(".codedungeon", "tasks") }
func (Codex) PlanDir() string     { return filepath.Join(".codedungeon", "plan") }
func (Codex) StateDir() string    { return filepath.Join(".codedungeon", "state") }
func (Codex) PlansDir() string    { return filepath.Join(".codedungeon", "plans") }
func (Codex) ReviewsDir() string  { return filepath.Join(".codedungeon", "reviews") }

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
	return ModelConfig{Reasoning: "gpt-5.5", ReasoningEffort: "xhigh", Fast: "gpt-5.5", FastEffort: "medium"}
}

func (Codex) ModelAlternatives() []ModelConfig {
	return []ModelConfig{
		{Reasoning: "gpt-5.5", ReasoningEffort: "xhigh", Fast: "gpt-5.5", FastEffort: "medium"},
		{Reasoning: "gpt-5.5", ReasoningEffort: "high", Fast: "gpt-5.5", FastEffort: "medium"},
		{Reasoning: "gpt-5.4", ReasoningEffort: "high", Fast: "gpt-5.4-mini", FastEffort: "medium"},
	}
}

func (Codex) RequiredCLIArgs() []string   { return nil }
func (Codex) ReviewCommentMarker() string { return "Codex Adversarial Code Review" }
func (Codex) SupportsThinking() bool      { return false }
