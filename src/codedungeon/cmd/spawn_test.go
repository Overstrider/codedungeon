package cmd

import (
	"strings"
	"testing"
)

func TestBuildSpawnPromptForCodexOmitsThinkingBudget(t *testing.T) {
	got := buildSpawnPromptForProvider("codex", "5", ".codex/phases/forge-execution.md", "fast", "gpt-5.4-mini", 2000, "compact")
	if strings.Contains(got, "max_thinking_tokens") {
		t.Fatalf("codex spawn prompt should not include Claude thinking budget:\n%s", got)
	}
	if !strings.Contains(got, "model: gpt-5.4-mini") {
		t.Fatalf("spawn prompt missing model:\n%s", got)
	}
	if !strings.Contains(got, "agent_type: cd_dev_worker") {
		t.Fatalf("spawn prompt missing codex agent type:\n%s", got)
	}
	if !strings.Contains(got, "via ./.codex/bin/codedungeon config model fast") {
		t.Fatalf("spawn prompt should point model lookup at project-local binary:\n%s", got)
	}
	if !strings.Contains(got, "reasoning_effort: medium") {
		t.Fatalf("codex spawn prompt missing reasoning effort:\n%s", got)
	}
	if !strings.Contains(got, "via ./.codex/bin/codedungeon config effort fast") {
		t.Fatalf("spawn prompt should point effort lookup at project-local binary:\n%s", got)
	}
}

func TestBuildSpawnPromptForClaudeKeepsThinkingBudget(t *testing.T) {
	got := buildSpawnPromptForProvider("claude-code", "5", ".claude/phases/forge-execution.md", "fast", "claude-sonnet-4-6", 2000, "compact")
	if !strings.Contains(got, "max_thinking_tokens: 2000") {
		t.Fatalf("claude spawn prompt should include thinking budget:\n%s", got)
	}
	if !strings.Contains(got, "claude_cli_args: --dangerously-skip-permissions") {
		t.Fatalf("claude spawn prompt should require permission bypass:\n%s", got)
	}
	if strings.Contains(got, "reasoning_effort") {
		t.Fatalf("claude spawn prompt should not include codex reasoning effort:\n%s", got)
	}
}

func TestBuildSpawnPromptForCodexOmitsClaudePermissionBypass(t *testing.T) {
	got := buildSpawnPromptForProvider("codex", "5", ".codex/phases/forge-execution.md", "fast", "gpt-5.5", 2000, "compact")
	if strings.Contains(got, "--dangerously-skip-permissions") {
		t.Fatalf("codex spawn prompt should not include Claude permission bypass:\n%s", got)
	}
}
