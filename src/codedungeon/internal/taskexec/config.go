package taskexec

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loldinis/codedungeon/internal/taskplanning"
)

type Config struct {
	SessionTTLHours int                              `json:"session_ttl_hours"`
	MaxIterations   int                              `json:"max_iterations"`
	TimeoutSeconds  int                              `json:"timeout_seconds"`
	Runner          string                           `json:"runner"`
	AutoCommit      bool                             `json:"auto_commit"`
	AutoPush        bool                             `json:"auto_push"`
	AutoTag         bool                             `json:"auto_tag"`
	Verbose         bool                             `json:"verbose"`
	AllowedTools    []string                         `json:"allowed_tools"`
	PromptPlanner   taskplanning.PromptPlannerConfig `json:"prompt_planner"`
}

func DefaultConfig() Config {
	return Config{
		SessionTTLHours: 24,
		MaxIterations:   9,
		TimeoutSeconds:  3600,
		Runner:          "codex",
		AutoCommit:      true,
		AutoPush:        false,
		AutoTag:         false,
		PromptPlanner:   taskplanning.DefaultPromptPlannerConfig(),
		AllowedTools: []string{
			"shell:go test ./...",
			"shell:go test ./internal/taskexec",
			"git:status",
			"git:diff",
			"git:add",
			"git:commit",
			"git:branch",
			"git:rev-parse",
		},
	}
}

func LoadConfig(root string) (Config, error) {
	cfg := DefaultConfig()
	path := filepath.Join(root, ".ralphrc")
	if body, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(body))) > 0 {
		if err := applyRalphrc(body, &cfg); err != nil {
			return Config{}, err
		}
	}
	applyIntEnv("CODEDUNGEON_EXEC_SESSION_TTL_HOURS", &cfg.SessionTTLHours)
	applyIntEnv("CODEDUNGEON_EXEC_MAX_ITERATIONS", &cfg.MaxIterations)
	applyIntEnv("CODEDUNGEON_EXEC_TIMEOUT_SECONDS", &cfg.TimeoutSeconds)
	applyStringEnv("CODEDUNGEON_EXEC_RUNNER", &cfg.Runner)
	applyBoolEnv("CODEDUNGEON_EXEC_AUTO_COMMIT", &cfg.AutoCommit)
	applyBoolEnv("CODEDUNGEON_EXEC_AUTO_PUSH", &cfg.AutoPush)
	applyBoolEnv("CODEDUNGEON_EXEC_AUTO_TAG", &cfg.AutoTag)
	applyBoolEnv("CODEDUNGEON_EXEC_VERBOSE", &cfg.Verbose)
	applyBoolEnv("CODEDUNGEON_EXEC_PROMPT_PLANNER_ENABLED", &cfg.PromptPlanner.Enabled)
	applyStringEnv("CODEDUNGEON_EXEC_PROMPT_PLANNER_MODEL", &cfg.PromptPlanner.Model)
	applyStringEnv("CODEDUNGEON_EXEC_PROMPT_PLANNER_REASONING_EFFORT", &cfg.PromptPlanner.ReasoningEffort)
	applyIntEnv("CODEDUNGEON_EXEC_PROMPT_PLANNER_TIMEOUT_SECONDS", &cfg.PromptPlanner.TimeoutSeconds)
	applyBoolEnv("CODEDUNGEON_EXEC_PROMPT_PLANNER_EPHEMERAL", &cfg.PromptPlanner.Ephemeral)
	return normalizeConfig(cfg), nil
}

func normalizeConfig(cfg Config) Config {
	if cfg.SessionTTLHours <= 0 {
		cfg.SessionTTLHours = 24
	}
	if cfg.MaxIterations <= 0 {
		cfg.MaxIterations = 9
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 3600
	}
	if strings.TrimSpace(cfg.Runner) == "" {
		cfg.Runner = "codex"
	}
	defaultPlanner := taskplanning.DefaultPromptPlannerConfig()
	if strings.TrimSpace(cfg.PromptPlanner.Model) == "" {
		cfg.PromptPlanner.Model = defaultPlanner.Model
	}
	if strings.TrimSpace(cfg.PromptPlanner.ReasoningEffort) == "" {
		cfg.PromptPlanner.ReasoningEffort = defaultPlanner.ReasoningEffort
	}
	if cfg.PromptPlanner.TimeoutSeconds <= 0 {
		cfg.PromptPlanner.TimeoutSeconds = defaultPlanner.TimeoutSeconds
	}
	if cfg.PromptPlanner.FallbackModels == nil {
		cfg.PromptPlanner.FallbackModels = append([]string(nil), defaultPlanner.FallbackModels...)
	}
	return cfg
}

type rawRalphrc struct {
	SessionTTLHours int                              `json:"session_ttl_hours"`
	MaxIterations   int                              `json:"max_iterations"`
	TimeoutSeconds  int                              `json:"timeout_seconds"`
	Runner          string                           `json:"runner"`
	AutoCommit      bool                             `json:"auto_commit"`
	AutoPush        bool                             `json:"auto_push"`
	AutoTag         bool                             `json:"auto_tag"`
	Verbose         bool                             `json:"verbose"`
	AllowedTools    []string                         `json:"allowed_tools"`
	PromptPlanner   taskplanning.PromptPlannerConfig `json:"prompt_planner"`
	Execution       struct {
		PromptPlanner promptPlannerRaw `json:"prompt_planner"`
	} `json:"execution"`
}

