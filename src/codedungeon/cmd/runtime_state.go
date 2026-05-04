package cmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/provider"
)

const codedungeonDir = ".codedungeon"

func ensureRuntimeState(root string, p provider.Provider) error {
	for _, dir := range []string{
		codedungeonDir,
		p.CommandsDir(),
		p.PhasesDir(),
		p.TasksDir(),
		p.PlanDir(),
		p.StateDir(),
		p.PlansDir(),
		p.ReviewsDir(),
		filepath.Join(codedungeonDir, "reports"),
		filepath.Join(codedungeonDir, "memory", "prs"),
		filepath.Join(codedungeonDir, "memory", "runs"),
		filepath.Join(codedungeonDir, "archive"),
	} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	readme := filepath.Join(root, codedungeonDir, "README.md")
	if _, err := os.Stat(readme); err == nil {
		return nil
	}
	body := `# .codedungeon

Project-local CodeDungeon runtime state.

- codedungeon.db: SQLite source of truth for runs, phases, tasks, handoffs, findings, and model configuration.
- commands/: editable workflow playbooks; Claude keeps thin slash-command wrappers in .claude/commands.
- phases/: editable phase prompts installed by CodeDungeon.
- plan/ and state/: human-readable views rendered from the DB.
- tasks/: task plans and execution notes.
- reviews/: adversarial review inputs and outputs.
- memory/prs/: durable PR/run summaries for later investigation.
- memory/runs/: durable run reports for continuation and audit.
- archive/: migrated or conflicting legacy runtime state.

Provider-native directories such as .claude, .codex, and .agents keep only files required by those CLIs.
`
	return os.WriteFile(readme, []byte(body), 0o644)
}

func migrateLegacyRuntimeState(root string, p provider.Provider) error {
	stamp := time.Now().UTC().Format("20060102-150405")
	legacyRoot := p.ConfigDir()
	moves := []struct {
		from string
		to   string
	}{
		{filepath.Join(legacyRoot, "phases"), p.PhasesDir()},
		{filepath.Join(legacyRoot, "tasks"), p.TasksDir()},
		{filepath.Join(legacyRoot, "plan"), p.PlanDir()},
		{filepath.Join(legacyRoot, "state"), p.StateDir()},
		{filepath.Join(legacyRoot, "plans"), p.PlansDir()},
		{filepath.Join(legacyRoot, "codereview"), p.ReviewsDir()},
	}
	for _, mv := range moves {
		if err := moveLegacyPath(root, mv.from, mv.to, p.Name(), stamp); err != nil {
			return err
		}
	}
	return migrateLegacyDBTriplet(root, legacyRoot, p.DBPath(), p.Name(), stamp)
}

func prepareCommandArtifactInstall(root string, p provider.Provider, artifacts []prompts.Artifact) error {
	stamp := time.Now().UTC().Format("20060102-150405")
	if err := archiveRenamedWorkflowArtifacts(root, stamp); err != nil {
		return err
	}
	if p.Name() == "codex" {
		legacy := filepath.Join(p.ConfigDir(), "commands")
		if _, err := os.Stat(projectPath(root, legacy)); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return fmt.Errorf("stat legacy command dir %s: %w", legacy, err)
		}
		return archiveLegacyPath(root, legacy, p.Name(), stamp)
	}
	if p.Name() != "claude" {
		return nil
	}
	wrappers := map[string][]byte{}
	for _, a := range artifacts {
		if a.Kind == "command-wrapper" {
			wrappers[filepath.Clean(projectPath(root, a.InstallPath))] = a.Content
		}
	}
	legacyDir := filepath.Join(p.ConfigDir(), "commands")
	entries, err := os.ReadDir(projectPath(root, legacyDir))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy command dir %s: %w", legacyDir, err)
	}
	for _, entry := range entries {
		rel := filepath.Join(legacyDir, entry.Name())
		abs := filepath.Clean(projectPath(root, rel))
		if entry.IsDir() {
			if err := archiveLegacyPath(root, rel, p.Name(), stamp); err != nil {
				return err
			}
			continue
		}
		wrapper, ok := wrappers[abs]
		if !ok {
			if err := archiveLegacyPath(root, rel, p.Name(), stamp); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return fmt.Errorf("read legacy command %s: %w", rel, err)
		}
		if bytes.Equal(data, wrapper) {
			continue
		}
		if err := archiveLegacyPath(root, rel, p.Name(), stamp); err != nil {
			return err
		}
	}
	return nil
}

func archiveRenamedWorkflowArtifacts(root, stamp string) error {
	for _, rel := range []string{
		filepath.Join(codedungeonDir, "commands", "codedungeon-dev-cycle.md"),
		filepath.Join(codedungeonDir, "commands", "minidungeon.md"),
		filepath.Join(".claude", "commands", "codedungeon-dev-cycle.md"),
		filepath.Join(".claude", "commands", "minidungeon.md"),
		filepath.Join(".agents", "skills", "codedungeon-dev-cycle"),
		filepath.Join(".agents", "skills", "minidungeon"),
	} {
		if err := archiveRenamedArtifactPath(root, rel, stamp); err != nil {
			return err
		}
	}
	return nil
}

