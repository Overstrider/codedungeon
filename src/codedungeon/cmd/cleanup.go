package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

// Dirs subject to cleanup. NEVER touched: commands/ agents/ skills/ settings.*
// Also NEVER: bin/ (has binary), .git/.
var cleanupDirs = map[string]string{
	"tasks":   ".claude/tasks",
	"plans":   ".claude/plan",
	"reviews": ".claude/codereview",
	"state":   ".claude/state", // handoff md cache
}

func CleanupCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove stale .claude/ artifacts (tasks, plans, reviews, state)",
		Long: `Deletes CONTENTS (not the directory itself) of:
  .claude/tasks/    — feature task dirs (PLAN.md, MASTER.md, TASK-NNN-*.md)
  .claude/plan/     — implementation + adversarial plans (arcplan.md, pipeline-state.md, adv-review/)
  .claude/codereview/ — legacy review output
  .claude/state/ — phase-N-output.md cache (DB is source of truth)

NEVER deletes: .claude/commands/, .claude/agents/, .claude/skills/, .claude/bin/,
.claude/settings*.json, .claude/codedungeon.db, or .git/.`,
		RunE: func(c *cobra.Command, _ []string) error {
			all, _ := c.Flags().GetBool("all")
			doTasks, _ := c.Flags().GetBool("tasks")
			doPlans, _ := c.Flags().GetBool("plans")
			doReviews, _ := c.Flags().GetBool("reviews")
			doState, _ := c.Flags().GetBool("state")
			feature, _ := c.Flags().GetString("feature")
			dry, _ := c.Flags().GetBool("dry-run")

			// Default inventory-only when no flags.
			if !all && !doTasks && !doPlans && !doReviews && !doState && feature == "" {
				inv := inventory()
				return EmitJSON(map[string]any{"ok": true, "mode": "inventory", "inventory": inv})
			}

			targets := map[string]bool{}
			if all {
				for k := range cleanupDirs {
					targets[k] = true
				}
			}
			if doTasks {
				targets["tasks"] = true
			}
			if doPlans {
				targets["plans"] = true
			}
			if doReviews {
				targets["reviews"] = true
			}
			if doState {
				targets["state"] = true
			}

			var deleted []string
			var errors []string
			if feature != "" {
				// Selective feature delete under .claude/tasks/{feature}/
				p := filepath.Join(cleanupDirs["tasks"], feature)
				if dry {
					deleted = append(deleted, "DRY: "+p)
				} else if err := os.RemoveAll(p); err != nil {
					errors = append(errors, err.Error())
				} else {
					deleted = append(deleted, p)
				}
			} else {
				for k := range targets {
					dir := cleanupDirs[k]
					entries, err := os.ReadDir(dir)
					if err != nil {
						continue
					}
					for _, e := range entries {
						full := filepath.Join(dir, e.Name())
						if dry {
							deleted = append(deleted, "DRY: "+full)
							continue
						}
						if err := os.RemoveAll(full); err != nil {
							errors = append(errors, err.Error())
							continue
						}
						deleted = append(deleted, full)
					}
				}
			}
			return EmitJSON(map[string]any{
				"ok":       len(errors) == 0,
				"mode":     modeLabel(dry),
				"deleted":  deleted,
				"errors":   errors,
				"summary":  fmt.Sprintf("%d paths processed", len(deleted)),
			})
		},
	}
	c.Flags().Bool("all", false, "delete contents of tasks/ + plans/ + reviews/ + state/")
	c.Flags().Bool("tasks", false, "delete .claude/tasks/*")
	c.Flags().Bool("plans", false, "delete .claude/plan/*")
	c.Flags().Bool("reviews", false, "delete .claude/codereview/*")
	c.Flags().Bool("state", false, "delete .claude/state/*")
	c.Flags().String("feature", "", "delete only .claude/tasks/<NAME>/")
	c.Flags().Bool("dry-run", false, "don't delete; just list what would be deleted")
	return c
}

func modeLabel(dry bool) string {
	if dry {
		return "dry-run"
	}
	return "delete"
}

// inventory returns a map of dirs → file counts + total bytes.
func inventory() map[string]any {
	out := map[string]any{}
	for k, dir := range cleanupDirs {
		n, size := walkStats(dir)
		out[k] = map[string]any{"dir": dir, "files": n, "bytes": size}
	}
	return out
}

func walkStats(dir string) (int, int64) {
	var files int
	var size int64
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		// Skip hidden files like .gitignore inside those dirs.
		if strings.HasPrefix(filepath.Base(p), ".") {
			return nil
		}
		files++
		size += info.Size()
		return nil
	})
	return files, size
}
