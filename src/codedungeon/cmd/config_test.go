package cmd

import (
	"bytes"
	"encoding/json"
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

func TestConfigModelsShowsActiveLockAndCompiledDefaultsSeparately(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	dbPath := filepath.Join(root, ".codedungeon", "codedungeon.db")
	s, err := db.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"model_reasoning": "claude-sonnet-4-6",
		"model_fast":      "claude-sonnet-4-6",
		"model_lock":      "claude-sonnet-4-6",
	} {
		if err := s.SetMeta(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	cmd := ConfigCmd()
	cmd.SetArgs([]string{"models"})
	out, err := executeCommandInDir(root, cmd)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("decode output %q: %v", out, err)
	}
	if payload["model_lock"] != "claude-sonnet-4-6" {
		t.Fatalf("model_lock missing: %+v", payload)
	}
	if _, ok := payload["compiled_defaults"]; !ok {
		t.Fatalf("compiled_defaults missing: %+v", payload)
	}
	if _, ok := payload["defaults"]; ok {
		t.Fatalf("config models should not expose ambiguous defaults key: %+v", payload)
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
	dbPath := filepath.Join(root, ".codedungeon", "codedungeon.db")
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
