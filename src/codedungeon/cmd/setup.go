package cmd

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/manifest"
	"github.com/loldinis/codedungeon/internal/osadapter"
	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/provider"
)

func buildModelTiers() []struct {
	Label           string
	Reasoning       string
	ReasoningEffort string
	Fast            string
	FastEffort      string
} {
	alts := provider.Detect().ModelAlternatives()
	var tiers []struct {
		Label           string
		Reasoning       string
		ReasoningEffort string
		Fast            string
		FastEffort      string
	}
	for i, a := range alts {
		label := a.Reasoning + " + " + a.Fast
		if a.ReasoningEffort != "" || a.FastEffort != "" {
			label = a.Reasoning + " (" + a.ReasoningEffort + ") + " + a.Fast + " (" + a.FastEffort + ")"
		}
		if i == 0 {
			label += "  [recommended]"
		}
		tiers = append(tiers, struct {
			Label           string
			Reasoning       string
			ReasoningEffort string
			Fast            string
			FastEffort      string
		}{label, a.Reasoning, a.ReasoningEffort, a.Fast, a.FastEffort})
	}
	return tiers
}

func SetupCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "setup",
		Short: "One-step project + global setup (interactive)",
		Long: `Download the binary, run 'codedungeon setup' in your git project — done.
Initializes the project DB, installs provider-native bootstrap files plus .codedungeon runtime artifacts, and lets you pick model tiers interactively.
Providers with global setup needs also install provider support.`,
		RunE: runSetup,
	}
	c.Flags().String("target", "", "project root (default: CWD)")
	c.Flags().String("reasoning", "", "reasoning model ID (skip interactive selection)")
	c.Flags().String("reasoning-effort", "", "reasoning effort (skip interactive selection)")
	c.Flags().String("fast", "", "fast model ID (skip interactive selection)")
	c.Flags().String("fast-effort", "", "fast effort (skip interactive selection)")
	c.Flags().Bool("force", false, "overwrite existing setup")
	c.Flags().Bool("skip-global", false, "skip global provider setup")
	c.Flags().BoolP("yes", "y", false, "accept all defaults, no interactive prompts")
	return c
}

type setupOptions struct {
	Target          string
	Reasoning       string
	ReasoningEffort string
	Fast            string
	FastEffort      string
	Force           bool
	SkipGlobal      bool
	Yes             bool
}

func runSetup(c *cobra.Command, _ []string) error {
	target, _ := c.Flags().GetString("target")
	reasoning, _ := c.Flags().GetString("reasoning")
	reasoningEffort, _ := c.Flags().GetString("reasoning-effort")
	fast, _ := c.Flags().GetString("fast")
	fastEffort, _ := c.Flags().GetString("fast-effort")
	force, _ := c.Flags().GetBool("force")
	skipGlobal, _ := c.Flags().GetBool("skip-global")
	yes, _ := c.Flags().GetBool("yes")
	return runSetupWithOptions(setupOptions{
		Target:          target,
		Reasoning:       reasoning,
		ReasoningEffort: reasoningEffort,
		Fast:            fast,
		FastEffort:      fastEffort,
		Force:           force,
		SkipGlobal:      skipGlobal,
		Yes:             yes,
	})
}

