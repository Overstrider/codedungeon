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
	ReviewsDir() string

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

// KnownModels returns the set of model IDs the provider ships as defaults or
// documented alternatives (both reasoning and fast tiers). Used to catch typos
// in `config set-models`. It is intentionally non-exhaustive — newer model IDs
// may be valid without being listed here — so callers should warn (not hard-fail)
// on an unknown model unless strict validation is requested.
func KnownModels(p Provider) map[string]bool {
	known := map[string]bool{}
	add := func(c ModelConfig) {
		if c.Reasoning != "" {
			known[c.Reasoning] = true
		}
		if c.Fast != "" {
			known[c.Fast] = true
		}
	}
	add(p.DefaultModels())
	for _, alt := range p.ModelAlternatives() {
		add(alt)
	}
	return known
}
