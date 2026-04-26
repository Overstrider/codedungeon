package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/cmd"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/osadapter"
	"github.com/loldinis/codedungeon/internal/provider"
)

// Version is set via -ldflags "-X main.Version=v0.3.0" at build time.
var Version = "dev"

func main() {
	cmd.SetVersion(Version)
	p := provider.Detect()

	root := &cobra.Command{
		Use:           "codedungeon",
		Short:         "codedungeon - deterministic CLI for codedungeon pipeline",
		Long:          fmt.Sprintf("Single Go binary. SQLite (FTS5) state. Embedded+versionable prompts. Project-scoped for provider %s.", p.Name()),
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().Bool("human", false, "human-readable output (default: JSON)")
	root.PersistentFlags().String("db", "", fmt.Sprintf("path to codedungeon.db (default: <project>/%s)", p.DBPath()))

	root.AddCommand(versionCmd())
	root.AddCommand(cmd.DBCmd())
	root.AddCommand(cmd.PhaseCmd())
	root.AddCommand(cmd.PromptsCmd())
	root.AddCommand(cmd.ReportCmd())
	root.AddCommand(cmd.GitCmd())
	root.AddCommand(cmd.RepoCmd())
	root.AddCommand(cmd.BootstrapCmd())
	root.AddCommand(cmd.ReviewCmd())
	root.AddCommand(cmd.PlanCmd())
	root.AddCommand(cmd.QACmd())
	root.AddCommand(cmd.CleanupCmd())
	root.AddCommand(cmd.ConfigCmd())
	root.AddCommand(cmd.InstallCmd())
	root.AddCommand(cmd.MigrateCmd())
	root.AddCommand(cmd.StatusCmd())
	root.AddCommand(cmd.SpawnCmd())
	root.AddCommand(cmd.MapCmd())
	root.AddCommand(cmd.RulesCmd())
	root.AddCommand(cmd.HooksCmd())
	root.AddCommand(cmd.SetupCmd())

	root.RunE = func(c *cobra.Command, args []string) error {
		cwd, _ := os.Getwd()
		projectRoot := cmd.ResolveProjectRoot(cwd)
		handled, err := cmd.HandleFirstRun(projectRoot)
		if err != nil {
			return err
		}
		if handled {
			return nil
		}
		return c.Help()
	}

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print codedungeon version + schema + runtime info",
		RunE: func(c *cobra.Command, _ []string) error {
			ad := osadapter.Detect()
			out := map[string]any{
				"binary":         Version,
				"schema_version": db.SchemaVersion,
				"go_version":     runtime.Version(),
				"os":             ad.OS(),
				"arch":           runtime.GOARCH,
			}
			human, _ := c.Flags().GetBool("human")
			if human {
				fmt.Printf("codedungeon %s (schema v%s) %s/%s on %s\n",
					out["binary"], out["schema_version"], out["os"], out["arch"], out["go_version"])
				return nil
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		},
	}
}
