package provider

import (
	"os"
	"sync"
)

// DefaultProvider is set by release builds via -ldflags. It lets provider-
// specific binaries select their provider without requiring environment vars.
var DefaultProvider string

type Provider interface {
	Name() string
	ConfigDir() string
	AgentConfigFile() string

	BinDir() string
	DBPath() string
	CommandsDir() string
	AgentsDir() string
	SkillsDir() string
	PhasesDir() string
	TasksDir() string
	PlanDir() string
	StateDir() string
	PlansDir() string

	PluginDir() string
	PluginManifest(version string) []byte
	HasPluginSystem() bool

	HomeGuardPaths() []string

	DefaultModels() ModelConfig
	ModelAlternatives() []ModelConfig

	RequiredCLIArgs() []string
	ReviewCommentMarker() string
	SupportsThinking() bool
}

type ModelConfig struct {
	Reasoning       string `json:"reasoning"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Fast            string `json:"fast"`
	FastEffort      string `json:"fast_effort,omitempty"`
}

var (
	current Provider
	mu      sync.Mutex
)

func Detect() Provider {
	mu.Lock()
	defer mu.Unlock()
	if current != nil {
		return current
	}
	if name := os.Getenv("CODEDUNGEON_PROVIDER"); name != "" {
		current = byName(name)
		return current
	}
	if DefaultProvider != "" {
		current = byName(DefaultProvider)
		return current
	}
	current = &Claude{}
	return current
}

func byName(name string) Provider {
	switch name {
	case "claude", "claude-code", "claude-ce":
		return &Claude{}
	case "codex", "codex-cli":
		return &Codex{}
	default:
		return &Claude{}
	}
}
