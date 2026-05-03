package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/osadapter"
	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/provider"
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
	ReasoningEffort    string
	Fast               string
	FastEffort         string
}

// RunBootstrap performs the core project-level bootstrap.
// Extracted so both BootstrapCmd (M2M) and SetupCmd (interactive) can call it.
func RunBootstrap(target, reasoning, fast string, force bool) (*BootstrapResult, error) {
	cfg, err := completeModelConfig(reasoning, "", fast, "")
	if err != nil {
		return nil, err
	}
	return RunBootstrapWithConfig(target, cfg, force)
}

func RunBootstrapWithConfig(target string, cfg provider.ModelConfig, force bool) (*BootstrapResult, error) {
	p := provider.Detect()
	dbPath := filepath.Join(target, p.DBPath())
	_, dbStatErr := os.Stat(dbPath)
	dbExistedBeforeMigration := dbStatErr == nil
	if err := migrateLegacyRuntimeState(target, p); err != nil {
		return nil, err
	}
	if err := ensureRuntimeState(target, p); err != nil {
		return nil, err
	}
	binDir := filepath.Join(target, p.BinDir())
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return nil, fmt.Errorf("cannot create %s: %w", binDir, err)
	}
	binPath := filepath.Join(binDir, "codedungeon"+osadapter.Detect().ExecutableExt())

	if _, err := os.Stat(dbPath); err == nil && !force && dbExistedBeforeMigration {
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
	if err := installEmbeddedArtifactsAt(s, target); err != nil {
		return nil, err
	}
	if all, err := prompts.Artifacts(); err == nil {
		artifactsInstalled = len(all)
	}

	ad := osadapter.Detect()
	meta := map[string]string{
		"os":                     ad.OS(),
		"project_root":           target,
		"cd_version":             versionString(),
		"bootstrapped_at":        fmt.Sprintf("%d", time.Now().Unix()),
		"model_reasoning":        cfg.Reasoning,
		"model_reasoning_effort": cfg.ReasoningEffort,
		"model_fast":             cfg.Fast,
		"model_fast_effort":      cfg.FastEffort,
	}
	if provider.Detect().Name() == "claude" && strings.TrimSpace(cfg.Reasoning) != "" && strings.TrimSpace(cfg.Reasoning) == strings.TrimSpace(cfg.Fast) {
		meta["model_lock"] = strings.TrimSpace(cfg.Reasoning)
	} else {
		meta["model_lock"] = ""
	}
	for k, v := range meta {
		if err := s.SetMeta(k, v); err != nil {
			return nil, err
		}
	}

	// Upsert codedungeon quick-reference in provider instruction file (best-effort).
	agentConfig := filepath.Join(target, p.AgentConfigFile())
	if err := upsertCodedungeonSection(agentConfig); err != nil {
		fmt.Fprintf(os.Stderr, "[WARN] %s upsert failed: %v\n", p.AgentConfigFile(), err)
	}

	return &BootstrapResult{
		ProjectRoot:        target,
		BinPath:            binPath,
		DBPath:             dbPath,
		OS:                 ad.OS(),
		PromptsSeeded:      seeded,
		ArtifactsInstalled: artifactsInstalled,
		CDVersion:          versionString(),
		Reasoning:          cfg.Reasoning,
		ReasoningEffort:    cfg.ReasoningEffort,
		Fast:               cfg.Fast,
		FastEffort:         cfg.FastEffort,
	}, nil
}

// BootstrapCmd self-copies codedungeon into the provider bin dir, creates
// the DB, and records project_root + os + cd_version in meta. First-run flow.
func BootstrapCmd() *cobra.Command {
	p := provider.Detect()
	c := &cobra.Command{
		Use:   "bootstrap",
		Short: fmt.Sprintf("First-run: copy binary into <project>/%s + init DB (requires .git)", p.BinDir()),
		Long: fmt.Sprintf(`Run once per project. codedungeon refuses to run inside provider home config paths (%s).
If --target is omitted, the current CWD is used as the project root.
Requires .git at the target (or --init-git to create one - not default).`,
			p.ConfigDir()),
		RunE: func(c *cobra.Command, _ []string) error {
			target, _ := c.Flags().GetString("target")
			force, _ := c.Flags().GetBool("force")
			reasoning, _ := c.Flags().GetString("reasoning")
			reasoningEffort, _ := c.Flags().GetString("reasoning-effort")
			fast, _ := c.Flags().GetString("fast")
			fastEffort, _ := c.Flags().GetString("fast-effort")
			if target == "" {
				cwd, _ := os.Getwd()
				target = cwd
			}
			target, _ = filepath.Abs(target)

			if IsHomeConfig(target) {
				return EmitPreflightErr(ErrHomeConfig)
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
			cfg, err := completeModelConfig(reasoning, reasoningEffort, fast, fastEffort)
			if err != nil {
				return EmitErr(err.Error(), "effort must be one of: low, medium, high, xhigh")
			}

			result, err := RunBootstrapWithConfig(target, cfg, force)
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
					"reasoning":        result.Reasoning,
					"reasoning_effort": result.ReasoningEffort,
					"fast":             result.Fast,
					"fast_effort":      result.FastEffort,
				},
			})
		},
	}
	c.Flags().String("target", "", "project root (default: CWD)")
	c.Flags().Bool("force", false, "overwrite existing bootstrap")
	c.Flags().String("reasoning", "", "model ID for deep phases")
	c.Flags().String("reasoning-effort", "", "reasoning effort for deep phases")
	c.Flags().String("fast", "", "model ID for fast phases")
	c.Flags().String("fast-effort", "", "reasoning effort for fast phases")
	return c
}

// installEmbeddedArtifacts writes provider-native embedded artifacts and
// records each in installed_artifacts.
// Called by bootstrap. Standalone path (without bootstrap) = `codedungeon install`.
func installEmbeddedArtifacts(s *db.Store) error {
	cwd, _ := os.Getwd()
	root := ResolveProjectRoot(cwd)
	return installEmbeddedArtifactsAt(s, root)
}

func installEmbeddedArtifactsAt(s *db.Store, root string) error {
	embedded, err := prompts.Artifacts()
	if err != nil {
		return err
	}
	if err := prepareCommandArtifactInstall(root, provider.Detect(), embedded); err != nil {
		return err
	}
	for _, a := range embedded {
		disk := filepath.Join(root, filepath.FromSlash(a.InstallPath))
		if err := os.MkdirAll(filepath.Dir(disk), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(disk, a.Content, 0o644); err != nil {
			return err
		}
		_ = s.UpsertArtifact(db.InstalledArtifact{
			RelPath:       a.RelPath,
			InstallPath:   a.InstallPath,
			SHA256:        sha256Hex(a.Content),
			BinaryVersion: versionString(),
			Provider:      a.Provider,
			PackID:        a.PackID,
			PackVersion:   a.PackVersion,
			Kind:          a.Kind,
			LogicalName:   a.LogicalName,
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
