package clauderuntime

import "strings"

// ModelEnv pins every Claude Code model-selection path that can otherwise
// resolve aliases or subagents to account defaults.
func ModelEnv(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	return []string{
		"CLAUDE_CODE_SUBAGENT_MODEL=" + model,
		"ANTHROPIC_MODEL=" + model,
		"ANTHROPIC_DEFAULT_SONNET_MODEL=" + model,
		"ANTHROPIC_DEFAULT_OPUS_MODEL=" + model,
		"ANTHROPIC_DEFAULT_HAIKU_MODEL=" + model,
	}
}

func MergeEnv(base []string, overrides []string) []string {
	if len(overrides) == 0 {
		return append([]string{}, base...)
	}
	overrideKeys := map[string]struct{}{}
	for _, item := range overrides {
		if key := envKey(item); key != "" {
			overrideKeys[strings.ToUpper(key)] = struct{}{}
		}
	}
	merged := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key := envKey(item)
		if key == "" {
			merged = append(merged, item)
			continue
		}
		if _, exists := overrideKeys[strings.ToUpper(key)]; exists {
			continue
		}
		merged = append(merged, item)
	}
	return append(merged, overrides...)
}

func envKey(item string) string {
	if idx := strings.Index(item, "="); idx > 0 {
		return item[:idx]
	}
	return ""
}
