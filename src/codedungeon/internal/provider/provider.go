package provider

import (
	"os"
	"sync"
)

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

	ReviewCommentMarker() string
	SupportsThinking() bool
}

type ModelConfig struct {
	Reasoning string `json:"reasoning"`
	Fast      string `json:"fast"`
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
	current = &Claude{}
	return current
}

func byName(name string) Provider {
	switch name {
	case "claude", "claude-code":
		return &Claude{}
	default:
		return &Claude{}
	}
}
