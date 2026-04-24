package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

// persistFindings writes each finding into the findings table of the active
// run so FTS5 search picks them up. Best-effort: silent on error (findings are
// also written to review.json which is the authoritative snapshot).
func persistFindings(store *db.Store, findings []reviewpipe.Finding, cycle int) {
	if store == nil {
		return
	}
	run, err := store.CurrentRun()
	if err != nil || run == nil {
		return
	}
	if cycle <= 0 {
		if max, err := store.MaxFindingCycle(run.ID); err == nil {
			cycle = max + 1
		} else {
			cycle = 1
		}
	}
	for _, f := range findings {
		raw, _ := json.Marshal(f)
		_, _ = store.InsertFinding(db.Finding{
			RunID:          run.ID,
			Cycle:          cycle,
			Severity:       f.Severity,
			File:           f.File,
			LineStart:      f.LineStart,
			LineEnd:        f.LineEnd,
			Title:          f.Title,
			EvidenceQuote:  f.EvidenceQuote,
			FlaggedBy:      f.FlaggedBy,
			Actionable:     f.Actionable,
			DesignDecision: f.DesignDecision,
			Rationale:      f.ClassifierRationale,
			FullJSON:       string(raw),
		})
	}
}

func ReviewCmd() *cobra.Command {
	c := &cobra.Command{Use: "review", Short: "Adversarial review pipeline (dedupe→filter→classify→render→verdict)"}
	c.AddCommand(reviewRunCmd())
	c.AddCommand(reviewContextPathsCmd())
	return c
}

func reviewRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Run dedupe+filter+classify+render+verdict on a findings dir",
		Long: `Reads findings-<persona>.json from --dir, applies validator-*.json
filters, merges classifier-*.json, renders review.md + review.json, returns verdict.
Use --only STEP to re-run one stage (dedupe|filter|classify|render|verdict).`,
		RunE: func(c *cobra.Command, _ []string) error {
			dir, _ := c.Flags().GetString("dir")
			only, _ := c.Flags().GetString("only")
			nitCap, _ := c.Flags().GetInt("nit-cap")
			validatorModel, _ := c.Flags().GetString("validator-model")
			classifierModel, _ := c.Flags().GetString("classifier-model")
			stackSpec, _ := c.Flags().GetString("stack-specialist")
			cycle, _ := c.Flags().GetInt("cycle")

			if dir == "" {
				dir = filepath.Join(provider.Detect().PlanDir(), "adv-review")
			}
			if _, err := os.Stat(dir); err != nil {
				return EmitErr("findings dir not found: "+dir, "create it or pass --dir")
			}

			// ---- Step 6: dedupe + promote ----
			personaFindings, personas, err := reviewpipe.LoadPersonaFindings(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if len(personaFindings) == 0 {
				// Still run through to produce an APPROVED verdict with empty results.
				fmt.Fprintln(os.Stderr, "[WARN] no persona findings loaded from", dir)
			}
			merged := reviewpipe.DedupeAndPromote(personaFindings)
			merged, suppressed := reviewpipe.ApplyNitCap(merged, nitCap)
			merged = reviewpipe.AssignIDs(merged)
			writeJSON(filepath.Join(dir, "findings-merged.json"), merged)
			if only == "dedupe" {
				return EmitJSON(map[string]any{"ok": true, "step": "dedupe", "count": len(merged), "suppressed_nits": suppressed})
			}

			// ---- Step 7: validator filter ----
			validators, err := reviewpipe.LoadValidators(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			merged, dropped := reviewpipe.ApplyValidators(merged, validators)
			writeJSON(filepath.Join(dir, "findings-validated.json"), merged)
			if only == "filter" {
				return EmitJSON(map[string]any{"ok": true, "step": "filter", "count": len(merged), "dropped": dropped})
			}

			// ---- Step 7.5: classifier merge ----
			classifiers, err := reviewpipe.LoadClassifiers(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			merged = reviewpipe.ApplyClassifiers(merged, classifiers)
			writeJSON(filepath.Join(dir, "findings-classified.json"), merged)
			if only == "classify" {
				return EmitJSON(map[string]any{"ok": true, "step": "classify", "count": len(merged)})
			}

			// ---- Step 8: merge stack-specialist findings (if present) ----
			stack, err := reviewpipe.LoadStackFindings(dir)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if len(stack) > 0 {
				// Stack findings are NEW — also classify them (classifier-stack-*.json).
				stack = reviewpipe.AssignIDs(append([]reviewpipe.Finding{}, stack...))
				stack = reviewpipe.ApplyClassifiers(stack, classifiers)
				merged = append(merged, stack...)
			}
			writeJSON(filepath.Join(dir, "findings-final.json"), merged)

			// ---- Step 9: render (DB-aware: user template overrides embedded) ----
			tally := reviewpipe.BuildTally(merged, dropped, suppressed)
			verdict := reviewpipe.Verdict(tally)
			store, _ := OpenDB(c) // best-effort; Render falls back to embedded when nil
			if store != nil {
				defer store.Close()
			}
			validatorLabel := validatorModel
			if len(validators) == 0 {
				validatorLabel = "SKIPPED"
			}
			classifierLabel := classifierModel
			if len(classifiers) == 0 {
				classifierLabel = "SKIPPED"
			}
			md, rj, err := reviewpipe.Render(store, merged, tally, verdict, personas, validatorLabel, classifierLabel, stackSpec)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := os.WriteFile(filepath.Join(dir, "review.md"), []byte(md), 0o644); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := writeJSON(filepath.Join(dir, "review.json"), rj); err != nil {
				return EmitErr(err.Error(), "")
			}
			// Persist findings into DB so FTS5 search + history work across runs.
			persistFindings(store, merged, cycle)
			if only == "render" {
				return EmitJSON(map[string]any{"ok": true, "step": "render", "path_md": filepath.Join(dir, "review.md"), "path_json": filepath.Join(dir, "review.json")})
			}

			// ---- Step 11: verdict ----
			return EmitJSON(map[string]any{
				"ok":         true,
				"verdict":    verdict,
				"tally":      tally,
				"review_md":  filepath.Join(dir, "review.md"),
				"review_json": filepath.Join(dir, "review.json"),
				"personas":   personas,
			})
		},
	}
	c.Flags().String("dir", "", "findings dir (default .claude/plan/adv-review)")
	c.Flags().String("only", "", "run only one step: dedupe|filter|classify|render|verdict")
	c.Flags().Int("nit-cap", 3, "max P2 findings before roll-up to suppressed count")
	c.Flags().String("validator-model", "sonnet-4.6", "label for review.json metadata")
	c.Flags().String("classifier-model", "sonnet-4.6", "label for review.json metadata")
	c.Flags().String("stack-specialist", "", "e.g. rust-specialist (optional)")
	c.Flags().Int("cycle", 0, "adversarial review cycle (0 = auto-detect as MAX(cycle)+1)")
	return c
}

func reviewContextPathsCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "context-paths",
		Short: "Resolve CLAUDE.md/REVIEW.md/ARCHITECTURE.md/ADRs/spec for classifier input",
		RunE: func(c *cobra.Command, _ []string) error {
			repo, _ := c.Flags().GetString("repo")
			if repo == "" {
				repo = "."
			}
			absRepo, _ := filepath.Abs(repo)
			projectRoot := ResolveProjectRoot(absRepo)

			existsOr := func(p string) string {
				if _, err := os.Stat(p); err == nil {
					return p
				}
				return "NONE"
			}
			adrs := "NONE"
			for _, d := range []string{"docs/adrs", "docs/adr"} {
				cand := filepath.Join(absRepo, d)
				if _, err := os.Stat(cand); err == nil {
					adrs = cand
					break
				}
			}
			p := provider.Detect()
			var taskFiles []string
			matches, _ := filepath.Glob(filepath.Join(absRepo, p.TasksDir(), "**", "task-*.md"))
			taskFiles = append(taskFiles, matches...)

			return EmitJSON(map[string]any{
				"ok":              true,
				"project_root":    projectRoot,
				"repo":            absRepo,
				"agent_config_root": existsOr(filepath.Join(projectRoot, p.AgentConfigFile())),
				"agent_config_repo": existsOr(filepath.Join(absRepo, p.AgentConfigFile())),
				"review_md":       existsOr(filepath.Join(absRepo, "REVIEW.md")),
				"architecture_md": firstExisting(filepath.Join(absRepo, "ARCHITECTURE.md"), filepath.Join(absRepo, "docs", "ARCHITECTURE.md")),
				"adr_paths":       adrs,
				"spec_md":         existsOr(filepath.Join(absRepo, "docs", "spec.md")),
				"task_files":      taskFiles,
			})
		},
	}
	c.Flags().String("repo", ".", "repo dir")
	return c
}

func firstExisting(paths ...string) string {
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "NONE"
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}
