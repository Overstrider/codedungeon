package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/provider"
)

var prov = provider.Detect()

var ModelDefaults = prov.DefaultModels()

var ModelAlternatives = prov.ModelAlternatives()

func ConfigCmd() *cobra.Command {
	c := &cobra.Command{Use: "config", Short: "Read/write pipeline config (models, etc.)"}
	c.AddCommand(configModelsCmd())
	c.AddCommand(configModelCmd())
	c.AddCommand(configSetModelsCmd())
	return c
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
			fast, _ := s.GetMeta("model_fast")
			return EmitJSON(map[string]any{
				"ok":        true,
				"reasoning": reasoning,
				"fast":      fast,
				"defaults":  ModelDefaults,
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
			tier := args[0]
			key := "model_" + tier
			if tier != "reasoning" && tier != "fast" {
				return EmitErr("unknown tier: "+tier, "use `reasoning` or `fast`")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			v, _ := s.GetMeta(key)
			if v == "" {
				return EmitErr("model "+tier+" not configured", "run `codedungeon config set-models --reasoning X --fast Y` or re-bootstrap with flags")
			}
			fmt.Print(v)
			return nil
		},
	}
}

func configSetModelsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "set-models",
		Short: "Write model_reasoning + model_fast into meta",
		RunE: func(c *cobra.Command, _ []string) error {
			reasoning, _ := c.Flags().GetString("reasoning")
			fast, _ := c.Flags().GetString("fast")
			if reasoning == "" || fast == "" {
				return EmitErr("--reasoning and --fast required", "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.SetMeta("model_reasoning", reasoning); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := s.SetMeta("model_fast", fast); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":        true,
				"reasoning": reasoning,
				"fast":      fast,
			})
		},
	}
	c.Flags().String("reasoning", "", "model ID for deep thinking tier")
	c.Flags().String("fast", "", "model ID for fast/cheap tier")
	return c
}
