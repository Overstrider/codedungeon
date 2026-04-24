package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
)

func ReportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "report",
		Short: "Render the final pipeline report (phase-7)",
	}
	c.AddCommand(reportRenderCmd())
	return c
}

// reportRepo is the view-layer shape for one repo row in the report template.
type reportRepo struct {
	Name              string
	Verdict           string
	PRNumber          string
	IntegrationResult string
	APIResult         string
	E2EResult         string
	Stack             string
	Lang              string
}

func reportRenderCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "render",
		Short: "Aggregate run state from DB + render report to stdout",
		RunE: func(c *cobra.Command, _ []string) error {
			bootstrap, _ := c.Flags().GetBool("bootstrap")
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
				return EmitErr("no active run", "")
			}

			repos := aggregateRepos(s, run)
			phases, _ := s.AllPhases(run.ID)
			execOrder := buildExecutionOrder(repos)

			// Auto-detect bootstrap mode unless flag overrides.
			if run.ProjectMode == "BOOTSTRAP" {
				bootstrap = true
			}
			tplName := "report-template-multi"
			if bootstrap {
				tplName = "report-template-bootstrap"
			}
			body, err := resolveReportTemplate(s, tplName)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			tpl, err := template.New(tplName).Parse(body)
			if err != nil {
				return EmitErr("template parse: "+err.Error(), "")
			}

			data := buildReportData(run, repos, phases, execOrder)

			var out bytes.Buffer
			if err := tpl.Execute(&out, data); err != nil {
				return EmitErr("template exec: "+err.Error(), "")
			}
			fmt.Print(out.String())
			return nil
		},
	}
	c.Flags().Bool("bootstrap", false, "force bootstrap template (auto-detected from project_mode)")
	return c
}

// resolveReportTemplate prefers DB-versioned templates over embedded.
func resolveReportTemplate(s *db.Store, name string) (string, error) {
	if p, err := s.LatestPrompt(name); err == nil && p != nil && p.Content != "" {
		return p.Content, nil
	}
	return prompts.Get(name)
}

// aggregateRepos builds the per-repo view from runs.repo_map + phase-5 handoff
// metadata (PR numbers live in handoff artifacts). Best-effort; missing data
// falls back to placeholders so the template always renders.
func aggregateRepos(s *db.Store, run *db.Run) []reportRepo {
	type repoEntry struct {
		Name  string `json:"name"`
		Stack string `json:"stack"`
		Lang  string `json:"lang"`
	}
	var entries []repoEntry
	if err := json.Unmarshal(run.RepoMap, &entries); err != nil || len(entries) == 0 {
		var singleName string
		if json.Unmarshal(run.RepoMap, &singleName) == nil && singleName != "" {
			entries = []repoEntry{{Name: singleName}}
		}
	}

	// PR numbers from phase-5 handoff artifacts.
	prMap := map[string]string{}
	verdictMap := map[string]string{}
	if h, _ := s.GetHandoff(run.ID, "5"); h != nil {
		for _, a := range h.Artifacts {
			if pr := extractPRNum(a); pr != "" {
				matched := false
				for _, e := range entries {
					if strings.Contains(a, e.Name) {
						prMap[e.Name] = pr
						matched = true
					}
				}
				if !matched && len(entries) == 1 {
					prMap[entries[0].Name] = pr
				}
			}
		}
		if strings.Contains(strings.ToUpper(h.Summary), "APPROVED") {
			for _, e := range entries {
				verdictMap[e.Name] = "APPROVED"
			}
		} else if strings.Contains(strings.ToUpper(h.Summary), "MAX_CYCLES_REACHED") {
			for _, e := range entries {
				verdictMap[e.Name] = "MAX_CYCLES_REACHED"
			}
		}
	}

	// Test results (phase-6 handoff): decisions often look like "backend:api:12/12".
	testResult := map[string]string{}
	if h, _ := s.GetHandoff(run.ID, "6"); h != nil {
		for _, d := range h.Decisions {
			testResult[d] = d
		}
	}

	var repos []reportRepo
	for _, e := range entries {
		repos = append(repos, reportRepo{
			Name:              e.Name,
			Stack:             e.Stack,
			Lang:              e.Lang,
			Verdict:           fallback(verdictMap[e.Name], "PENDING"),
			PRNumber:          prMap[e.Name],
			IntegrationResult: fallback(matchPrefix(testResult, e.Name+":integration"), "n/a"),
			APIResult:         fallback(matchPrefix(testResult, e.Name+":api"), "n/a"),
			E2EResult:         fallback(matchPrefix(testResult, e.Name+":e2e"), "n/a"),
		})
	}
	return repos
}

var prNumRE = regexp.MustCompile(`(?:#|PR\s*#?\s*)(\d+)`)

func extractPRNum(s string) string {
	if m := prNumRE.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

func matchPrefix(m map[string]string, prefix string) string {
	for k, v := range m {
		if strings.HasPrefix(k, prefix) {
			return v
		}
	}
	return ""
}

func buildExecutionOrder(repos []reportRepo) string {
	var names []string
	for _, r := range repos {
		names = append(names, r.Name)
	}
	return strings.Join(names, " → ")
}

func fallback(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func buildReportData(run *db.Run, repos []reportRepo, phases []db.Phase, execOrder string) map[string]any {
	// Conventional plan paths.
	var domainPlans, qaPlans []string
	for _, r := range repos {
		domainPlans = append(domainPlans, fmt.Sprintf(".claude/plan/%splan.md", r.Name))
		qaPlans = append(qaPlans, fmt.Sprintf(".claude/plan/%sqaplan.md", r.Name))
	}
	testBugs := 0
	for _, p := range phases {
		if p.Phase == "6" {
			testBugs = len(p.Artifacts)
		}
	}
	// Bootstrap mode: first repo (usually ".") drives the single-line view.
	bs := reportRepo{Name: "."}
	if len(repos) > 0 {
		bs = repos[0]
	}
	return map[string]any{
		"Feature":         run.Feature,
		"Mode":            run.Mode,
		"ProjectMode":     run.ProjectMode,
		"Branch":          run.Branch,
		"DomainPlans":     strings.Join(domainPlans, ", "),
		"QAPlans":         strings.Join(qaPlans, ", "),
		"DomainPlanCount": fmt.Sprintf("%d", len(domainPlans)),
		"ExecutionOrder":  execOrder,
		"Repos":           repos,
		"TestBugsFound":   testBugs,
		// Bootstrap-specific fields.
		"Stack":      bs.Stack,
		"Lang":       bs.Lang,
		"PRNumber":   bs.PRNumber,
		"Status":     bs.Verdict,
		"TestResult": bs.IntegrationResult,
		"DevTasks":   0,
		"TestTasks":  0,
	}
}
