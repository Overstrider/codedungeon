package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/codereview"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/projectcontext"
	"github.com/loldinis/codedungeon/internal/provider"
)

func ObserveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "observe",
		Short: "Inspect CodeDungeon run, phase, and agent telemetry",
	}
	c.AddCommand(observeAgentsCmd())
	c.AddCommand(observeReportCmd())
	return c
}

func observeAgentsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "agents",
		Short: "Show agent telemetry for the current run",
		RunE: func(c *cobra.Command, _ []string) error {
			s, run, err := openObserveStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			agents, err := s.AgentRuns(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			events, err := s.AgentEvents(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			runEvents, err := s.RunEvents(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":         true,
				"run":        run,
				"summary":    agentTelemetrySummary(agents),
				"agents":     agents,
				"events":     events,
				"run_events": runEvents,
			})
		},
	}
}

func observeReportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "report",
		Short: "Render a Markdown observability report for the current run",
		RunE: func(_ *cobra.Command, _ []string) error {
			root := ResolveProjectRoot(".")
			report, err := RenderObserveReport(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			_, _ = os.Stdout.WriteString(report)
			return nil
		},
	}
}

func openObserveStore(c *cobra.Command) (*db.Store, *db.Run, error) {
	s, err := OpenDB(c)
	if err != nil {
		return nil, nil, EmitErr(err.Error(), "")
	}
	if err := s.Init(); err != nil {
		s.Close()
		return nil, nil, EmitErr(err.Error(), "")
	}
	run, err := s.CurrentRun()
	if err != nil {
		s.Close()
		return nil, nil, EmitErr(err.Error(), "")
	}
	if run == nil {
		s.Close()
		return nil, nil, EmitErr("no active run", "")
	}
	return s, run, nil
}

