package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
)

func DBCmd() *cobra.Command {
	c := &cobra.Command{Use: "db", Short: "SQLite housekeeping"}
	c.AddCommand(dbInitCmd())
	c.AddCommand(dbMigrateCmd())
	c.AddCommand(dbSearchCmd())
	c.AddCommand(dbExportCmd())
	return c
}

func dbInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create schema + seed embedded prompts (idempotent)",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "check --db path")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "schema apply failed")
			}
			seeded, err := seedEmbeddedPrompts(s)
			if err != nil {
				return EmitErr(err.Error(), "prompt seed failed")
			}
			ver, _ := s.SchemaVersion()
			return EmitJSON(map[string]any{
				"ok":             true,
				"db_path":        s.Path,
				"schema_version": ver,
				"prompts_seeded": seeded,
			})
		},
	}
}

// seedEmbeddedPrompts inserts each embedded prompt as version 1 with
// source='embedded' if it doesn't already exist in the DB.
func seedEmbeddedPrompts(s *db.Store) ([]string, error) {
	names, err := prompts.List()
	if err != nil {
		return nil, err
	}
	var seeded []string
	for _, n := range names {
		existing, err := s.LatestPrompt(n)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			continue
		}
		content, err := prompts.Get(n)
		if err != nil {
			return nil, err
		}
		if _, err := s.InsertPrompt(n, content, "embedded"); err != nil {
			return nil, err
		}
		seeded = append(seeded, n)
	}
	return seeded, nil
}

func dbMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Re-apply schema (idempotent). Also seeds missing prompts.",
		RunE: func(c *cobra.Command, _ []string) error {
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			seeded, _ := seedEmbeddedPrompts(s)
			return EmitJSON(map[string]any{"ok": true, "prompts_seeded": seeded})
		},
	}
}

func dbSearchCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "search <query>",
		Short: "FTS5 search across handoffs/prompts/findings/tasks",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			table, _ := c.Flags().GetString("table")
			limit, _ := c.Flags().GetInt("limit")
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			hits, err := s.Search(table, args[0], limit)
			if err != nil {
				return EmitErr(err.Error(), "valid tables: handoffs, prompts, findings, tasks")
			}
			return EmitJSON(map[string]any{"ok": true, "table": table, "hits": hits, "count": len(hits)})
		},
	}
	c.Flags().String("table", "handoffs", "one of: handoffs, prompts, findings, tasks")
	c.Flags().Int("limit", 20, "max hits to return")
	return c
}

func dbExportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "export",
		Short: "Dump pipeline state as markdown to stdout",
		RunE: func(c *cobra.Command, _ []string) error {
			what, _ := c.Flags().GetString("what")
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no run found", "create one with `codedungeon phase init`")
			}
			var b strings.Builder
			switch what {
			case "runs":
				fmt.Fprintf(&b, "# Run %d\n\nFeature: %s\nBranch: %s\nMode: %s\nProject mode: %s\n",
					run.ID, run.Feature, run.Branch, run.Mode, run.ProjectMode)
			case "phases", "":
				ps, err := s.AllPhases(run.ID)
				if err != nil {
					return EmitErr(err.Error(), "")
				}
				fmt.Fprintf(&b, "# Phases for run %d\n\n", run.ID)
				fmt.Fprintln(&b, "| Phase | Status | Notes |")
				fmt.Fprintln(&b, "|-------|--------|-------|")
				for _, p := range ps {
					fmt.Fprintf(&b, "| %s | %s | %s |\n", p.Phase, p.Status, p.Notes)
				}
			default:
				return EmitErr("unknown --what: "+what, "valid: runs, phases")
			}
			_, _ = os.Stdout.WriteString(b.String())
			return nil
		},
	}
	c.Flags().String("what", "phases", "runs | phases")
	return c
}
