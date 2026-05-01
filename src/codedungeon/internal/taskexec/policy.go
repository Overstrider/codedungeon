package taskexec

import (
	"fmt"
	"strings"

	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func EffectiveConfigForTask(cfg Config, task taskplanning.TaskSpec) Config {
	cfg = normalizeConfig(cfg)
	allowed := append([]string{}, cfg.AllowedTools...)
	if len(allowed) == 0 {
		allowed = append(allowed, "git:status", "git:diff", "git:branch", "git:rev-parse")
	}
	for _, command := range task.VerificationCommands {
		command = strings.TrimSpace(command)
		if command != "" {
			allowed = append(allowed, "shell:"+command)
		}
	}
	if cfg.AutoCommit {
		allowed = append(allowed, "git:add", "git:commit")
	}
	if cfg.AutoTag {
		allowed = append(allowed, "git:tag")
	}
	if cfg.AutoPush {
		allowed = append(allowed, "git:push")
	}
	cfg.AllowedTools = dedupeStrings(allowed)
	return cfg
}

func DefaultWorkspacePolicy(task taskplanning.TaskSpec) string {
	return fmt.Sprintf(`Only modify files declared in the task write_scope unless a required test fix proves broader scope is necessary.
Task write_scope:
- %s
Before editing, search the existing codebase for the target behavior. Do not assume it is missing.
Do not add placeholders, stubs, fake tests, or TODO-only implementations.
If an unrelated test fails, investigate and fix it when the failure is caused by the current workspace state; otherwise report BLOCKED with exact evidence.
Capture the intent of new or changed tests near the test when the reason would not be obvious to a future maintainer.`, strings.Join(task.WriteScope, "\n- "))
}

func dedupeStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
