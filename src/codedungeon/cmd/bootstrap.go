package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/osadapter"
	"github.com/loldinis/codedungeon/internal/prompts"
)

// BootstrapCmd self-copies codedungeon into <project>/.claude/bin/, creates
// the DB, and records project_root + os + cd_version in meta. First-run flow.
func BootstrapCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "bootstrap",
		Short: "First-run: copy binary into <project>/.claude/bin + init DB (requires .git)",
		Long: `Run once per project. codedungeon refuses to run inside ~/.claude or /root/.claude.
If --target is omitted, the current CWD is used as the project root.
Requires .git at the target (or --init-git to create one — not default).`,
		RunE: func(c *cobra.Command, _ []string) error {
			target, _ := c.Flags().GetString("target")
			force, _ := c.Flags().GetBool("force")
			reasoning, _ := c.Flags().GetString("reasoning")
			fast, _ := c.Flags().GetString("fast")
			if target == "" {
				cwd, _ := os.Getwd()
				target = cwd
			}
			target, _ = filepath.Abs(target)

			// Guard: never bootstrap under home .claude.
			if IsHomeClaude(target) {
				return EmitPreflightErr(ErrHomeClaude)
			}

			// Require .git at target — never auto-init.
			if !HasGit(target) {
				return EmitPreflightErr(ErrNoGit)
			}

			// Models must be specified (M2M: agent parses error and re-invokes).
			if reasoning == "" || fast == "" {
				_ = EmitJSON(map[string]any{
					"error":        "models-not-configured",
					"action":       "ask-user-models",
					"defaults":     ModelDefaults,
					"alternatives": ModelAlternatives,
					"hint":         "re-invoke: codedungeon bootstrap --reasoning <id> --fast <id>  (pick from defaults/alternatives or ask user)",
				})
				return fmt.Errorf("models not configured")
			}

			// Layout: <target>/.claude/bin/codedungeon + <target>/.claude/codedungeon.db
			claudeDir := filepath.Join(target, ".claude")
			binDir := filepath.Join(claudeDir, "bin")
			if err := os.MkdirAll(binDir, 0o755); err != nil {
				return EmitErr(err.Error(), "cannot create "+binDir)
			}
			dbPath := filepath.Join(claudeDir, "codedungeon.db")
			binPath := filepath.Join(binDir, "codedungeon"+osadapter.Detect().ExecutableExt())

			// Refuse if already bootstrapped (unless --force).
			if _, err := os.Stat(dbPath); err == nil && !force {
				return EmitErr("already bootstrapped: "+dbPath, "use --force to overwrite, or just call `codedungeon db migrate`")
			}

			// Self-copy the running binary.
			srcBin, err := os.Executable()
			if err != nil {
				return EmitErr("cannot resolve running binary: "+err.Error(), "")
			}
			if err := copyFile(srcBin, binPath, 0o755); err != nil {
				return EmitErr("copy binary: "+err.Error(), "")
			}

			// Init DB.
			s, err := OpenDBNoGuard(dbPath)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			// Seed embedded prompts (idempotent).
			seeded, err := seedEmbeddedPrompts(s)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			// Install embedded artifacts (agents/skills/commands/phases) into .claude/.
			// Best-effort: failures here don't block bootstrap; user can re-run
			// `codedungeon install` separately.
			artifactsInstalled := 0
			if err := installEmbeddedArtifacts(s); err == nil {
				if all, err := prompts.Artifacts(); err == nil {
					artifactsInstalled = len(all)
				}
			}
			_ = artifactsInstalled
			// Record project meta.
			ad := osadapter.Detect()
			meta := map[string]string{
				"os":              ad.OS(),
				"project_root":    target,
				"cd_version":      versionString(),
				"bootstrapped_at": fmt.Sprintf("%d", time.Now().Unix()),
				"model_reasoning": reasoning,
				"model_fast":      fast,
			}
			for k, v := range meta {
				if err := s.SetMeta(k, v); err != nil {
					return EmitErr(err.Error(), "")
				}
			}

			return EmitJSON(map[string]any{
				"ok":                  true,
				"bootstrapped":        true,
				"project_root":        target,
				"bin":                 binPath,
				"db":                  dbPath,
				"os":                  ad.OS(),
				"prompts_seeded":      seeded,
				"artifacts_installed": artifactsInstalled,
				"cd_version":          versionString(),
				"models": map[string]string{
					"reasoning": reasoning,
					"fast":      fast,
				},
			})
		},
	}
	c.Flags().String("target", "", "project root (default: CWD)")
	c.Flags().Bool("force", false, "overwrite existing bootstrap")
	c.Flags().String("reasoning", "", "model ID for deep phases (e.g. claude-opus-4-7)")
	c.Flags().String("fast", "", "model ID for fast phases (e.g. claude-sonnet-4-6)")
	return c
}

// installEmbeddedArtifacts writes embedded agents/skills/commands/phases
// into <project-root>/.claude/ and records each in installed_artifacts.
// Called by bootstrap. Standalone path (without bootstrap) = `codedungeon install`.
func installEmbeddedArtifacts(s *db.Store) error {
	cwd, _ := os.Getwd()
	root := ResolveProjectRoot(cwd)
	embedded, err := prompts.Artifacts()
	if err != nil {
		return err
	}
	for _, a := range embedded {
		disk := filepath.Join(root, ".claude", a.RelPath)
		if err := os.MkdirAll(filepath.Dir(disk), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(disk, a.Content, 0o644); err != nil {
			return err
		}
		_ = s.UpsertArtifact(db.InstalledArtifact{
			RelPath:       a.RelPath,
			SHA256:        sha256Hex(a.Content),
			BinaryVersion: versionString(),
			InstalledAt:   time.Now().Unix(),
		})
	}
	return nil
}

// copyFile copies src → dst with perm.
func copyFile(src, dst string, perm os.FileMode) error {
	if src == dst {
		return errors.New("src and dst are the same path")
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// versionString reads main.Version via a hack — cmd package can't import main.
// We expose it through a settable var, wired from main.go if needed. For now
// return "unknown" if not overridden.
var versionOverride string

func versionString() string {
	if versionOverride != "" {
		return versionOverride
	}
	return "unknown"
}

// SetVersion lets main.go pass its Version into cmd package for bootstrap meta.
func SetVersion(v string) { versionOverride = v }
