package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/provider"
)

var phaseTier = map[string]string{
	"0": "fast", "1": "reasoning", "2'": "reasoning", "3.5": "fast",
	"4": "reasoning", "5": "fast", "5.5": "fast", "5.6": "reasoning",
	"6": "fast", "7": "fast",
}

var phaseThinking = map[string]int{
	"0": 0, "1": 32000, "2'": 8000, "3.5": 2000, "4": 32000,
	"5": 2000, "5.5": 2000, "5.6": 32000, "6": 2000, "7": 0,
}

var phaseFileNames = map[string]string{
	"0":   "entrance-hall-validation.md",
	"1":   "war-room-architect.md",
	"2'":  "guild-quarter-domain.md",
	"3.5": "trap-workshop-qa.md",
	"4":   "armory-decomposition.md",
	"5":   "forge-execution.md",
	"5.5": "crucible-qa-refine.md",
	"5.6": "laboratory-test-decomp.md",
	"6":   "arena-tests.md",
	"7":   "throne-room-report.md",
}

func phaseFilePath(phase string) (string, bool) {
	name, ok := phaseFileNames[phase]
	if !ok {
		return "", false
	}
	return filepath.Join(provider.Detect().PhasesDir(), name), true
}

// SpawnCmd emits a ready-to-use spawn prompt for a phase agent.
// Baked-in: caveman-ultra block, phase file path, handoff-schema reminder,
// model tier, thinking budget, and the required `codedungeon phase done` hint.
// Orchestrator pastes this straight into the Task spawn argument.
func SpawnCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "spawn-prompt <phase>",
		Short: "Emit composed spawn prompt for a phase (caveman + phase file + handoff + model tier)",
		Long: `Returns a self-contained spawn prompt block. All deterministic plumbing
(caveman-ultra mode, phase file path, handoff schema reminder, model tier,
thinking budget, phase done recipe) is baked in so the orchestrator does
not have to narrate it.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			phase := args[0]
			file, ok := phaseFilePath(phase)
			if !ok {
				return EmitErr("unknown phase: "+phase, "valid: 0, 1, 2', 3.5, 4, 5, 5.5, 5.6, 6, 7")
			}
			tier := phaseTier[phase]
			think := phaseThinking[phase]

			// Resolve model from DB (falls back if absent).
			s, err := OpenDB(c)
			model := "(configure via: codedungeon config set-models)"
			if err == nil {
				defer s.Close()
				if v, _ := s.GetMeta("model_" + tier); v != "" {
					model = v
				}
			}

			cavemanBlk, err := prompts.Get("caveman-ultra")
			if err != nil {
				return EmitErr(err.Error(), "embedded caveman-ultra missing")
			}

			raw, _ := c.Flags().GetBool("raw")
			out := buildSpawnPromptForProvider(provider.Detect().Name(), phase, file, tier, model, think, cavemanBlk)
			if raw {
				fmt.Print(out)
				return nil
			}
			return EmitJSON(map[string]any{
				"ok":                  true,
				"phase":               phase,
				"phase_file":          file,
				"model_tier":          tier,
				"model":               model,
				"max_thinking_tokens": think,
				"prompt":              out,
			})
		},
	}
	c.Flags().Bool("raw", false, "emit only the prompt text (no JSON wrapper)")
	return c
}

func buildSpawnPrompt(phase, file, tier, model string, think int, caveman string) string {
	return buildSpawnPromptForProvider(provider.Detect().Name(), phase, file, tier, model, think, caveman)
}

func buildSpawnPromptForProvider(providerName, phase, file, tier, model string, think int, caveman string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "You are executing Phase %s of the codedungeon pipeline.\n\n", phase)
	fmt.Fprintf(&b, "Read your full phase instructions from: %s\n", file)
	fmt.Fprintf(&b, "Read prior-phase handoff (if any) via: codedungeon phase info <PREV_PHASE> --field rendered_md\n")
	fmt.Fprintf(&b, "Read pipeline state via: codedungeon phase info %s\n\n", phase)
	b.WriteString("When this phase is DONE, close it atomically:\n")
	fmt.Fprintf(&b, "  codedungeon phase done %s --summary \"<1-line caveman>\" --decisions ... --artifacts ... --next ... --promise \"PHASE_%s_COMPLETE[: ...]\"\n", phase, strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(phase, "'", "PRIME"), ".", "")))
	b.WriteString("Or skip/fail via: codedungeon phase {skip|fail} --reason \"...\"\n\n")
	b.WriteString("--- OUTPUT MODE (baked in, non-negotiable) ---\n")
	b.WriteString(strings.TrimSpace(caveman))
	b.WriteString("\n--- END OUTPUT MODE ---\n\n")
	if providerName == "claude" || providerName == "claude-code" || providerName == "claude-ce" {
		fmt.Fprintf(&b, "max_thinking_tokens: %d\n", think)
	}
	fmt.Fprintf(&b, "model: %s   # tier=%s (via codedungeon config model %s)\n", model, tier, tier)
	return b.String()
}
