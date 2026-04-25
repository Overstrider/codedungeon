package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
)

func TestInstallEmbeddedArtifactsAtUsesExplicitRoot(t *testing.T) {
	root := t.TempDir()
	s, err := db.Open(filepath.Join(root, ".claude", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	if err := installEmbeddedArtifactsAt(s, root); err != nil {
		t.Fatal(err)
	}

	arts, err := prompts.Artifacts()
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range arts {
		if a.LogicalName == "minidungeon" {
			assertFileExists(t, filepath.Join(root, filepath.FromSlash(a.InstallPath)))
			return
		}
	}
	t.Fatal("minidungeon artifact not found")
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}
