package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/cartographer"
)

// MapCmd scans a codebase into file tree and estimated token counts.
func MapCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "map <path>",
		Short: "Scan codebase - file tree + estimated tokens",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			abs, err := filepath.Abs(path)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			format, _ := c.Flags().GetString("format")
			maxTokens, _ := c.Flags().GetInt("max-tokens")
			result, err := cartographer.Scan(abs, cartographer.Options{MaxTokens: maxTokens})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			switch format {
			case "tree":
				fmt.Print(cartographer.RenderTree(result))
				return nil
			case "compact":
				fmt.Print(cartographer.RenderCompact(result))
				return nil
			default:
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(result)
			}
		},
	}
	c.Flags().String("format", "json", "json | tree | compact")
	c.Flags().Int("max-tokens", 50000, "skip files exceeding this token estimate")
	return c
}
