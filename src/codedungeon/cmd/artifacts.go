package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/db"
)

func ArtifactsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "artifacts",
		Short: "Inspect, verify, and backfill the runtime artifact registry",
	}
	c.AddCommand(artifactsListCmd())
	c.AddCommand(artifactsVerifyCmd())
	c.AddCommand(artifactsBackfillCmd())
	return c
}

func artifactsListCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "list",
		Short: "List registered runtime artifacts",
		RunE: func(c *cobra.Command, _ []string) error {
			s, runID, err := openArtifactsStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			limit, _ := c.Flags().GetInt("limit")
			module, _ := c.Flags().GetString("module")
			var rows []db.Artifact
			if runID > 0 {
				rows, err = s.ArtifactsByRun(runID)
			} else {
				rows, err = s.LatestArtifacts(limit)
			}
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			rows = filterArtifactsByModule(rows, module)
			if Human(c) {
				for _, row := range rows {
					fmt.Printf("%s\t%s\t%s\t%s\n", row.Module, row.Role, row.ArtifactType, row.Path)
				}
				return nil
			}
			return EmitJSON(map[string]any{"ok": true, "run_id": runID, "artifacts": rows})
		},
	}
	addArtifactRunFlags(c)
	c.Flags().String("module", "", "filter by module")
	c.Flags().Int("limit", 50, "latest artifact limit when no run is selected")
	return c
}

func artifactsVerifyCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "verify",
		Short: "Verify registered artifact files and directories still match",
		RunE: func(c *cobra.Command, _ []string) error {
			s, runID, err := openArtifactsStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			if runID <= 0 {
				return EmitErr("--run or an active run is required", "")
			}
			registry := artifactreg.NewRegistry(s, currentProjectRoot())
			results, err := registry.VerifyRun(runID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			missing, drifted := artifactVerificationCounts(results)
			ok := missing == 0 && drifted == 0
			if Human(c) {
				for _, result := range results {
					fmt.Printf("%s\t%s\t%s\n", result.Status, result.Artifact.Module, result.Artifact.Path)
				}
				if !ok {
					return fmt.Errorf("artifact verification failed: missing=%d drifted=%d", missing, drifted)
				}
				return nil
			}
			if err := EmitJSON(map[string]any{"ok": ok, "run_id": runID, "missing": missing, "drifted": drifted, "results": results}); err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("artifact verification failed")
			}
			return nil
		},
	}
	addArtifactRunFlags(c)
	return c
}

func artifactsBackfillCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "backfill",
		Short: "Index existing module evidence into the runtime artifact registry",
		RunE: func(c *cobra.Command, _ []string) error {
			s, runID, err := openArtifactsStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			if runID <= 0 {
				return EmitErr("--run or an active run is required", "")
			}
			root := currentProjectRoot()
			count, err := artifactreg.BackfillRun(s, root, runID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "run_id": runID, "registered": count})
		},
	}
	addArtifactRunFlags(c)
	return c
}

func addArtifactRunFlags(c *cobra.Command) {
	c.Flags().Int64("run", 0, "run id to inspect")
	c.Flags().Bool("latest-run", false, "use the current latest run")
}

func openArtifactsStore(c *cobra.Command) (*db.Store, int64, error) {
	s, err := OpenDB(c)
	if err != nil {
		return nil, 0, EmitErr(err.Error(), "")
	}
	if err := s.Init(); err != nil {
		s.Close()
		return nil, 0, EmitErr(err.Error(), "")
	}
	runID, _ := c.Flags().GetInt64("run")
	latest, _ := c.Flags().GetBool("latest-run")
	if runID == 0 || latest {
		run, err := s.CurrentRun()
		if err != nil {
			s.Close()
			return nil, 0, EmitErr(err.Error(), "")
		}
		if run != nil {
			runID = run.ID
		}
	}
	return s, runID, nil
}

func filterArtifactsByModule(rows []db.Artifact, module string) []db.Artifact {
	module = strings.TrimSpace(module)
	if module == "" {
		return rows
	}
	out := make([]db.Artifact, 0, len(rows))
	for _, row := range rows {
		if row.Module == module {
			out = append(out, row)
		}
	}
	return out
}

func artifactVerificationCounts(results []artifactreg.Verification) (missing int, drifted int) {
	for _, result := range results {
		switch result.Status {
		case artifactreg.VerifyMissing:
			missing++
		case artifactreg.VerifyDrifted:
			drifted++
		}
	}
	return missing, drifted
}
