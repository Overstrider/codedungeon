package cmd

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/preflight"
)

type diagnosticReport = preflight.Report
type diagnosticCheck = preflight.Check

func DiagnoseCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "diagnose",
		Short: "Diagnose CodeDungeon environment readiness",
		RunE: func(c *cobra.Command, _ []string) error {
			strict, _ := c.Flags().GetBool("strict")
			report, err := buildDiagnosticsWithOptions(c.Context(), currentProjectRoot(), strict)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(report)
		},
	}
	c.Flags().Bool("strict", false, "treat WARN checks as not ready")
	return c
}

func buildDiagnostics(root string) (diagnosticReport, error) {
	return buildDiagnosticsWithOptions(context.Background(), root, false)
}

func buildDiagnosticsWithOptions(ctx context.Context, root string, strict bool) (diagnosticReport, error) {
	return preflight.Run(ctx, preflight.Request{Root: root, Strict: strict})
}
