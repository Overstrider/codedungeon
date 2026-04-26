package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/provider"
)

func cleanupDirsMap(root string) map[string]string {
	p := provider.Detect()
	return map[string]string{
		"tasks":   projectPath(root, p.TasksDir()),
		"plans":   projectPath(root, p.PlanDir()),
		"reviews": projectPath(root, p.ReviewsDir()),
		"state":   projectPath(root, p.StateDir()),
	}
}

func CleanupCmd() *cobra.Command {
	p := provider.Detect()
	c := &cobra.Command{
		Use:   "cleanup",
		Short: "Remove stale .codedungeon artifacts (tasks, plans, reviews, state)",
		Long: `Deletes CONTENTS (not the directory itself) of ephemeral artifact dirs
(tasks, plans, reviews, state). NEVER deletes commands, agents, skills, bin,
settings, codedungeon.db, or .git.`,
		RunE: func(c *cobra.Command, _ []string) error {
			all, _ := c.Flags().GetBool("all")
			doTasks, _ := c.Flags().GetBool("tasks")
			doPlans, _ := c.Flags().GetBool("plans")
			doReviews, _ := c.Flags().GetBool("reviews")
			doState, _ := c.Flags().GetBool("state")
			feature, _ := c.Flags().GetString("feature")
			dry, _ := c.Flags().GetBool("dry-run")

			root := currentProjectRoot()
			dirs := cleanupDirsMap(root)

			if !all && !doTasks && !doPlans && !doReviews && !doState && feature == "" {
				inv := inventory(root)
				return EmitJSON(map[string]any{"ok": true, "mode": "inventory", "inventory": inv})
			}

			targets := map[string]bool{}
			if all {
				for k := range dirs {
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
				fp := filepath.Join(dirs["tasks"], feature)
				if dry {
					deleted = append(deleted, "DRY: "+fp)
				} else if err := os.RemoveAll(fp); err != nil {
					errors = append(errors, err.Error())
				} else {
					deleted = append(deleted, fp)
				}
			} else {
				for k := range targets {
					dir := dirs[k]
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
				"ok":      len(errors) == 0,
				"mode":    modeLabel(dry),
				"deleted": deleted,
				"errors":  errors,
				"summary": fmt.Sprintf("%d paths processed", len(deleted)),
			})
		},
	}
	c.Flags().Bool("all", false, "delete contents of tasks/ + plans/ + reviews/ + state/")
	c.Flags().Bool("tasks", false, fmt.Sprintf("delete %s/*", p.TasksDir()))
	c.Flags().Bool("plans", false, fmt.Sprintf("delete %s/*", p.PlanDir()))
	c.Flags().Bool("reviews", false, fmt.Sprintf("delete %s/*", p.ReviewsDir()))
	c.Flags().Bool("state", false, fmt.Sprintf("delete %s/*", p.StateDir()))
	c.Flags().String("feature", "", fmt.Sprintf("delete only %s/<NAME>/", p.TasksDir()))
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
func inventory(root string) map[string]any {
	out := map[string]any{}
	for k, dir := range cleanupDirsMap(root) {
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