func runSetupWithOptions(opts setupOptions) error {
	target := opts.Target
	reasoning := opts.Reasoning
	reasoningEffort := opts.ReasoningEffort
	fast := opts.Fast
	fastEffort := opts.FastEffort
	force := opts.Force
	skipGlobal := opts.SkipGlobal
	yes := opts.Yes

	interactive := isTTY() && !yes

	if target == "" {
		cwd, _ := os.Getwd()
		target = cwd
	}
	target, _ = filepath.Abs(target)

	const totalSteps = 5

	// ---- Step 1: Environment check ----
	if interactive {
		printBanner(versionString())
		printStep(1, totalSteps, "Checking environment...")
		printDetail(fmt.Sprintf("OS: %s/%s", runtime.GOOS, runtime.GOARCH))
		printDetail(fmt.Sprintf("Project: %s", target))
	}

	if IsHomeConfig(target) {
		if interactive {
			printErr("Cannot run inside provider config directory. cd to a project directory.")
		}
		return EmitPreflightErr(ErrHomeConfig)
	}

	if !HasGit(target) {
		if interactive {
			printErr("No git repository found. Run 'git init' first.")
		}
		return EmitPreflightErr(ErrNoGit)
	}

	if interactive {
		printDetail("Git: found")
	}

	info, _ := manifest.Detect(target)
	if interactive {
		if info.Stack != "" {
			printDetail(fmt.Sprintf("Stack: %s (%s)", info.Manifest, info.Stack))
		} else {
			printDetail("Stack: (not detected)")
		}
	}

	// ---- Step 2: Global provider setup ----
	if interactive {
		fmt.Fprintln(tuiOut)
		printStep(2, totalSteps, "Global provider setup...")
	}

	globalStatus := ""
	if !skipGlobal {
		status, err := installGlobalPlugin()
		if err != nil {
			globalStatus = "failed: " + err.Error()
			if interactive {
				printWarn("Plugin install failed: " + err.Error())
				printWarn("CLI works, but provider-native command shortcuts may not be available.")
			}
		} else {
			globalStatus = status
			if interactive {
				printOK("Plugin:", provider.Detect().PluginDir()+" ["+status+"]")
			}
		}
	} else {
		globalStatus = "skipped"
		if interactive {
			printDetail("Skipped (--skip-global)")
		}
	}

	codexFeatureStatus := ""
	if provider.Detect().Name() == "codex" {
		if skipGlobal {
			codexFeatureStatus = "skipped"
		} else {
			status, err := enableCodexMultiAgentV2()
			if err != nil {
				codexFeatureStatus = "failed: " + err.Error()
				if interactive {
					printWarn("Codex custom agents feature flag failed: " + err.Error())
					printWarn("Start Codex with '--enable multi_agent_v2' if custom subagents are rejected.")
				}
			} else {
				codexFeatureStatus = status
				if interactive {
					printOK("Codex custom agents:", status)
				}
			}
		}
	}

	// ---- Step 3: Model selection ----
	if interactive {
		fmt.Fprintln(tuiOut)
		printStep(3, totalSteps, "Model configuration")
	}

	if reasoning == "" || fast == "" || reasoningEffort == "" || fastEffort == "" {
		if interactive {
			tiers := buildModelTiers()
			var labels []string
			for _, t := range tiers {
				labels = append(labels, t.Label)
			}
			choice := promptChoice("Select model tier:", labels, 0)
			if reasoning == "" {
				reasoning = tiers[choice].Reasoning
			}
			if reasoningEffort == "" {
				reasoningEffort = tiers[choice].ReasoningEffort
			}
			if fast == "" {
				fast = tiers[choice].Fast
			}
			if fastEffort == "" {
				fastEffort = tiers[choice].FastEffort
			}
		} else {
			defaults := provider.Detect().DefaultModels()
			if reasoning == "" {
				reasoning = defaults.Reasoning
			}
			if reasoningEffort == "" {
				reasoningEffort = defaults.ReasoningEffort
			}
			if fast == "" {
				fast = defaults.Fast
			}
			if fastEffort == "" {
				fastEffort = defaults.FastEffort
			}
		}
	}
	cfg, err := completeModelConfig(reasoning, reasoningEffort, fast, fastEffort)
	if err != nil {
		return EmitErr(err.Error(), "effort must be one of: low, medium, high, xhigh")
	}

	if interactive {
		printDetail(fmt.Sprintf("Reasoning: %s (%s)", cfg.Reasoning, cfg.ReasoningEffort))
		printDetail(fmt.Sprintf("Fast:      %s (%s)", cfg.Fast, cfg.FastEffort))
	}

	// ---- Step 4: Project bootstrap ----
	if interactive {
		fmt.Fprintln(tuiOut)
		printStep(4, totalSteps, "Project bootstrap...")
	}

	// Check if already bootstrapped
	dbPath := filepath.Join(target, provider.Detect().DBPath())
	if _, err := os.Stat(dbPath); err == nil && !force {
		if interactive {
			if !promptYesNo("Already bootstrapped. Re-bootstrap?", false) {
				printDetail("Skipped. Use --force to overwrite.")
				printStep(5, totalSteps, "Done! (no changes)")
				fmt.Fprintln(tuiOut)
				return nil
			}
			force = true
		} else {
			if !yes {
				return EmitErr("already bootstrapped: "+dbPath, "use --force to overwrite")
			}
			return EmitJSON(map[string]any{
				"ok":                   true,
				"already_bootstrapped": true,
				"project_root":         target,
				"db":                   dbPath,
				"global_plugin":        globalStatus,
				"codex_multi_agent_v2": codexFeatureStatus,
			})
		}
	}

	result, err := RunBootstrapWithConfig(target, cfg, force)
	if err != nil {
		if interactive {
			printErr(err.Error())
		}
		return EmitErr(err.Error(), "")
	}

	if interactive {
		printOK("Binary:", provider.Detect().BinDir()+"/codedungeon [copied]")
		printOK("Database:", provider.Detect().DBPath()+" [created]")
		printOK("Prompts:", fmt.Sprintf("%d seeded", len(result.PromptsSeeded)))
		printOK("Artifacts:", fmt.Sprintf("%d installed", result.ArtifactsInstalled))
	}

	// ---- Step 5: Done ----
	if interactive {
		fmt.Fprintln(tuiOut)
		printStep(5, totalSteps, ansi("32", "Done!"))
		fmt.Fprintln(tuiOut)
		printDetail(fmt.Sprintf("codedungeon is ready in %s", target))
		printDetail("Run 'codedungeon version' to verify.")
		if provider.Detect().Name() == "claude" {
			printDetail("Claude Code slash commands are now available.")
			printDetail("Recommended next step: /codedungeon --rules")
		} else {
			printDetail("Codex workflow skills are installed under .agents/skills.")
			printDetail("Recommended next step: $codedungeon --rules")
		}
		fmt.Fprintln(tuiOut)
	} else {
		_ = EmitJSON(map[string]any{
			"ok":                   true,
			"project_root":         result.ProjectRoot,
			"bin":                  result.BinPath,
			"db":                   result.DBPath,
			"global_plugin":        globalStatus,
			"codex_multi_agent_v2": codexFeatureStatus,
			"prompts_seeded":       len(result.PromptsSeeded),
			"artifacts_installed":  result.ArtifactsInstalled,
			"models": map[string]string{
				"reasoning":        result.Reasoning,
				"reasoning_effort": result.ReasoningEffort,
				"fast":             result.Fast,
				"fast_effort":      result.FastEffort,
			},
		})
	}

	return nil
}