func RenderObserveReport(root string) (string, error) {
	root = ResolveProjectRoot(root)
	s, err := db.Open(filepath.Join(root, provider.Detect().DBPath()))
	if err != nil {
		return "", err
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		return "", err
	}
	run, err := s.CurrentRun()
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", fmt.Errorf("no active run")
	}
	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		return "", err
	}
	events, err := s.AgentEvents(run.ID)
	if err != nil {
		return "", err
	}
	runEvents, err := s.RunEvents(run.ID)
	if err != nil {
		return "", err
	}
	phases, _ := s.AllPhases(run.ID)
	session, _ := s.LatestRunSession(run.ID)
	review, _ := s.LatestReviewEvidence(run.ID)
	prPost, _ := s.LatestPRReviewPost(run.ID)
	verification, _ := s.VerificationRecords(run.ID, "6")
	planning, _ := s.LatestPlanningSession()
	executionSessions, _ := s.ExecutionSessions(run.ID)
	executionAttempts := map[string][]db.ExecutionAttempt{}
	for _, execSession := range executionSessions {
		attempts, _ := s.ExecutionAttempts(execSession.ID)
		executionAttempts[execSession.ID] = attempts
	}
	var planningAgents []db.PlanningAgent
	var planningEvaluations []db.PlanningEvaluation
	var planningGraphs []db.PlanningTaskGraph
	if planning != nil && (planning.RunID == 0 || planning.RunID == run.ID) {
		planningAgents, _ = s.PlanningAgents(planning.ID)
		planningEvaluations, _ = s.PlanningEvaluations(planning.ID)
		planningGraphs, _ = s.PlanningTaskGraphs(planning.ID)
	} else {
		planning = nil
	}
	contextStatus, contextErr := projectcontext.Status(root, projectcontext.NewSQLiteStore(s))
	registeredArtifacts, _ := s.ArtifactsByRun(run.ID)
	artifactChecks, _ := artifactreg.NewRegistry(s, root).VerifyRun(run.ID)

	var b strings.Builder
	summary := agentTelemetrySummary(agents)
	fmt.Fprintln(&b, "# CodeDungeon Observability Report")
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "- Run: %d\n", run.ID)
	fmt.Fprintf(&b, "- Mode: %s\n", fallback(run.Mode, "unknown"))
	fmt.Fprintf(&b, "- Branch: %s\n", fallback(run.Branch, "unknown"))
	if session != nil {
		fmt.Fprintf(&b, "- Session: %s (%s, provider %s)\n", session.ID, session.Status, session.Provider)
	} else {
		fmt.Fprintln(&b, "- Session: none recorded")
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Agent Timeline")
	if len(agents) == 0 {
		fmt.Fprintln(&b, "No agent telemetry recorded.")
	} else {
		fmt.Fprintln(&b, "| ID | Phase | Role | Agent Type | Status | Model | Started | Finished | Artifact |")
		fmt.Fprintln(&b, "|----|-------|------|------------|--------|-------|---------|----------|----------|")
		for _, a := range agents {
			fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s | %s |\n",
				a.ID, fallback(a.Phase, "-"), fallback(a.Role, "-"), fallback(a.AgentType, "-"),
				a.Status, fallback(a.Model, "-"), formatUnix(a.StartedAt), formatUnix(a.FinishedAt),
				fallback(a.ArtifactPath, fallback(a.TaskPath, "-")))
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Telemetry warnings")
	warnings := telemetryWarnings(summary, agents)
	for _, warning := range warnings {
		fmt.Fprintf(&b, "- %s\n", warning)
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Phase Status")
	if len(phases) == 0 {
		fmt.Fprintln(&b, "No phase rows recorded.")
	} else {
		for _, p := range phases {
			fmt.Fprintf(&b, "- Phase %s: %s%s\n", p.Phase, p.Status, noteSuffix(p.Notes))
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Task Planning")
	if planning == nil {
		fmt.Fprintln(&b, "- Status: none recorded")
	} else {
		fmt.Fprintf(&b, "- Session: %s\n", planning.ID)
		fmt.Fprintf(&b, "- Status: %s\n", planning.Status)
		fmt.Fprintf(&b, "- Human gate policy: %s\n", planning.HumanGatePolicy)
		fmt.Fprintf(&b, "- Output: %s\n", planning.OutputDir)
		fmt.Fprintf(&b, "- Planning agents: %d\n", len(planningAgents))
		if len(planningEvaluations) > 0 {
			last := planningEvaluations[len(planningEvaluations)-1]
			fmt.Fprintf(&b, "- Evaluator: %s, needs_user_input=%t, score=%.2f\n", last.Verdict, last.NeedsUserInput, last.Score)
			for _, question := range last.Questions {
				fmt.Fprintf(&b, "- User question: %s\n", question)
			}
		}
		if len(planningGraphs) > 0 {
			last := planningGraphs[len(planningGraphs)-1]
			fmt.Fprintf(&b, "- Task graph: v%d %s\n", last.Version, last.Status)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Task Execution")
	if len(executionSessions) == 0 {
		fmt.Fprintln(&b, "- Status: none recorded")
	} else {
		for _, execSession := range executionSessions {
			fmt.Fprintf(&b, "- Session: %s task %s status %s attempts %d output %s\n",
				execSession.ID, execSession.TaskID, execSession.Status, execSession.Attempt, execSession.OutputDir)
			for _, attempt := range executionAttempts[execSession.ID] {
				files := "-"
				if len(attempt.ChangedFiles) > 0 {
					files = strings.Join(attempt.ChangedFiles, ", ")
				}
				fmt.Fprintf(&b, "  - Attempt %d: worker=%s verification=%s head=%s..%s files=%s\n",
					attempt.Attempt, fallback(attempt.WorkerStatus, "-"), fallback(attempt.VerificationStatus, "-"),
					fallback(attempt.HeadBefore, "-"), fallback(attempt.HeadAfter, "-"), files)
			}
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Project Context")
	if contextErr != nil {
		fmt.Fprintf(&b, "- Status: error - %s\n", contextErr)
	} else {
		fmt.Fprintf(&b, "- Status: %s\n", contextStatus.Status)
		fmt.Fprintf(&b, "- Active version: %d\n", contextStatus.ActiveVersion)
		fmt.Fprintf(&b, "- Pending proposals: %d\n", contextStatus.PendingProposals)
		if contextStatus.StaleReason != "" {
			fmt.Fprintf(&b, "- Stale reason: %s\n", contextStatus.StaleReason)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Artifact Registry")
	if len(registeredArtifacts) == 0 {
		fmt.Fprintln(&b, "- Status: none recorded")
	} else {
		missing, drifted := artifactVerificationCounts(artifactChecks)
		integrity := "PASS"
		if missing > 0 || drifted > 0 {
			integrity = "WARN"
		}
		fmt.Fprintf(&b, "- Artifacts: %d\n", len(registeredArtifacts))
		fmt.Fprintf(&b, "- Integrity: %s (missing=%d, drifted=%d)\n", integrity, missing, drifted)
		for _, item := range artifactModuleCounts(registeredArtifacts) {
			fmt.Fprintf(&b, "- %s: %d\n", item.module, item.count)
		}
	}
	fmt.Fprintln(&b)

	fmt.Fprintln(&b, "## Evidence")
	if review != nil {
		fmt.Fprintf(&b, "- Review: %s, PR #%s, personas %s\n", review.Verdict, review.PRNumber, strings.Join(review.PersonasRun, ", "))
		if _, err := codereview.ValidateResultDir(review.ReviewDir); err != nil {
			fmt.Fprintf(&b, "- Review Integrity: FAIL - %s\n", err)
		} else {
			fmt.Fprintln(&b, "- Review Integrity: PASS")
		}
	} else {
		fmt.Fprintln(&b, "- Review: none recorded")
		fmt.Fprintln(&b, "- Review Integrity: FAIL - none recorded")
	}
	if prPost != nil {
		fmt.Fprintf(&b, "- PR review post: #%s %s\n", prPost.PRNumber, prPost.CommentURL)
	} else {
		fmt.Fprintln(&b, "- PR review post: none recorded")
	}
	if len(verification) == 0 {
		fmt.Fprintln(&b, "- Verification: no phase 6 records")
	} else {
		for _, record := range latestVerificationRecords(verification) {
			fmt.Fprintf(&b, "- Verification: %s - %s\n", record.Status, record.Command)
		}
	}
	if len(events) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Agent Events")
		for _, event := range events {
			fmt.Fprintf(&b, "- %s agent #%d phase %s: %s%s\n",
				formatUnix(event.CreatedAt), event.AgentRunID, fallback(event.Phase, "-"),
				event.Event, noteSuffix(event.Detail))
		}
	}
	if len(runEvents) > 0 {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "## Run Events")
		for _, event := range runEvents {
			fmt.Fprintf(&b, "- %s %s%s\n", formatUnix(event.CreatedAt), event.Event, noteSuffix(event.Detail))
		}
	}
	return b.String(), nil
}

type artifactModuleCount struct {
	module string
	count  int
}

func artifactModuleCounts(rows []db.Artifact) []artifactModuleCount {
	counts := map[string]int{}
	for _, row := range rows {
		counts[row.Module]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]artifactModuleCount, 0, len(keys))
	for _, key := range keys {
		out = append(out, artifactModuleCount{module: key, count: counts[key]})
	}
	return out
}

func agentTelemetrySummary(agents []db.AgentRun) map[string]any {
	statuses := map[string]int{}
	phases := map[string]int{}
	open := 0
	for _, a := range agents {
		statuses[a.Status]++
		if a.Status == "RUNNING" {
			open++
		}
		if a.Phase != "" {
			phases[a.Phase]++
		}
	}
	return map[string]any{
		"total":         len(agents),
		"open":          open,
		"status_counts": statuses,
		"phase_counts":  phases,
	}
}

func telemetryWarnings(summary map[string]any, agents []db.AgentRun) []string {
	open, _ := summary["open"].(int)
	warnings := []string{fmt.Sprintf("open agent runs: %d", open)}
	if len(agents) == 0 {
		warnings = append(warnings, "no agent telemetry recorded")
	}
	var failed []string
	for _, a := range agents {
		if a.Status == "FAILED" || a.Status == "ABORTED" {
			failed = append(failed, fmt.Sprintf("#%d %s", a.ID, a.Status))
		}
	}
	sort.Strings(failed)
	if len(failed) > 0 {
		warnings = append(warnings, "terminal non-success statuses: "+strings.Join(failed, ", "))
	}
	return warnings
}

func formatUnix(ts int64) string {
	if ts == 0 {
		return "-"
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func noteSuffix(note string) string {
	note = strings.TrimSpace(note)
	if note == "" {
		return ""
	}
	return " - " + strings.ReplaceAll(note, "\n", " ")
}
