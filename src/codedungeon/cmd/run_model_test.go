package cmd

import (
	"path/filepath"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

func TestProviderChildModelUsesConfiguredClaudeReasoningModel(t *testing.T) {
	root := t.TempDir()
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("model_reasoning", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("model_fast", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}

	got := providerChildModel(root, "full", provider.Claude{})
	if got != "claude-sonnet-4-6" {
		t.Fatalf("provider child model = %q, want claude-sonnet-4-6", got)
	}
}

func TestProviderChildModelUsesClaudeModelLockInsteadOfCompiledDefault(t *testing.T) {
	root := t.TempDir()
	store, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("model_lock", "claude-sonnet-4-6"); err != nil {
		t.Fatal(err)
	}
	if err := store.SetMeta("model_reasoning", "claude-opus-4-7"); err != nil {
		t.Fatal(err)
	}

	got := providerChildModel(root, "full", provider.Claude{})
	if got != "claude-sonnet-4-6" {
		t.Fatalf("provider child model = %q, want locked claude-sonnet-4-6", got)
	}
}
