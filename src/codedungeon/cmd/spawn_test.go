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
}

func TestBuildSpawnPromptForClaudeKeepsThinkingBudget(t *testing.T) {
	got := buildSpawnPromptForProvider("claude-code", "5", ".claude/phases/forge-execution.md", "fast", "claude-sonnet-4-6", 2000, "compact")
	if !strings.Contains(got, "max_thinking_tokens: 2000") {
		t.Fatalf("claude spawn prompt should include thinking budget:\n%s", got)
	}
}
