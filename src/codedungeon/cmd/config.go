package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

var prov = provider.Detect()

var ModelDefaults = prov.DefaultModels()

var ModelAlternatives = prov.ModelAlternatives()

func ConfigCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Read/write pipeline config (models, etc.)"}
	c.AddCommand(configModelsCmd())
	c.AddCommand(configModelCmd())
	c.AddCommand(configEffortCmd())
	c.AddCommand(configSetModelsCmd())
	return c
}

func normalizeModelTier(tier string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(tier)) {
	case "reasoning", "architect":
		return "reasoning", nil
	case "fast", "execution", "exec":
		return "fast", nil
	default:
		return "", fmt.Errorf("unknown tier: %s", tier)
	}
}

func normalizeReasoningEffort(effort string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(effort)) {
	case "low":
		return "low", nil
	case "med", "medium":
		return "medium", nil
	case "high":
		return "high", nil
	case "xhigh":
		return "xhigh", nil
	default:
		return "", fmt.Errorf("unknown reasoning effort: %s", effort)
	}
}

func completeModelConfig(reasoning, reasoningEffort, fast, fastEffort string) (provider.ModelConfig, error) {
	defaults := provider.Detect().DefaultModels()
	cfg := provider.ModelConfig{
		Reasoning:       strings.TrimSpace(reasoning),
		ReasoningEffort: strings.TrimSpace(reasoningEffort),
		Fast:            strings.TrimSpace(fast),
		FastEffort:      strings.TrimSpace(fastEffort),
	}
	if cfg.Reasoning == "" {
		cfg.Reasoning = defaults.Reasoning
	}
	if cfg.ReasoningEffort == "" {
		cfg.ReasoningEffort = defaults.ReasoningEffort
	}
	if cfg.Fast == "" {
		cfg.Fast = defaults.Fast
	}
	if cfg.FastEffort == "" {
		cfg.FastEffort = defaults.FastEffort
	}
	if cfg.ReasoningEffort != "" {
		var err error
		cfg.ReasoningEffort, err = normalizeReasoningEffort(cfg.ReasoningEffort)
		if err != nil {
			return provider.ModelConfig{}, err
		}
	}
	if cfg.FastEffort != "" {
		var err error
		cfg.FastEffort, err = normalizeReasoningEffort(cfg.FastEffort)
		if err != nil {
			return provider.ModelConfig{}, err
		}
	}
	return cfg, nil
}

func configModelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "Show configured model tiers {reasoning, fast}",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			reasoning, _ := s.GetMeta("model_reasoning")
			reasoningEffort, _ := s.GetMeta("model_reasoning_effort")
			fast, _ := s.GetMeta("model_fast")
			fastEffort, _ := s.GetMeta("model_fast_effort")
			return EmitJSON(map[string]any{
				"ok":               true,
				"reasoning":        reasoning,
				"reasoning_effort": reasoningEffort,
				"fast":             fast,
				"fast_effort":      fastEffort,
				"defaults":         provider.Detect().DefaultModels(),
			})
		},
	}
}

func configModelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "model <tier>",
		Short: "Print a single model ID (tier = reasoning | fast) — bare string to stdout",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			tier, err := normalizeModelTier(args[0])
			if err != nil {
				return EmitErr(err.Error(), "use `reasoning|architect` or `fast|execution|exec`")
			}
			key := "model_" + tier
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			v, _ := s.GetMeta(key)
			if v == "" {
				return EmitErr("model "+tier+" not configured", "run `codedungeon config set-models --reasoning X --fast Y` or re-bootstrap with flags")
			}
			fmt.Fprint(c.OutOrStdout(), v)
			return nil
		},
	}
}

func configEffortCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "effort <tier>",
		Short: "Print a single reasoning effort (tier = reasoning|architect | fast|execution|exec)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			tier, err := normalizeModelTier(args[0])
			if err != nil {
				return EmitErr(err.Error(), "use `reasoning|architect` or `fast|execution|exec`")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			v, _ := s.GetMeta("model_" + tier + "_effort")
			if v == "" {
				defaults := provider.Detect().DefaultModels()
				if tier == "reasoning" {
					v = defaults.ReasoningEffort
				} else {
					v = defaults.FastEffort
				}
			}
			if v == "" {
				return EmitErr("effort "+tier+" not configured", "run `codedungeon config set-models --reasoning-effort X --fast-effort Y` or re-bootstrap with flags")
			}
			fmt.Fprint(c.OutOrStdout(), v)
			return nil
		},
	}
}

func configSetModelsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "set-models",
		Short: "Write model_reasoning + model_fast into meta",
		RunE: func(c *cobra.Command, _ []string) error {
			cfg, err := completeModelConfigFromFlags(c)
			if err != nil {
				return EmitErr(err.Error(), "effort must be one of: low, medium, high, xhigh")
			}
			if cfg.Reasoning == "" || cfg.Fast == "" {
				return EmitErr("--reasoning and --fast required", "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.SetMeta("model_reasoning", cfg.Reasoning); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := s.SetMeta("model_reasoning_effort", cfg.ReasoningEffort); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := s.SetMeta("model_fast", cfg.Fast); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := s.SetMeta("model_fast_effort", cfg.FastEffort); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":               true,
				"reasoning":        cfg.Reasoning,
				"reasoning_effort": cfg.ReasoningEffort,
				"fast":             cfg.Fast,
				"fast_effort":      cfg.FastEffort,
			})
		},
	}
	c.Flags().String("reasoning", "", "model ID for deep thinking tier")
	c.Flags().String("reasoning-effort", "", "reasoning effort for deep thinking tier")
	c.Flags().String("fast", "", "model ID for fast/cheap tier")
	c.Flags().String("fast-effort", "", "reasoning effort for fast/cheap tier")
	return c
}

func completeModelConfigFromFlags(c *cobra.Command) (provider.ModelConfig, error) {
	reasoning, _ := c.Flags().GetString("reasoning")
	reasoningEffort, _ := c.Flags().GetString("reasoning-effort")
	fast, _ := c.Flags().GetString("fast")
	fastEffort, _ := c.Flags().GetString("fast-effort")
	return completeModelConfig(reasoning, reasoningEffort, fast, fastEffort)
}

func ensureModelConfigDefaults(s *db.Store) error {
	defaults := provider.Detect().DefaultModels()
	pairs := map[string]string{
		"model_reasoning":        defaults.Reasoning,
		"model_reasoning_effort": defaults.ReasoningEffort,
		"model_fast":             defaults.Fast,
		"model_fast_effort":      defaults.FastEffort,
	}
	for key, value := range pairs {
		if value == "" {
			continue
		}
		current, err := s.GetMeta(key)
		if err != nil {
			return err
		}
		if current == "" {
			if err := s.SetMeta(key, value); err != nil {
				return err
			}
		}
	}
	return nil
}
