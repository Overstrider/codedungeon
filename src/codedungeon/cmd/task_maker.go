package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/taskmaker"
)

func TaskMakerCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "task-maker",
		Short: "Render Task Maker design and run-full prompt artifacts",
	}
	c.AddCommand(taskMakerRenderCmd())
	return c
}

func taskMakerRenderCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "render",
		Short: "Render a confirmed Task Maker request",
		RunE: func(c *cobra.Command, _ []string) error {
			input, _ := c.Flags().GetString("input")
			out, _ := c.Flags().GetString("out")
			surface, _ := c.Flags().GetString("surface")
			printOutput, _ := c.Flags().GetBool("print")
			if input == "" {
				return fmt.Errorf("--input is required")
			}
			if out == "" {
				return fmt.Errorf("--out is required")
			}
			body, err := os.ReadFile(input)
			if err != nil {
				return err
			}
			var req taskmaker.Request
			if err := json.Unmarshal(body, &req); err != nil {
				return err
			}
			surface = strings.ToLower(strings.TrimSpace(surface))
			if surface == "auto" {
				surface = provider.Detect().Name()
			}
			result, err := taskmaker.RenderWithSurface(req, out, surface)
			if err != nil {
				return err
			}
			if printOutput {
				_, err = fmt.Fprint(c.OutOrStdout(), result.Output)
				return err
			}
			return EmitJSON(map[string]any{"ok": true, "out": result.OutDir})
		},
	}
	c.Flags().String("input", "", "Task Maker request JSON")
	c.Flags().String("out", "", "output directory for rendered artifacts")
	c.Flags().String("surface", "auto", "command surface to print: auto, codex, or claude")
	c.Flags().Bool("print", false, "print output.md content to stdout")
	return c
}