func projectPath(root, rel string) string {
	if filepath.IsAbs(rel) {
		return filepath.Clean(rel)
	}
	return filepath.Join(root, rel)
}

func currentProjectRoot() string {
	cwd, _ := os.Getwd()
	return ResolveCodeDungeonRoot(cwd)
}

func ResolveCodeDungeonRoot(start string) string {
	if root, ok := resolveCodeDungeonDBRoot(start); ok {
		return root
	}
	return ResolveProjectRoot(start)
}

func ResolveCodeDungeonInstallRoot(start string) string {
	absStart, _ := filepath.Abs(start)
	if root, ok := resolveCodeDungeonDBRoot(absStart); ok {
		return root
	}
	return absStart
}

func resolveCodeDungeonDBRoot(start string) (string, bool) {
	dir, _ := filepath.Abs(start)
	dbRel := provider.Detect().DBPath()
	for i := 0; i < 64; i++ {
		if _, err := os.Stat(filepath.Join(dir, dbRel)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

func migrateLegacyDBTriplet(root, legacyRoot, runtimeDBRel, providerName, stamp string) error {
	type dbFile struct {
		fromRel string
		toRel   string
	}
	files := []dbFile{
		{filepath.Join(legacyRoot, "codedungeon.db"), runtimeDBRel},
		{filepath.Join(legacyRoot, "codedungeon.db-wal"), runtimeDBRel + "-wal"},
		{filepath.Join(legacyRoot, "codedungeon.db-shm"), runtimeDBRel + "-shm"},
	}
	var existing []dbFile
	for _, f := range files {
		if _, err := os.Stat(projectPath(root, f.fromRel)); err == nil {
			existing = append(existing, f)
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat legacy path %s: %w", f.fromRel, err)
		}
	}
	if len(existing) == 0 {
		return nil
	}
	_, legacyDBErr := os.Stat(projectPath(root, filepath.Join(legacyRoot, "codedungeon.db")))
	legacyDBExists := legacyDBErr == nil
	if legacyDBErr != nil && !os.IsNotExist(legacyDBErr) {
		return fmt.Errorf("stat legacy db: %w", legacyDBErr)
	}
	archive := !legacyDBExists
	for _, f := range files {
		if _, err := os.Stat(projectPath(root, f.toRel)); err == nil {
			archive = true
			break
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("stat destination %s: %w", f.toRel, err)
		}
	}
	for _, f := range existing {
		if archive {
			if err := archiveLegacyPath(root, f.fromRel, providerName, stamp); err != nil {
				return err
			}
			continue
		}
		if err := moveLegacyPath(root, f.fromRel, f.toRel, providerName, stamp); err != nil {
			return err
		}
	}
	return nil
}

func moveLegacyPath(root, fromRel, toRel, providerName, stamp string) error {
	from := filepath.Join(root, fromRel)
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat legacy path %s: %w", fromRel, err)
	}
	to := filepath.Join(root, toRel)
	if _, err := os.Stat(to); err == nil {
		return archiveLegacyPath(root, fromRel, providerName, stamp)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat destination %s: %w", toRel, err)
	}
	if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
		return fmt.Errorf("mkdir destination for %s: %w", toRel, err)
	}
	if err := os.Rename(from, to); err != nil {
		return fmt.Errorf("move legacy path %s to %s: %w", fromRel, toRel, err)
	}
	return nil
}

func archiveLegacyPath(root, fromRel, providerName, stamp string) error {
	from := projectPath(root, fromRel)
	archiveRel := filepath.Join(codedungeonDir, "archive", "legacy", providerName, stamp, filepath.ToSlash(fromRel))
	archive := filepath.Join(root, archiveRel)
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		return fmt.Errorf("mkdir archive for %s: %w", fromRel, err)
	}
	if err := os.Rename(from, archive); err != nil {
		return fmt.Errorf("archive legacy path %s: %w", fromRel, err)
	}
	return nil
}

func archiveRenamedArtifactPath(root, fromRel, stamp string) error {
	from := projectPath(root, fromRel)
	if _, err := os.Stat(from); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("stat renamed artifact %s: %w", fromRel, err)
	}
	archiveRel := filepath.Join(codedungeonDir, "archive", "renamed-artifacts", stamp, filepath.ToSlash(fromRel))
	archive := filepath.Join(root, archiveRel)
	if err := os.MkdirAll(filepath.Dir(archive), 0o755); err != nil {
		return fmt.Errorf("mkdir archive for renamed artifact %s: %w", fromRel, err)
	}
	if err := os.Rename(from, archive); err != nil {
		return fmt.Errorf("archive renamed artifact %s: %w", fromRel, err)
	}
	return nil
}
