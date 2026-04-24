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

// BootstrapResult holds the outcome of a RunBootstrap call.
type BootstrapResult struct {
	ProjectRoot        string
	BinPath            string
	DBPath             string
	OS                 string
	PromptsSeeded      []string
	ArtifactsInstalled int
	CDVersion          string
	Reasoning          string
	Fast               string
}

// RunBootstrap performs the core project-level bootstrap.
// Extracted so both BootstrapCmd (M2M) and SetupCmd (interactive) can call it.
func RunBootstrap(target, reasoning, fast string, force bool) (*BootstrapResult, error) {
	claudeDir := filepath.Join(target, ".claude")
	binDir := filepath.Join(claudeDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", binDir, err)
	}
	dbPath := filepath.Join(claudeDir, "codedungeon.db")
	binPath := filepath.Join(binDir, "codedungeon"+osadapter.Detect().ExecutableExt())

	if _, err := os.Stat(dbPath); err == nil && !force {
		return nil, fmt.Errorf("already bootstrapped: %s (use force to overwrite)", dbPath)
	}

	srcBin, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve running binary: %w", err)
	}
	if err := copyFile(srcBin, binPath, 0o755); err != nil {
		return nil, fmt.Errorf("copy binary: %w", err)
	}

	s, err := OpenDBNoGuard(dbPath)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		return nil, err
	}

	seeded, err := seedEmbeddedPrompts(s)
	if err != nil {
		return nil, err
	}

	artifactsInstalled := 0
	if err := installEmbeddedArtifacts(s); err == nil {
		if all, err := prompts.Artifacts(); err == nil {
			artifactsInstalled = len(all)
		}
	}

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
			return nil, err
		}
	}

	// Upsert codedungeon quick-reference in CLAUDE.md (best-effort).
	claudeMD := filepath.Join(target, "CLAUDE.md")
	if err := upsertCodedungeonSection(claudeMD); err != nil {
		fmt.Fprintln(os.Stderr, "[WARN] CLAUDE.md upsert failed:", err)
	}

	return &BootstrapResult{
		ProjectRoot:        target,
		BinPath:            binPath,
		DBPath:             dbPath,
		OS:                 ad.OS(),
		PromptsSeeded:      seeded,
		ArtifactsInstalled: artifactsInstalled,
		CDVersion:          versionString(),
		Reasoning:          reasoning,
		Fast:               fast,
	}, nil
}

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

			if IsHomeClaude(target) {
				return EmitPreflightErr(ErrHomeClaude)
			}
			if !HasGit(target) {
				return EmitPreflightErr(ErrNoGit)
			}

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

			result, err := RunBootstrap(target, reasoning, fast, force)
			if err != nil {
				return EmitErr(err.Error(), "")
			}

			return EmitJSON(map[string]any{
				"ok":                  true,
				"bootstrapped":        true,
				"project_root":        result.ProjectRoot,
				"bin":                 result.BinPath,
				"db":                  result.DBPath,
				"os":                  result.OS,
				"prompts_seeded":      result.PromptsSeeded,
				"artifacts_installed": result.ArtifactsInstalled,
				"cd_version":          result.CDVersion,
				"models": map[string]string{
					"reasoning": result.Reasoning,
					"fast":      result.Fast,
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
