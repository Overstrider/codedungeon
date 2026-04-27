package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/loldinis/codedungeon/internal/provider"
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
	PRURL             string
	Branch            string
	ReviewCycles      string
	ReviewMode        string
	Summary           string
	AdvReviewCount    string
	RemainingFindings string
	TasksCompleted    string
	ChangedFiles      string
	Verification      string
	NextAction        string
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

			if err := validateReportGates(s, run, false, true); err != nil {
				return EmitErr("report-gate: "+err.Error(), "")
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
			report := out.String()
			root := ResolveProjectRoot(".")
			if err := writeReportMemoryFiles(root, run, repos, report); err != nil {
				return EmitErr(err.Error(), "")
			}
			if err := recordReportEvidence(s, root, run, report); err != nil {
				return EmitErr(err.Error(), "")
			}
			fmt.Print(report)
			return nil
		},
	}
	c.Flags().Bool("bootstrap", false, "force bootstrap template (auto-detected from project_mode)")
	return c
}

func validateReportGates(s *db.Store, run *db.Run, requireReportEvidence, requireGitVerify bool) error {
	phases, err := s.AllPhases(run.ID)
	if err != nil {
		return err
	}
	for _, p := range phases {
		if p.Phase == "7" {
			break
		}
		if p.Status != "DONE" && p.Status != "SKIPPED" {
			return fmt.Errorf("phase %s is %s", p.Phase, p.Status)
		}
	}
	reviewEvidence, err := s.LatestReviewEvidence(run.ID)
	if err != nil {
		return err
	}
	if err := validateReviewEvidence(reviewEvidence); err != nil {
		return err
	}
	records, err := s.VerificationRecords(run.ID, "6")
	if err != nil {
		return err
	}
	if err := validateVerificationRecords(records); err != nil {
		return err
	}
	if requireGitVerify {
		status, err := gitVerifyStatus(".", run.Branch)
		if err != nil {
			return err
		}
		if ok, _ := status["ok"].(bool); !ok {
			return fmt.Errorf("codedungeon git verify did not pass")
		}
	}
	if requireReportEvidence {
		evidence, err := s.LatestReportEvidence(run.ID)
		if err != nil {
			return err
		}
		if evidence == nil {
			return fmt.Errorf("report render evidence is required")
		}
		body, err := os.ReadFile(evidence.ReportPath)
		if err != nil {
			return fmt.Errorf("report evidence not readable: %w", err)
		}
		sum := sha256.Sum256(body)
		if hex.EncodeToString(sum[:]) != evidence.SHA256 {
			return fmt.Errorf("report evidence sha mismatch")
		}
	}
	return nil
}

func validateVerificationRecords(records []db.VerificationRecord) error {
	if len(records) == 0 {
		return fmt.Errorf("verification ledger is required")
	}
	for _, record := range records {
		if record.Status != "PASS" {
			return fmt.Errorf("verification command failed: %s", record.Command)
		}
		info, err := os.Stat(record.LogPath)
		if err != nil {
			return fmt.Errorf("verification log not found: %s", record.LogPath)
		}
		if info.Size() == 0 {
			return fmt.Errorf("verification log is empty: %s", record.LogPath)
		}
	}
	return nil
}

func recordReportEvidence(s *db.Store, root string, run *db.Run, report string) error {
	reportPath := filepath.Join(root, codedungeonDir, "reports", fmt.Sprintf("run-%d.md", run.ID))
	sum := sha256.Sum256([]byte(report))
	_, err := s.InsertReportEvidence(db.ReportEvidence{
		RunID:      run.ID,
		ReportPath: reportPath,
		SHA256:     hex.EncodeToString(sum[:]),
	})
	return err
}

