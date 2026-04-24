package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/reviewpipe"
)

func PlanCmd() *cobra.Command {
	c := &cobra.Command{Use: "plan", Short: "PLAN.md + task file operations"}
	c.AddCommand(planMetaCmd())
	c.AddCommand(planAppendFixTasksCmd())
	return c
}

// PlanMeta is the JSON produced by `plan meta`.
type PlanMeta struct {
	OK           bool   `json:"ok"`
	Path         string `json:"path"`
	Feature      string `json:"feature,omitempty"`
	Repo         string `json:"repo,omitempty"`
	Lang         string `json:"lang,omitempty"`
	Pending      int    `json:"pending"`
	Done         int    `json:"done"`
	Blocked      int    `json:"blocked"`
	TotalTasks   int    `json:"total_tasks"`
	NextTaskNum  int    `json:"next_task_num"`
	MaxTaskNum   int    `json:"max_task_num"`
}

func planMetaCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "meta <PLAN.md>",
		Short: "Parse header + count tasks + compute next task num (one JSON)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := args[0]
			b, err := os.ReadFile(path)
			if err != nil {
				return EmitErr(err.Error(), "plan file missing?")
			}
			m := parsePlanMD(string(b))
			m.Path = path
			m.OK = true
			return EmitJSON(m)
		},
	}
}

// Regexes for PLAN.md header + task lines.
var (
	hdrFeature = regexp.MustCompile(`(?mi)^#\s*(?:Plan:|Feature:)\s*(.+?)\s*$`)
	hdrRepo    = regexp.MustCompile(`(?mi)^#\s*Repo:\s*(.+?)\s*$`)
	hdrLang    = regexp.MustCompile(`(?mi)^#\s*Lang:\s*(.+?)\s*$`)
	// Task line: `- [ ] TASK-001 ...` or `- [x] TASK-001 ...` or `- [!] ...`
	taskLine = regexp.MustCompile(`(?mi)^\s*-\s*\[([ xX!])\]\s*TASK-(\d+)\b`)
)

func parsePlanMD(src string) PlanMeta {
	var m PlanMeta
	if sub := hdrFeature.FindStringSubmatch(src); len(sub) > 1 {
		m.Feature = sub[1]
	}
	if sub := hdrRepo.FindStringSubmatch(src); len(sub) > 1 {
		m.Repo = sub[1]
	}
	if sub := hdrLang.FindStringSubmatch(src); len(sub) > 1 {
		m.Lang = sub[1]
	}
	for _, mm := range taskLine.FindAllStringSubmatch(src, -1) {
		var n int
		fmt.Sscanf(mm[2], "%d", &n)
		m.TotalTasks++
		if n > m.MaxTaskNum {
			m.MaxTaskNum = n
		}
		switch mm[1] {
		case "x", "X":
			m.Done++
		case "!":
			m.Blocked++
		default:
			m.Pending++
		}
	}
	m.NextTaskNum = m.MaxTaskNum + 1
	if m.NextTaskNum < 1 {
		m.NextTaskNum = 1
	}
	return m
}

// --- append-fix-tasks ---

func planAppendFixTasksCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "append-fix-tasks",
		Short: "Generate fix task files from review.json actionable findings + append checkboxes to PLAN.md",
		RunE: func(c *cobra.Command, _ []string) error {
			from, _ := c.Flags().GetString("from")
			to, _ := c.Flags().GetString("to")
			cycle, _ := c.Flags().GetInt("cycle")
			if from == "" || to == "" {
				return EmitErr("--from <review.json> and --to <PLAN.md> required", "")
			}
			// Load review.json.
			rb, err := os.ReadFile(from)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			var rv reviewpipe.ReviewJSON
			if err := json.Unmarshal(rb, &rv); err != nil {
				return EmitErr(err.Error(), "expected review.json shape")
			}
			// Load + parse PLAN.md.
			pb, err := os.ReadFile(to)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			meta := parsePlanMD(string(pb))
			// Template.
			tplBody, err := prompts.Get("fix-task-template")
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			tpl, err := template.New("fix").Parse(tplBody)
			if err != nil {
				return EmitErr("parse fix template: "+err.Error(), "")
			}

			// Best-effort DB persistence for FTS5 search.
			store, _ := OpenDB(c)
			if store != nil {
				defer store.Close()
			}
			var runID int64
			if store != nil {
				if run, _ := store.CurrentRun(); run != nil {
					runID = run.ID
				}
			}

			taskDir := filepath.Dir(to)
			var created []string
			var appendLines []string
			num := meta.NextTaskNum
			for _, f := range rv.Findings {
				if !f.Actionable {
					continue
				}
				data := map[string]any{
					"Num":                  fmt.Sprintf("%03d", num),
					"Cycle":                cycle,
					"Title":                f.Title,
					"Severity":             f.Severity,
					"FlaggedBy":            strings.Join(f.FlaggedBy, ", "),
					"EvidenceQuote":        f.EvidenceQuote,
					"SuggestedFix":         f.SuggestedFix,
					"File":                 f.File,
					"LineStart":            f.LineStart,
					"LineEnd":              f.LineEnd,
					"Lang":                 meta.Lang,
					"ClassifierRationale":  f.ClassifierRationale,
					"ClassifierConfidence": f.Confidence,
				}
				var buf bytes.Buffer
				if err := tpl.Execute(&buf, data); err != nil {
					return EmitErr("template exec: "+err.Error(), "")
				}
				fname := fmt.Sprintf("task-%03d-fix-%s.md", num, slugify(f.Title))
				fpath := filepath.Join(taskDir, fname)
				if err := os.WriteFile(fpath, buf.Bytes(), 0o644); err != nil {
					return EmitErr(err.Error(), "")
				}
				created = append(created, fpath)
				appendLines = append(appendLines,
					fmt.Sprintf("- [ ] TASK-%03d Fix (cycle %d): %s", num, cycle, f.Title))
				// Persist to DB (best-effort).
				if store != nil && runID != 0 {
					_ = store.UpsertTask(db.Task{
						RunID:  runID,
						Repo:   meta.Repo,
						TaskID: fmt.Sprintf("TASK-%03d", num),
						Kind:   "fix",
						Status: "pending",
						Title:  fmt.Sprintf("Fix (cycle %d): %s", cycle, f.Title),
						Content: buf.String(),
					})
				}
				num++
			}

			// Append section to PLAN.md if we created any.
			if len(created) > 0 {
				header := fmt.Sprintf("\n## Fix tasks from adversarial review (cycle %d)\n\n", cycle)
				body := strings.Join(appendLines, "\n") + "\n"
				out := string(pb)
				if !strings.HasSuffix(out, "\n") {
					out += "\n"
				}
				out += header + body
				if err := os.WriteFile(to, []byte(out), 0o644); err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			return EmitJSON(map[string]any{
				"ok":          true,
				"created":     created,
				"count":       len(created),
				"next_num":    num,
				"cycle":       cycle,
				"plan_path":   to,
			})
		},
	}
	c.Flags().String("from", "", "path to review.json")
	c.Flags().String("to", "", "path to PLAN.md")
	c.Flags().Int("cycle", 1, "adversarial review cycle number")
	return c
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 40 {
		s = strings.TrimRight(s[:40], "-")
	}
	if s == "" {
		s = "fix"
	}
	return s
}