func enableCodexMultiAgentV2() (string, error) {
	if _, err := exec.LookPath("codex"); err != nil {
		return "", fmt.Errorf("codex CLI not found: %w", err)
	}
	cmd := exec.Command("codex", "features", "enable", "multi_agent_v2")
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return "", fmt.Errorf("%w: %s", err, msg)
		}
		return "", err
	}
	return "enabled", nil
}

func installGlobalPlugin() (string, error) {
	p := provider.Detect()
	if !p.HasPluginSystem() {
		return "skipped (no plugin system)", nil
	}
	plugDir := p.PluginDir()
	binDir := filepath.Join(plugDir, "bin")
	skillDir := filepath.Join(plugDir, "skills", "grimoire-cli")
	manifestDir := filepath.Join(plugDir, ".claude-plugin")

	for _, d := range []string{binDir, skillDir, manifestDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return "", fmt.Errorf("mkdir %s: %w", d, err)
		}
	}

	srcBin, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve binary: %w", err)
	}

	ext := osadapter.Detect().ExecutableExt()
	dstBin := filepath.Join(binDir, "codedungeon"+ext)

	status := "installed"
	if existing, err := os.Stat(dstBin); err == nil && existing.Size() > 0 {
		if filesMatch(srcBin, dstBin) {
			status = "up to date"
		} else {
			status = "updated"
		}
	}

	if status != "up to date" {
		if err := copyFile(srcBin, dstBin, 0o755); err != nil {
			return "", fmt.Errorf("copy binary: %w", err)
		}
	}

	skillContent, err := prompts.GetRaw("skills/grimoire-cli/SKILL.md")
	if err != nil {
		return "", fmt.Errorf("read embedded SKILL.md: %w", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), skillContent, 0o644); err != nil {
		return "", fmt.Errorf("write SKILL.md: %w", err)
	}

	pluginJSON := p.PluginManifest(versionString())
	if err := os.WriteFile(filepath.Join(manifestDir, "plugin.json"), pluginJSON, 0o644); err != nil {
		return "", fmt.Errorf("write plugin.json: %w", err)
	}

	return status, nil
}

func filesMatch(a, b string) bool {
	ha := fileHash(a)
	hb := fileHash(b)
	return ha != "" && ha == hb
}

func fileHash(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}