func writeReportMemoryFiles(root string, run *db.Run, repos []reportRepo, report string) error {
	runDir := filepath.Join(root, codedungeonDir, "memory", "runs")
	prDir := filepath.Join(root, codedungeonDir, "memory", "prs")
	reportDir := filepath.Join(root, codedungeonDir, "reports")
	for _, dir := range []string{runDir, prDir, reportDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", dir, err)
		}
	}
	header := fmt.Sprintf("# CodeDungeon Run %d\n\nFeature: %s\nBranch: %s\n\n", run.ID, run.Feature, run.Branch)
	runBody := header + report
	if err := os.WriteFile(filepath.Join(runDir, fmt.Sprintf("run-%d.md", run.ID)), []byte(runBody), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(reportDir, fmt.Sprintf("run-%d.md", run.ID)), []byte(report), 0o644); err != nil {
		return err
	}
	for _, repo := range repos {
		if repo.PRNumber == "" {
			continue
		}
		body := fmt.Sprintf("# PR #%s\n\nRun: %d\nFeature: %s\nBranch: %s\nRepo: %s\nVerdict: %s\n\n%s",
			repo.PRNumber, run.ID, run.Feature, run.Branch, repo.Name, repo.Verdict, report)
		if err := os.WriteFile(filepath.Join(prDir, "pr-"+repo.PRNumber+".md"), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
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
	prURLMap := map[string]string{}
	verdictMap := map[string]string{}
	if h, _ := s.GetHandoff(run.ID, "5"); h != nil {
		for _, a := range h.Artifacts {
			if pr := extractPRNum(a); pr != "" {
				matched := false
				for _, e := range entries {
					if strings.Contains(a, e.Name) {
						prMap[e.Name] = pr
						if prURL := extractPRURL(a); prURL != "" {
							prURLMap[e.Name] = prURL
						}
						matched = true
					}
				}
				if !matched && len(entries) == 1 {
					prMap[entries[0].Name] = pr
					if prURL := extractPRURL(a); prURL != "" {
						prURLMap[entries[0].Name] = prURL
					}
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
	verification := summarizeVerificationRecords(s, run.ID)

	var repos []reportRepo
	for _, e := range entries {
		repos = append(repos, reportRepo{
			Name:              e.Name,
			Stack:             e.Stack,
			Lang:              e.Lang,
			Verdict:           fallback(verdictMap[e.Name], "PENDING"),
			PRNumber:          prMap[e.Name],
			PRURL:             fallback(prURLMap[e.Name], "url unavailable"),
			Branch:            fallback(run.Branch, "unknown"),
			ReviewCycles:      "unknown",
			ReviewMode:        "not_run",
			Summary:           fallback(run.Feature, "n/a"),
			AdvReviewCount:    "unknown",
			RemainingFindings: "unknown",
			TasksCompleted:    "unknown",
			ChangedFiles:      "unknown",
			Verification:      verification,
			NextAction:        "inspect PR review state",
			IntegrationResult: fallback(matchPrefix(testResult, e.Name+":integration"), "n/a"),
			APIResult:         fallback(matchPrefix(testResult, e.Name+":api"), "n/a"),
			E2EResult:         fallback(matchPrefix(testResult, e.Name+":e2e"), "n/a"),
		})
	}
	return repos
}

func summarizeVerificationRecords(s *db.Store, runID int64) string {
	records, err := s.VerificationRecords(runID, "6")
	if err != nil || len(records) == 0 {
		return "missing"
	}
	var parts []string
	for _, r := range records {
		parts = append(parts, fmt.Sprintf("%s: %s", r.Command, r.Status))
	}
	return strings.Join(parts, "; ")
}

var prNumRE = regexp.MustCompile(`(?:#|PR\s*#?\s*)(\d+)`)
var prURLRE = regexp.MustCompile(`https?://[^\s)]+/pull/\d+`)

func extractPRNum(s string) string {
	if m := prNumRE.FindStringSubmatch(s); len(m) > 1 {
		return m[1]
	}
	return ""
}

func extractPRURL(s string) string {
	return strings.TrimRight(prURLRE.FindString(s), ".,;:")
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
		planDir := provider.Detect().PlanDir()
		domainPlans = append(domainPlans, fmt.Sprintf("%s/%splan.md", planDir, r.Name))
		qaPlans = append(qaPlans, fmt.Sprintf("%s/%sqaplan.md", planDir, r.Name))
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
		"Stack":             bs.Stack,
		"Lang":              bs.Lang,
		"PRNumber":          bs.PRNumber,
		"PRURL":             bs.PRURL,
		"ReviewCycles":      bs.ReviewCycles,
		"ReviewMode":        bs.ReviewMode,
		"AdvReviewCount":    bs.AdvReviewCount,
		"RemainingFindings": bs.RemainingFindings,
		"ChangedFiles":      bs.ChangedFiles,
		"NextAction":        bs.NextAction,
		"Status":            bs.Verdict,
		"TestResult":        bs.IntegrationResult,
		"DevTasks":          0,
		"TestTasks":         0,
	}
}
