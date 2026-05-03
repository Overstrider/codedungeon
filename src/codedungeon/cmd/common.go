// Package cmd implements the cobra subcommands for codedungeon.
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

// ---- Preflight guards ----

// ErrHomeConfig is returned when the CWD is under a provider's home config dir.
var ErrHomeConfig = errors.New("refuse: codedungeon must not run inside the provider's home config directory - use a project directory")

// ErrNoGit is returned when the project root has no .git/.
var ErrNoGit = errors.New("refuse: project root has no .git - codedungeon requires a git repository")

// ErrNoProject is returned when no project root could be resolved.
var ErrNoProject = errors.New("no project root found - invoke `codedungeon bootstrap` with --target")

type migrationRequiredError struct {
	From string
	To   string
}

func (e migrationRequiredError) Error() string {
	return fmt.Sprintf("migration-required: %s -> %s", e.From, e.To)
}

func isMigrationRequired(err error) bool {
	var target migrationRequiredError
	return errors.As(err, &target)
}

// ResolveProjectRoot walks up from start looking for a `.git/` dir (project root).
// Falls back to abs(start) if none found — callers verify via GuardGit.
func ResolveProjectRoot(start string) string {
	dir, _ := filepath.Abs(start)
	for i := 0; i < 64; i++ {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	abs, _ := filepath.Abs(start)
	return abs
}

// IsHomeConfig returns true if path is under the provider's home config dir.
func IsHomeConfig(path string) bool {
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)
	for _, guard := range provider.Detect().HomeGuardPaths() {
		guardAbs, _ := filepath.Abs(filepath.FromSlash(guard))
		guardAbs = filepath.Clean(guardAbs)
		if abs == guardAbs || strings.HasPrefix(abs, guardAbs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// HasGit returns true if dir has .git (dir OR file — gitfile is valid for worktrees/submodules).
func HasGit(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// GuardHomeConfig refuses if CWD is under the provider's home config dir.
func GuardHomeConfig() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if IsHomeConfig(cwd) {
		return ErrHomeConfig
	}
	return nil
}

// GuardGit refuses if there's no .git at the resolved project root.
func GuardGit() error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root := ResolveProjectRoot(cwd)
	if !HasGit(root) {
		return ErrNoGit
	}
	return nil
}

// Preflight runs both guards. Used by all DB-touching commands.
// `bootstrap` and `version` bypass this by not calling OpenDB / Preflight.
func Preflight() error {
	if err := GuardHomeConfig(); err != nil {
		return err
	}
	if err := GuardGit(); err != nil {
		return err
	}
	return nil
}

// ---- DB helpers ----

// OpenDB opens the store using --db flag or the provider project DB path.
// Runs preflight first (enforces project-only + git-gate).
// Also checks cd_version vs binary.Version: mismatch → structured error.
func OpenDB(c *cobra.Command) (*db.Store, error) {
	if err := Preflight(); err != nil {
		return nil, err
	}
	path, _ := c.Flags().GetString("db")
	if path == "" {
		cwd, _ := os.Getwd()
		root := ResolveProjectRoot(cwd)
		p := provider.Detect()
		if err := migrateLegacyRuntimeState(root, p); err != nil {
			return nil, err
		}
		if err := ensureRuntimeState(root, p); err != nil {
			return nil, err
		}
		path = projectPath(root, provider.Detect().DBPath())
	}
	s, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	// Auto-migration gate — only for commands that already routed through here.
	// Bootstrap / migrate / install / version opt out by NOT calling OpenDB.
	if skipAutoMigrate[c.Name()] {
		return s, nil
	}
	// Ignore gate when DB is brand new (no cd_version yet — bootstrap writes it).
	storedVer, _ := s.GetMeta("cd_version")
	if storedVer == "" {
		return s, nil
	}
	binVer := versionString()
	if binVer != "" && binVer != "unknown" && binVer != storedVer {
		s.Close()
		return nil, migrationRequiredError{From: storedVer, To: binVer}
	}
	return s, nil
}

// skipAutoMigrate lists cobra Use-names that must run without the version gate.
// (bootstrap, migrate, install, version, status/report — anything read-only or that can heal drift.)
var skipAutoMigrate = map[string]bool{
	"bootstrap": true,
	"migrate":   true,
	"install":   true,
	"report":    true,
	"status":    true,
	"version":   true,
	"setup":     true,
	"artifacts": true,
	"list":      true,
	"verify":    true,
	"backfill":  true,
}

// OpenDBNoGuard skips preflight (only bootstrap uses this).
func OpenDBNoGuard(path string) (*db.Store, error) {
	if path == "" {
		cwd, _ := os.Getwd()
		path = filepath.Join(cwd, provider.Detect().DBPath())
	}
	return db.Open(path)
}

// ---- Output helpers ----

// EmitJSON writes val as indented JSON to stdout.
func EmitJSON(val any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(val)
}

// EmitErr prints a generic error JSON + returns it so cobra exits 1.
func EmitErr(msg string, hint string) error {
	_ = EmitJSON(map[string]any{"ok": false, "error": msg, "hint": hint})
	return fmt.Errorf("%s", msg)
}

// EmitPreflightErr emits a structured error the agent can parse.
func EmitPreflightErr(err error) error {
	var msg, hint, action string
	msg = err.Error()
	switch {
	case errors.Is(err, ErrHomeConfig):
		hint = "cd into a project directory that is NOT under the provider's home config directory"
		action = "change-directory"
	case errors.Is(err, ErrNoGit):
		hint = "run `git init` in the project root, OR invoke `codedungeon bootstrap --target <path>`"
		action = "init-git-or-bootstrap"
	case errors.Is(err, ErrNoProject):
		hint = "invoke `codedungeon bootstrap --target <abs path>`"
		action = "bootstrap"
	}
	_ = EmitJSON(map[string]any{"ok": false, "error": msg, "hint": hint, "action": action})
	return fmt.Errorf("%s", msg)
}

// Human returns the --human flag value.
func Human(c *cobra.Command) bool {
	h, _ := c.Flags().GetBool("human")
	return h
}
