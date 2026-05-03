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

var phaseAgentType = map[string]string{
	"0":   "cd_architect_planner",
	"1":   "cd_architect_planner",
	"2'":  "cd_backend_planner",
	"3.5": "cd_qa_planner",
	"4":   "cd_task_architect",
	"5":   "cd_dev_worker",
	"5.5": "cd_review_validator",
	"5.6": "cd_test_reviewer",
	"6":   "cd_api_tester",
	"7":   "cd_review_validator",
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
			model := fmt.Sprintf("(configure via: %s config set-models)", codedungeonCommandForProvider(provider.Detect().Name()))
			effort := defaultEffortForTier(tier)
			if err == nil {
				defer s.Close()
				if v, _ := s.GetMeta("model_" + tier); v != "" {
					model = v
				}
				if v, _ := s.GetMeta("model_" + tier + "_effort"); v != "" {
					effort = v
				}
			}

			cavemanBlk, err := prompts.Get("caveman-ultra")
			if err != nil {
				return EmitErr(err.Error(), "embedded caveman-ultra missing")
			}

			raw, _ := c.Flags().GetBool("raw")
			out := buildSpawnPromptForProviderWithEffort(provider.Detect().Name(), phase, file, tier, model, effort, think, cavemanBlk)
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
				"reasoning_effort":    effort,
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
	return buildSpawnPromptForProviderWithEffort(providerName, phase, file, tier, model, defaultEffortForProviderTier(providerName, tier), think, caveman)
}

func buildSpawnPromptForProviderWithEffort(providerName, phase, file, tier, model, effort string, think int, caveman string) string {
	var b strings.Builder
	cmdName := codedungeonCommandForProvider(providerName)
	fmt.Fprintf(&b, "You are executing Phase %s of the codedungeon pipeline.\n\n", phase)
	fmt.Fprintf(&b, "Read your full phase instructions from: %s\n", file)
	fmt.Fprintf(&b, "Read prior-phase handoff (if any) via: %s phase info <PREV_PHASE> --field rendered_md\n", cmdName)
	fmt.Fprintf(&b, "Read pipeline state via: %s phase info %s\n\n", cmdName, phase)
	b.WriteString("When this phase is DONE, close it atomically:\n")
	fmt.Fprintf(&b, "  %s phase done %s --summary \"<1-line caveman>\" --decisions ... --artifacts ... --next ... --promise \"PHASE_%s_COMPLETE[: ...]\"\n", cmdName, phase, strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(phase, "'", "PRIME"), ".", "")))
	fmt.Fprintf(&b, "Or skip/fail via: %s phase {skip|fail} --reason \"...\"\n\n", cmdName)
	b.WriteString("--- OUTPUT MODE (baked in, non-negotiable) ---\n")
	b.WriteString(strings.TrimSpace(caveman))
	b.WriteString("\n--- END OUTPUT MODE ---\n\n")
	if providerName == "claude" || providerName == "claude-code" || providerName == "claude-ce" {
		fmt.Fprintf(&b, "max_thinking_tokens: %d\n", think)
		fmt.Fprintf(&b, "claude_cli_args: %s\n", strings.Join(provider.Claude{}.RequiredCLIArgs(), " "))
	} else if agentType := phaseAgentType[phase]; agentType != "" {
		fmt.Fprintf(&b, "agent_type: %s\n", agentType)
	}
	fmt.Fprintf(&b, "model: %s   # tier=%s (via %s config model %s)\n", model, tier, cmdName, tier)
	if providerName == "codex" || providerName == "codex-cli" {
		fmt.Fprintf(&b, "reasoning_effort: %s   # tier=%s (via %s config effort %s)\n", effort, tier, cmdName, tier)
	}
	return b.String()
}

func defaultEffortForTier(tier string) string {
	return defaultEffortForProviderTier(provider.Detect().Name(), tier)
}

func defaultEffortForProviderTier(providerName, tier string) string {
	defaults := provider.Detect().DefaultModels()
	if providerName == "codex" || providerName == "codex-cli" {
		defaults = provider.Codex{}.DefaultModels()
	}
	if providerName == "claude" || providerName == "claude-code" || providerName == "claude-ce" {
		defaults = provider.Claude{}.DefaultModels()
	}
	if tier == "reasoning" {
		return defaults.ReasoningEffort
	}
	if tier == "fast" {
		return defaults.FastEffort
	}
	return ""
}

func codedungeonCommandForProvider(providerName string) string {
	switch providerName {
	case "codex", "codex-cli":
		return "./.codex/bin/codedungeon"
	case "claude", "claude-code", "claude-ce":
		return "./.claude/bin/codedungeon"
	default:
		return "codedungeon"
	}
}
