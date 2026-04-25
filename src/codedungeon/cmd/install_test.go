package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
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

func TestRunBootstrapReturnsArtifactInstallErrors(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "agents"), []byte("not a dir"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RunBootstrap(root, "reasoning-model", "fast-model", false); err == nil {
		t.Fatal("RunBootstrap returned nil error, want artifact install failure")
	}
}

func TestCodexSetupInstallsProviderArtifacts(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")

	cmd := exec.Command("go", "run", ".", "setup", "--target", root, "--yes")
	cmd.Dir = filepath.Clean("..")
	cmd.Env = append(os.Environ(), "CODEDUNGEON_PROVIDER=codex")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("codex setup failed: %v\n%s", err, out)
	}
	var payload struct {
		OK                 bool              `json:"ok"`
		ArtifactsInstalled int               `json:"artifacts_installed"`
		Models             map[string]string `json:"models"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("setup output is not JSON: %v\n%s", err, out)
	}
	if !payload.OK || payload.ArtifactsInstalled == 0 {
		t.Fatalf("setup payload = %+v, want ok with artifacts", payload)
	}
	if payload.Models["reasoning"] != "gpt-5.5" || payload.Models["reasoning_effort"] != "xhigh" {
		t.Fatalf("reasoning model config = %#v, want gpt-5.5/xhigh", payload.Models)
	}
	if payload.Models["fast"] != "gpt-5.5" || payload.Models["fast_effort"] != "medium" {
		t.Fatalf("fast model config = %#v, want gpt-5.5/medium", payload.Models)
	}
	for _, path := range []string{
		"AGENTS.md",
		filepath.Join(".codex", "config.toml"),
		filepath.Join(".codex", "agents", "cd_dev_worker.toml"),
		filepath.Join(".agents", "skills", "codedungeon-dev-cycle", "SKILL.md"),
	} {
		assertFileExists(t, filepath.Join(root, path))
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s: %v", path, err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