type promptPlannerRaw struct {
	Enabled         *bool    `json:"enabled"`
	Model           *string  `json:"model"`
	ReasoningEffort *string  `json:"reasoning_effort"`
	Timeout         *string  `json:"timeout"`
	TimeoutSeconds  *int     `json:"timeout_seconds"`
	Ephemeral       *bool    `json:"ephemeral"`
	FallbackModels  []string `json:"fallback_models"`
}

func applyRalphrc(body []byte, cfg *Config) error {
	body = trimUTF8BOM(body)
	trimmed := strings.TrimSpace(string(body))
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), cfg); err != nil {
			return err
		}
		var raw rawRalphrc
		if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
			return err
		}
		return applyPromptPlannerRaw(&cfg.PromptPlanner, raw.Execution.PromptPlanner)
	}
	return applyDottedRalphrc(trimmed, cfg)
}

func applyPromptPlannerRaw(cfg *taskplanning.PromptPlannerConfig, raw promptPlannerRaw) error {
	if raw.Enabled != nil {
		cfg.Enabled = *raw.Enabled
	}
	if raw.Model != nil {
		cfg.Model = *raw.Model
	}
	if raw.ReasoningEffort != nil {
		cfg.ReasoningEffort = *raw.ReasoningEffort
	}
	if raw.TimeoutSeconds != nil {
		cfg.TimeoutSeconds = *raw.TimeoutSeconds
	}
	if raw.Timeout != nil {
		seconds, err := parseDurationSeconds(*raw.Timeout)
		if err != nil {
			return err
		}
		cfg.TimeoutSeconds = seconds
	}
	if raw.Ephemeral != nil {
		cfg.Ephemeral = *raw.Ephemeral
	}
	if raw.FallbackModels != nil {
		cfg.FallbackModels = append([]string(nil), raw.FallbackModels...)
	}
	return nil
}

func applyDottedRalphrc(src string, cfg *Config) error {
	for _, line := range strings.Split(src, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid .ralphrc line %q", line)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "session_ttl_hours":
			if err := assignInt(value, &cfg.SessionTTLHours); err != nil {
				return err
			}
		case "max_iterations":
			if err := assignInt(value, &cfg.MaxIterations); err != nil {
				return err
			}
		case "timeout_seconds":
			if err := assignInt(value, &cfg.TimeoutSeconds); err != nil {
				return err
			}
		case "runner":
			cfg.Runner = unquoteRalphValue(value)
		case "auto_commit":
			if err := assignBool(value, &cfg.AutoCommit); err != nil {
				return err
			}
		case "auto_push":
			if err := assignBool(value, &cfg.AutoPush); err != nil {
				return err
			}
		case "auto_tag":
			if err := assignBool(value, &cfg.AutoTag); err != nil {
				return err
			}
		case "verbose":
			if err := assignBool(value, &cfg.Verbose); err != nil {
				return err
			}
		case "execution.prompt_planner.enabled":
			if err := assignBool(value, &cfg.PromptPlanner.Enabled); err != nil {
				return err
			}
		case "execution.prompt_planner.model":
			cfg.PromptPlanner.Model = unquoteRalphValue(value)
		case "execution.prompt_planner.reasoning_effort":
			cfg.PromptPlanner.ReasoningEffort = unquoteRalphValue(value)
		case "execution.prompt_planner.timeout":
			seconds, err := parseDurationSeconds(unquoteRalphValue(value))
			if err != nil {
				return err
			}
			cfg.PromptPlanner.TimeoutSeconds = seconds
		case "execution.prompt_planner.timeout_seconds":
			if err := assignInt(value, &cfg.PromptPlanner.TimeoutSeconds); err != nil {
				return err
			}
		case "execution.prompt_planner.ephemeral":
			if err := assignBool(value, &cfg.PromptPlanner.Ephemeral); err != nil {
				return err
			}
		case "execution.prompt_planner.fallback_models":
			var values []string
			if err := json.Unmarshal([]byte(value), &values); err != nil {
				return fmt.Errorf("parse fallback_models: %w", err)
			}
			cfg.PromptPlanner.FallbackModels = values
		}
	}
	return nil
}

func assignInt(raw string, dst *int) error {
	v, err := strconv.Atoi(unquoteRalphValue(raw))
	if err != nil {
		return err
	}
	*dst = v
	return nil
}

func assignBool(raw string, dst *bool) error {
	v, err := strconv.ParseBool(unquoteRalphValue(raw))
	if err != nil {
		return err
	}
	*dst = v
	return nil
}

func parseDurationSeconds(raw string) (int, error) {
	if seconds, err := strconv.Atoi(raw); err == nil {
		return seconds, nil
	}
	duration, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse prompt planner timeout %q: %w", raw, err)
	}
	return int(duration.Seconds()), nil
}

func unquoteRalphValue(raw string) string {
	raw = strings.TrimSpace(raw)
	if unquoted, err := strconv.Unquote(raw); err == nil {
		return unquoted
	}
	return raw
}

func applyIntEnv(key string, dst *int) {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			*dst = v
		}
	}
}

func applyStringEnv(key string, dst *string) {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		*dst = raw
	}
}

func applyBoolEnv(key string, dst *bool) {
	if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
		if v, err := strconv.ParseBool(raw); err == nil {
			*dst = v
		}
	}
}
