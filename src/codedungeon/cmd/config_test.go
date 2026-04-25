package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
)

func TestNormalizeModelTierAliases(t *testing.T) {
	for input, want := range map[string]string{
		"reasoning": "reasoning",
		"architect": "reasoning",
		"fast":      "fast",
		"execution": "fast",
		"exec":      "fast",
	} {
		got, err := normalizeModelTier(input)
		if err != nil {
			t.Fatalf("normalizeModelTier(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeModelTier(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNormalizeReasoningEffort(t *testing.T) {
	for input, want := range map[string]string{
		"low":    "low",
		"med":    "medium",
		"medium": "medium",
		"high":   "high",
		"xhigh":  "xhigh",
	} {
		got, err := normalizeReasoningEffort(input)
		if err != nil {
			t.Fatalf("normalizeReasoningEffort(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeReasoningEffort(%q) = %q, want %q", input, got, want)
		}
	}
	if _, err := normalizeReasoningEffort("huge"); err == nil {
		t.Fatal("normalizeReasoningEffort accepted invalid effort")
	}
}

func TestConfigEffortPrintsConfiguredTierEffort(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	dbPath := filepath.Join(root, ".claude", "codedungeon.db")
	s, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("model_reasoning_effort", "xhigh"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMeta("model_fast_effort", "medium"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	cmd := ConfigCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"effort", "architect"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.String() != "xhigh" {
		t.Fatalf("config effort output = %q, want xhigh", out.String())
	}
}
