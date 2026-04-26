package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/provider"
)

func TestInstallEmbeddedArtifactsAtUsesExplicitRoot(t *testing.T) {
	root := t.TempDir()
	s, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
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
		if a.LogicalName == "side-quest" {
			assertFileExists(t, filepath.Join(root, filepath.FromSlash(a.InstallPath)))
			return
		}
	}
	t.Fatal("side-quest artifact not found")
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

func TestRunBootstrapMigratesLegacyRuntimeState(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	legacyDB := filepath.Join(root, ".claude", "codedungeon.db")
	legacyPlan := filepath.Join(root, ".claude", "plan", "pipeline-state.md")
	if err := os.MkdirAll(filepath.Dir(legacyPlan), 0o755); err != nil {
		t.Fatal(err)
	}
	legacyStore, err := db.Open(legacyDB)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacyStore.Init(); err != nil {
		t.Fatal(err)
	}
	if err := legacyStore.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyPlan, []byte("legacy-plan"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RunBootstrap(root, "reasoning-model", "fast-model", true); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}

	assertFileExists(t, filepath.Join(root, ".codedungeon", "codedungeon.db"))
	assertFileExists(t, filepath.Join(root, ".codedungeon", "plan", "pipeline-state.md"))
	if _, err := os.Stat(legacyDB); !os.IsNotExist(err) {
		t.Fatalf("legacy db still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(legacyPlan); !os.IsNotExist(err) {
		t.Fatalf("legacy plan still exists or stat failed unexpectedly: %v", err)
	}
}

func TestLegacyDBSidecarsArchiveWithLegacyDBWhenRuntimeDBExists(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	runtimeStore, err := db.Open(filepath.Join(root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.Init(); err != nil {
		t.Fatal(err)
	}
	if err := runtimeStore.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"codedungeon.db", "codedungeon.db-wal", "codedungeon.db-shm"} {
		path := filepath.Join(root, ".claude", name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("legacy-"+name), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := migrateLegacyRuntimeState(root, provider.Claude{}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"codedungeon.db-wal", "codedungeon.db-shm"} {
		if _, err := os.Stat(filepath.Join(root, ".codedungeon", name)); !os.IsNotExist(err) {
			t.Fatalf("legacy sidecar %s should not be moved next to runtime DB: %v", name, err)
		}
	}
	var archived []string
	err = filepath.Walk(filepath.Join(root, ".codedungeon", "archive"), func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			archived = append(archived, filepath.Base(path))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(archived, ",")
	for _, name := range []string{"codedungeon.db", "codedungeon.db-wal", "codedungeon.db-shm"} {
		if !strings.Contains(got, name) {
			t.Fatalf("archive missing %s, got %v", name, archived)
		}
	}
}

func TestPhaseOutputsFromSubdirectoryWriteToProjectRoot(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}

	initCmd := PhaseCmd()
	initCmd.SetArgs([]string{"init", "--feature", "demo"})
	if err := initCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	doneCmd := PhaseCmd()
	doneCmd.SetArgs([]string{"done", "0", "--summary", "ok", "--promise", "PHASE_0_COMPLETE"})
	if err := doneCmd.Execute(); err != nil {
		t.Fatal(err)
	}
	renderCmd := PhaseCmd()
	renderCmd.SetArgs([]string{"render-state"})
	if err := renderCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	assertFileExists(t, filepath.Join(root, ".codedungeon", "state", "phase-0-output.md"))
	assertFileExists(t, filepath.Join(root, ".codedungeon", "plan", "pipeline-state.md"))
	if _, err := os.Stat(filepath.Join(sub, ".codedungeon")); !os.IsNotExist(err) {
		t.Fatalf("unexpected subdir runtime state: %v", err)
	}
}

func TestSetupYesIsIdempotentWhenAlreadyBootstrapped(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	opts := setupOptions{Target: root, Yes: true, SkipGlobal: true}
	if err := runSetupWithOptions(opts); err != nil {
		t.Fatalf("first setup failed: %v", err)
	}
	if err := runSetupWithOptions(opts); err != nil {
		t.Fatalf("second setup should be idempotent: %v", err)
	}
}

func TestClaudeSetupArchivesLegacyCommandsAndInstallsWrappers(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	legacyCommand := filepath.Join(root, ".claude", "commands", "side-quest.md")
	if err := os.MkdirAll(filepath.Dir(legacyCommand), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(legacyCommand, []byte("custom legacy command"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := RunBootstrap(root, "reasoning-model", "fast-model", true); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}

	playbook := filepath.Join(root, ".codedungeon", "commands", "side-quest.md")
	wrapper := filepath.Join(root, ".claude", "commands", "side-quest.md")
	assertFileExists(t, playbook)
	wrapperBody, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(wrapperBody), "@.codedungeon/commands/side-quest.md") {
		t.Fatalf("wrapper body should point at .codedungeon playbook, got:\n%s", wrapperBody)
	}

	var archived bool
	err = filepath.Walk(filepath.Join(root, ".codedungeon", "archive"), func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return err
		}
		if filepath.Base(path) != "side-quest.md" {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if string(data) == "custom legacy command" {
			archived = true
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !archived {
		t.Fatal("legacy command was not archived before wrapper install")
	}
}

func TestSetupArchivesRenamedWorkflowArtifacts(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")
	oldPaths := []string{
		filepath.Join(".codedungeon", "commands", "codedungeon-dev-cycle.md"),
		filepath.Join(".codedungeon", "commands", "minidungeon.md"),
		filepath.Join(".claude", "commands", "codedungeon-dev-cycle.md"),
		filepath.Join(".claude", "commands", "minidungeon.md"),
		filepath.Join(".agents", "skills", "codedungeon-dev-cycle", "SKILL.md"),
		filepath.Join(".agents", "skills", "minidungeon", "SKILL.md"),
	}
	for _, rel := range oldPaths {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("old "+rel), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := RunBootstrap(root, "reasoning-model", "fast-model", true); err != nil {
		t.Fatalf("RunBootstrap failed: %v", err)
	}

	for _, rel := range oldPaths {
		top := rel
		if strings.HasSuffix(rel, string(filepath.Separator)+"SKILL.md") {
			top = filepath.Dir(rel)
		}
		if _, err := os.Stat(filepath.Join(root, top)); !os.IsNotExist(err) {
			t.Fatalf("old renamed artifact still exists at %s: %v", top, err)
		}
	}
	assertFileExists(t, filepath.Join(root, ".codedungeon", "commands", "main-quest.md"))
	assertFileExists(t, filepath.Join(root, ".codedungeon", "commands", "side-quest.md"))
	assertFileExists(t, filepath.Join(root, ".codedungeon", "commands", "one-shot.md"))
	assertFileExists(t, filepath.Join(root, ".codedungeon", "commands", "codedungeon.md"))
	assertFileExists(t, filepath.Join(root, ".claude", "commands", "main-quest.md"))
	assertFileExists(t, filepath.Join(root, ".claude", "commands", "side-quest.md"))
	assertFileExists(t, filepath.Join(root, ".claude", "commands", "one-shot.md"))
	assertFileExists(t, filepath.Join(root, ".claude", "commands", "codedungeon.md"))

	var archived int
	err := filepath.Walk(filepath.Join(root, ".codedungeon", "archive", "renamed-artifacts"), func(path string, info os.FileInfo, err error) error {
		if err == nil && info != nil && !info.IsDir() {
			archived++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if archived != len(oldPaths) {
		t.Fatalf("archived renamed artifacts = %d, want %d", archived, len(oldPaths))
	}
}

func TestCodexSetupInstallsProviderArtifacts(t *testing.T) {
	root := t.TempDir()
	runGit(t, root, "init")

	cmd := exec.Command("go", "run", ".", "setup", "--target", root, "--yes", "--skip-global")
	cmd.Dir = filepath.Clean("..")
	cmd.Env = append(os.Environ(), "CODEDUNGEON_PROVIDER=codex")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("codex setup failed: %v\n%s", err, out)
	}
	var payload struct {
		OK                 bool              `json:"ok"`
		ArtifactsInstalled int               `json:"artifacts_installed"`
		CodexMultiAgentV2  string            `json:"codex_multi_agent_v2"`
		Models             map[string]string `json:"models"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		t.Fatalf("setup output is not JSON: %v\n%s", err, out)
	}
	if !payload.OK || payload.ArtifactsInstalled == 0 {
		t.Fatalf("setup payload = %+v, want ok with artifacts", payload)
	}
	if payload.CodexMultiAgentV2 != "skipped" {
		t.Fatalf("codex_multi_agent_v2 = %q, want skipped", payload.CodexMultiAgentV2)
	}
	if payload.Models["reasoning"] != "gpt-5.5" || payload.Models["reasoning_effort"] != "xhigh" {
		t.Fatalf("reasoning model config = %#v, want gpt-5.5/xhigh", payload.Models)
	}
	if payload.Models["fast"] != "gpt-5.5" || payload.Models["fast_effort"] != "medium" {
		t.Fatalf("fast model config = %#v, want gpt-5.5/medium", payload.Models)
	}
	for _, path := range []string{
		"AGENTS.md",
		filepath.Join(".codedungeon", "codedungeon.db"),
		filepath.Join(".codedungeon", "README.md"),
		filepath.Join(".codedungeon", "commands", "main-quest.md"),
		filepath.Join(".codedungeon", "commands", "side-quest.md"),
		filepath.Join(".codedungeon", "commands", "one-shot.md"),
		filepath.Join(".codedungeon", "commands", "codedungeon.md"),
		filepath.Join(".codedungeon", "phases", "forge-execution.md"),
		filepath.Join(".codex", "config.toml"),
		filepath.Join(".codex", "agents", "cd_dev_worker.toml"),
		filepath.Join(".agents", "skills", "codedungeon", "SKILL.md"),
		filepath.Join(".agents", "skills", "main-quest", "SKILL.md"),
		filepath.Join(".agents", "skills", "side-quest", "SKILL.md"),
		filepath.Join(".agents", "skills", "one-shot", "SKILL.md"),
	} {
		assertFileExists(t, filepath.Join(root, path))
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "codedungeon.db")); !os.IsNotExist(err) {
		t.Fatalf("unexpected provider-local db at .codex/codedungeon.db: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".codex", "commands")); !os.IsNotExist(err) {
		t.Fatalf("unexpected codex command dir at .codex/commands: %v", err)
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
