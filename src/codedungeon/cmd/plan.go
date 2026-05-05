package cmd

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/prompts"
	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/reviewpipe"
	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func PlanCmd() *cobra.Command {
	c := &cobra.Command{Use: "plan", Short: "PLAN.md + task file operations"}
	c.AddCommand(planRunCmd())
	c.AddCommand(planStatusCmd())
	c.AddCommand(planResumeCmd())
	c.AddCommand(planValidateCmd())
	c.AddCommand(planPromoteCmd())
	c.AddCommand(planMetaCmd())
	c.AddCommand(planAppendFixTasksCmd())
	return c
}

func planRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Run the standalone task-planning swarm",
		RunE: func(c *cobra.Command, _ []string) error {
			req, runner, err := planningRequestFromFlags(c, "")
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			s, run, err := openPlanningStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			if run != nil {
				req.RunID = run.ID
			}
			if req.OutputDir == "" {
				req.OutputDir = defaultPlanningOutputDir(req.Prompt, req.SessionID)
			}
			if req.SessionID == "" {
				req.SessionID = filepath.Base(req.OutputDir)
			}
			if req.SessionID == "." || req.SessionID == string(filepath.Separator) {
				req.SessionID = "plan-" + time.Now().Format("20060102150405")
			}
			_ = persistPlanningSessionStart(s, req)
			if req.RunID != 0 {
				_, _ = s.InsertRunEvent(db.RunEvent{RunID: req.RunID, Event: "planning_started", Detail: req.SessionID})
			}
			result, execErr := taskplanning.Execute(context.Background(), req, runner)
			legacyPhase4, _ := c.Flags().GetBool("legacy-phase4")
			if execErr == nil && legacyPhase4 && result.TaskGraph != nil {
				feature := req.Prompt
				if run != nil && run.Feature != "" {
					feature = run.Feature
				}
				legacyArtifacts, legacyErr := mirrorPlanningLegacyArtifacts(currentProjectRoot(), feature, result)
				if legacyErr != nil {
					execErr = legacyErr
				} else {
					result.Artifacts = append(result.Artifacts, legacyArtifacts...)
					if result.Metadata == nil {
						result.Metadata = map[string]any{}
					}
					result.Metadata["legacy_phase4_artifacts"] = legacyArtifacts
				}
			}
			promote, _ := c.Flags().GetBool("promote")
			promoteRepo, _ := c.Flags().GetString("promote-repo")
			if execErr == nil && promote && result.TaskGraph != nil {
				feature := req.Prompt
				if run != nil && run.Feature != "" {
					feature = run.Feature
				}
				promotion, promoteErr := promotePlanningArtifacts(currentProjectRoot(), result.OutputDir, promoteRepo, feature)
				if promoteErr != nil {
					execErr = promoteErr
				} else {
					result.Artifacts = append(result.Artifacts, promotion.Artifacts...)
					result.PromotionMode = promotion.Mode
					result.PromotedRepos = promotion.Repos
					result.PromotedArtifacts = promotion.Artifacts
					if result.Metadata == nil {
						result.Metadata = map[string]any{}
					}
					result.Metadata["promotion_mode"] = promotion.Mode
					result.Metadata["promoted_repos"] = promotion.Repos
					result.Metadata["promoted_artifacts"] = promotion.Artifacts
				}
			}
			if persistErr := persistPlanningResult(s, req, result, execErr); persistErr != nil && execErr == nil {
				execErr = persistErr
			}
			if execErr != nil {
				if req.RunID != 0 {
					_, _ = s.InsertRunEvent(db.RunEvent{RunID: req.RunID, Event: "planning_failed", Detail: execErr.Error()})
				}
				return EmitCustodyErr(execErr.Error(), planningRecoveryCommands(req, promote, promoteRepo))
			}
			if req.RunID != 0 {
				_, _ = s.InsertRunEvent(db.RunEvent{RunID: req.RunID, Event: "planning_" + strings.ToLower(planningStatusOrCompleted(result.Status)), Detail: req.SessionID})
			}
			return EmitJSON(result)
		},
	}
	addPlanningRunFlags(c)
	return c
}

func planStatusCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "status",
		Short: "Show task-planning session status",
		RunE: func(c *cobra.Command, _ []string) error {
			sessionID, _ := c.Flags().GetString("session")
			s, _, err := openPlanningStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			session, err := planningSessionByFlag(s, sessionID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			agents, _ := s.PlanningAgents(session.ID)
			evals, _ := s.PlanningEvaluations(session.ID)
			graphs, _ := s.PlanningTaskGraphs(session.ID)
			return EmitJSON(map[string]any{
				"ok":          true,
				"session":     session,
				"agents":      agents,
				"evaluations": evals,
				"task_graphs": graphs,
			})
		},
	}
	c.Flags().String("session", "", "planning session id (defaults to latest)")
	return c
}

func planResumeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "resume",
		Short: "Resume a failed or user-blocked task-planning session",
		RunE: func(c *cobra.Command, _ []string) error {
			sessionID, _ := c.Flags().GetString("session")
			s, _, err := openPlanningStore(c)
			if err != nil {
				return err
			}
			defer s.Close()
			session, err := planningSessionByFlag(s, sessionID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if session.Status == taskplanning.StatusCompleted {
				return EmitErr("planning session already completed", session.ID)
			}
			wasFailed := session.Status == taskplanning.StatusFailed
			req, err := readPlanningRequest(filepath.Join(session.OutputDir, "planning-request.json"))
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			req.SessionID = session.ID
			req.OutputDir = session.OutputDir
			req.RunID = session.RunID
			_, runner, err := planningRequestFromFlags(c, req.ProjectContext)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			_ = s.UpdatePlanningSessionStatus(session.ID, taskplanning.StatusRunning, "")
			result, execErr := taskplanning.Execute(context.Background(), req, runner)
			if persistErr := persistPlanningResult(s, req, result, execErr); persistErr != nil && execErr == nil {
				execErr = persistErr
			}
			if execErr != nil {
				if req.RunID != 0 {
					_, _ = s.InsertRunEvent(db.RunEvent{RunID: req.RunID, Event: "planning_failed", Detail: execErr.Error()})
				}
				return EmitErr(execErr.Error(), "")
			}
			if req.RunID != 0 {
				if wasFailed {
					_, _ = s.InsertRunEvent(db.RunEvent{RunID: req.RunID, Event: "planning_recovered", Detail: session.ID})
				}
				_, _ = s.InsertRunEvent(db.RunEvent{RunID: req.RunID, Event: "planning_" + strings.ToLower(planningStatusOrCompleted(result.Status)), Detail: session.ID})
			}
			return EmitJSON(result)
		},
	}
	c.Flags().String("session", "", "planning session id (defaults to latest)")
	addPlanningRunnerFlags(c)
	return c
}

func planValidateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "validate",
		Short: "Validate a task-planning task graph JSON",
		RunE: func(c *cobra.Command, _ []string) error {
			path, _ := c.Flags().GetString("task-graph")
			if strings.TrimSpace(path) == "" {
				return EmitErr("--task-graph is required", "")
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			var graph taskplanning.TaskGraph
			if err := json.Unmarshal(body, &graph); err != nil {
				return EmitErr(err.Error(), "invalid task graph JSON")
			}
			if err := taskplanning.ValidateTaskGraph(graph); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "task_graph": path, "tasks": len(graph.Tasks)})
		},
	}
	c.Flags().String("task-graph", "", "path to task-graph.json")
	return c
}

func planPromoteCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "promote",
		Short: "Promote task-planning artifacts to canonical .codedungeon paths",
		RunE: func(c *cobra.Command, _ []string) error {
			from, _ := c.Flags().GetString("from")
			repo, _ := c.Flags().GetString("repo")
			feature, _ := c.Flags().GetString("feature")
			if strings.TrimSpace(from) == "" {
				return EmitErr("--from is required", "")
			}
			promotion, err := promotePlanningArtifacts(currentProjectRoot(), from, repo, feature)
			if err != nil {
				return EmitCustodyErr(err.Error(), promotionRecoveryCommands(from, repo, feature))
			}
			return EmitJSON(map[string]any{
				"ok":                 true,
				"artifacts":          promotion.Artifacts,
				"promotion_mode":     promotion.Mode,
				"promoted_repos":     promotion.Repos,
				"promoted_artifacts": promotion.Artifacts,
			})
		},
	}
	c.Flags().String("from", "", "task-planning output directory")
	c.Flags().String("repo", "", "repo key from task graph; defaults to the only rendered repo")
	c.Flags().String("feature", "", "feature directory for multi-repo promotion (defaults to output directory slug)")
	return c
}

// PlanMeta is the JSON produced by `plan meta`.
type PlanMeta struct {
	OK          bool   `json:"ok"`
	Path        string `json:"path"`
	Feature     string `json:"feature,omitempty"`
	Repo        string `json:"repo,omitempty"`
	Lang        string `json:"lang,omitempty"`
	Pending     int    `json:"pending"`
	Done        int    `json:"done"`
	Blocked     int    `json:"blocked"`
	TotalTasks  int    `json:"total_tasks"`
	NextTaskNum int    `json:"next_task_num"`
	MaxTaskNum  int    `json:"max_task_num"`
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
			store, openErr := OpenDB(c)
			if openErr != nil && isMigrationRequired(openErr) {
				return EmitErr(openErr.Error(), "run: codedungeon migrate")
			}
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
						RunID:   runID,
						Repo:    meta.Repo,
						TaskID:  fmt.Sprintf("TASK-%03d", num),
						Kind:    "fix",
						Status:  "pending",
						Title:   fmt.Sprintf("Fix (cycle %d): %s", cycle, f.Title),
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
				"ok":        true,
				"created":   created,
				"count":     len(created),
				"next_num":  num,
				"cycle":     cycle,
				"plan_path": to,
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

func addPlanningRunFlags(c *cobra.Command) {
	c.Flags().String("prompt", "", "user prompt to plan")
	c.Flags().String("mode", "full", "planning mode: full, lite, oneshot")
	c.Flags().String("project-context", "", "project context text or path")
	c.Flags().String("out", "", "output directory for planning artifacts")
	c.Flags().String("session", "", "planning session id")
	c.Flags().String("human-gate-policy", taskplanning.HumanGateMaterialAmbiguity, "material_ambiguity | always_before_split | never")
	c.Flags().StringSlice("role", nil, "exploration role to run; repeatable")
	c.Flags().String("project-rules-status", "", "PROJECT_RULES_STATUS override")
	c.Flags().String("project-rules-digest", "", "PROJECT_RULES_DIGEST override")
	c.Flags().String("project-rules-read", "yes", "PROJECT_RULES_READ override")
	c.Flags().Bool("legacy-phase4", false, "mirror completed graph to .codedungeon/plan and .codedungeon/tasks for Phase 5 compatibility")
	c.Flags().Bool("auto-repair", false, "repair deterministic task graph conflicts before rendering")
	c.Flags().Bool("promote", false, "promote rendered artifacts to canonical .codedungeon/plan and .codedungeon/tasks paths")
	c.Flags().String("promote-repo", "", "repo key to promote when graph has more than one repo")
	addPlanningRunnerFlags(c)
}

func addPlanningRunnerFlags(c *cobra.Command) {
	c.Flags().String("runner", "", "planning runner: provider default, claude, codex, or files")
	c.Flags().String("input-dir", "", "input fixture directory for --runner files")
}

func planningRequestFromFlags(c *cobra.Command, fallbackProjectContext string) (taskplanning.Request, taskplanning.Runner, error) {
	prompt, _ := c.Flags().GetString("prompt")
	mode, _ := c.Flags().GetString("mode")
	projectContextArg, _ := c.Flags().GetString("project-context")
	outDir, _ := c.Flags().GetString("out")
	sessionID, _ := c.Flags().GetString("session")
	humanGate, _ := c.Flags().GetString("human-gate-policy")
	roles, _ := c.Flags().GetStringSlice("role")
	rulesStatus, _ := c.Flags().GetString("project-rules-status")
	rulesDigest, _ := c.Flags().GetString("project-rules-digest")
	rulesRead, _ := c.Flags().GetString("project-rules-read")
	runnerName, _ := c.Flags().GetString("runner")
	inputDir, _ := c.Flags().GetString("input-dir")
	autoRepair, _ := c.Flags().GetBool("auto-repair")

	projectContext := fallbackProjectContext
	var err error
	if strings.TrimSpace(projectContextArg) != "" {
		projectContext, err = readOptionalContextArg(projectContextArg)
		if err != nil {
			return taskplanning.Request{}, nil, fmt.Errorf("project-context: %w", err)
		}
	}
	if strings.TrimSpace(projectContext) == "" {
		projectContext, err = defaultPlanningProjectContext(currentProjectRoot())
		if err != nil {
			return taskplanning.Request{}, nil, err
		}
	}
	if rulesStatus == "" || rulesDigest == "" {
		if st, stErr := computeProjectRulesStatus(currentProjectRoot()); stErr == nil {
			if rulesStatus == "" {
				rulesStatus = st.Status
			}
			if rulesDigest == "" {
				rulesDigest = st.RulesDigest
			}
		}
	}
	if rulesRead == "" {
		rulesRead = "yes"
	}
	runner, err := planningRunner(runnerName, inputDir)
	if err != nil {
		return taskplanning.Request{}, nil, err
	}
	return taskplanning.Request{
		SessionID:       sessionID,
		Prompt:          prompt,
		Mode:            mode,
		ProjectContext:  projectContext,
		OutputDir:       outDir,
		Roles:           roles,
		HumanGatePolicy: humanGate,
		ProjectRules: taskplanning.ProjectRulesEnvelope{
			Status: rulesStatus,
			Digest: rulesDigest,
			Read:   rulesRead,
		},
		AutoRepair: autoRepair,
	}, runner, nil
}

func planningRunner(name, inputDir string) (taskplanning.Runner, error) {
	if strings.TrimSpace(name) == "" || strings.EqualFold(strings.TrimSpace(name), "provider") {
		name = defaultPlanningRunnerName(provider.Detect().Name())
	}
	switch strings.TrimSpace(name) {
	case "codex":
		return taskplanning.CodexRunner{WorkDir: currentProjectRoot()}, nil
	case "claude":
		root := currentProjectRoot()
		model := configuredModelAtRoot(root, "reasoning", provider.Claude{}.DefaultModels().Reasoning)
		return taskplanning.ClaudeRunner{WorkDir: root, Model: model}, nil
	case "files":
		return taskplanning.FilesRunner{InputDir: inputDir}, nil
	default:
		return nil, fmt.Errorf("unknown planning runner %q", name)
	}
}

func defaultPlanningRunnerName(providerName string) string {
	switch strings.TrimSpace(providerName) {
	case "claude", "claude-code", "claude-ce":
		return "claude"
	default:
		return "codex"
	}
}

func openPlanningStore(c *cobra.Command) (*db.Store, *db.Run, error) {
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
	return s, run, nil
}

func defaultPlanningOutputDir(prompt, sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		sessionID = "plan-" + time.Now().Format("20060102150405") + "-" + slugifyFeature(prompt)
	}
	return projectPath(currentProjectRoot(), filepath.Join(provider.Detect().StateDir(), "..", "task-planning", sessionID))
}

func defaultPlanningProjectContext(root string) (string, error) {
	candidates := []string{
		filepath.Join(root, ".codedungeon", "project-context.md"),
		filepath.Join(root, ".codedungeon", "project-rules.compact.md"),
	}
	for _, path := range candidates {
		body, err := os.ReadFile(path)
		if err == nil && strings.TrimSpace(string(body)) != "" {
			return string(body), nil
		}
	}
	return "", fmt.Errorf("project-context is required; pass --project-context or create .codedungeon/project-context.md")
}

func persistPlanningSessionStart(s *db.Store, req taskplanning.Request) error {
	return s.UpsertPlanningSession(db.PlanningSession{
		ID:                   req.SessionID,
		RunID:                req.RunID,
		Mode:                 strings.ToUpper(req.Mode),
		Prompt:               req.Prompt,
		PromptSHA256:         shaText(req.Prompt),
		ProjectContextSHA256: shaText(req.ProjectContext),
		RulesStatus:          req.ProjectRules.Status,
		RulesDigest:          req.ProjectRules.Digest,
		RulesRead:            req.ProjectRules.Read,
		HumanGatePolicy:      req.HumanGatePolicy,
		Status:               taskplanning.StatusRunning,
		OutputDir:            req.OutputDir,
	})
}

func planningStatusOrCompleted(status string) string {
	if strings.TrimSpace(status) == "" {
		return taskplanning.StatusCompleted
	}
	return status
}

func persistPlanningResult(s *db.Store, req taskplanning.Request, result taskplanning.Result, execErr error) error {
	status := result.Status
	failure := ""
	if execErr != nil {
		status = taskplanning.StatusFailed
		failure = execErr.Error()
	}
	if status == "" {
		status = taskplanning.StatusCompleted
	}
	if err := s.UpsertPlanningSession(db.PlanningSession{
		ID:                   req.SessionID,
		RunID:                req.RunID,
		Mode:                 strings.ToUpper(req.Mode),
		Prompt:               req.Prompt,
		PromptSHA256:         shaText(req.Prompt),
		ProjectContextSHA256: shaText(req.ProjectContext),
		RulesStatus:          req.ProjectRules.Status,
		RulesDigest:          req.ProjectRules.Digest,
		RulesRead:            req.ProjectRules.Read,
		HumanGatePolicy:      req.HumanGatePolicy,
		Status:               status,
		OutputDir:            req.OutputDir,
		FinishedAt:           time.Now().Unix(),
		FailureMessage:       failure,
	}); err != nil {
		return err
	}
	for _, agent := range result.Agents {
		if _, err := s.InsertPlanningAgent(db.PlanningAgent{
			SessionID:  req.SessionID,
			RunID:      req.RunID,
			Role:       agent.Role,
			Round:      planningRound(agent.Role),
			Provider:   agent.Provider,
			Model:      agent.Model,
			AgentName:  agent.AgentName,
			Status:     "COMPLETED",
			Confidence: agent.Confidence,
			OutputPath: filepath.Join(req.OutputDir, "agent-outputs", agent.Role+".json"),
			Summary:    agent.Summary,
		}); err != nil {
			return err
		}
		if req.RunID != 0 {
			agentRunID, err := s.StartAgentRun(db.AgentRun{
				RunID:           req.RunID,
				Phase:           "4",
				Role:            "planning-" + agent.Role,
				AgentType:       agent.Role,
				AgentName:       agent.AgentName,
				Model:           agent.Model,
				ReasoningEffort: "provider-default",
				TaskPath:        filepath.Join(req.OutputDir, "agent-outputs", agent.Role+".json"),
				InputSummary:    "task planning swarm role " + agent.Role,
			})
			if err == nil {
				_ = s.FinishAgentRun(agentRunID, "COMPLETED", agent.Summary, filepath.Join(req.OutputDir, "agent-outputs", agent.Role+".json"), "")
				_, _ = s.InsertAgentEvent(db.AgentEvent{
					RunID:      req.RunID,
					AgentRunID: agentRunID,
					Phase:      "4",
					Event:      "planning_agent_completed",
					Detail:     agent.Role,
				})
			}
		}
	}
	if result.BlackboardPath != "" {
		if err := persistPlanningBlackboard(s, req.RunID, req.SessionID, result.BlackboardPath); err != nil {
			return err
		}
	}
	if result.Evaluation != nil {
		full, _ := json.Marshal(result.Evaluation)
		questions := make([]string, 0, len(result.Evaluation.Questions))
		for _, question := range result.Evaluation.Questions {
			questions = append(questions, question.Question)
		}
		if _, err := s.InsertPlanningEvaluation(db.PlanningEvaluation{
			SessionID:      req.SessionID,
			RunID:          req.RunID,
			Verdict:        result.Evaluation.Verdict,
			Score:          result.Evaluation.Score,
			NeedsUserInput: result.Evaluation.NeedsUserInput,
			Questions:      questions,
			Issues:         result.Evaluation.Issues,
			FullJSON:       string(full),
		}); err != nil {
			return err
		}
	}
	if result.TaskGraph != nil {
		graphJSON, _ := json.Marshal(result.TaskGraph)
		if _, err := s.InsertPlanningTaskGraph(db.PlanningTaskGraph{
			SessionID: req.SessionID,
			RunID:     req.RunID,
			Version:   result.TaskGraph.Version,
			Status:    result.Status,
			GraphJSON: string(graphJSON),
		}); err != nil {
			return err
		}
		if req.RunID != 0 {
			if err := exportPlanningTasks(s, req.RunID, req.OutputDir, *result.TaskGraph); err != nil {
				return err
			}
		}
	}
	return registerPlanningArtifacts(s, req, result, status)
}

func registerPlanningArtifacts(s *db.Store, req taskplanning.Request, result taskplanning.Result, status string) error {
	registry := artifactreg.NewRegistry(s, currentProjectRoot())
	meta := map[string]any{"status": status, "mode": req.Mode, "human_gate_policy": req.HumanGatePolicy}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"directory", "directory", req.OutputDir},
		{"request", "json", result.RequestPath},
		{"blackboard", "jsonl", result.BlackboardPath},
		{"evaluation", "json", result.EvaluationPath},
		{"task_graph", "json", result.TaskGraphPath},
		{"master", "markdown", result.MasterPath},
		{"result", "json", filepath.Join(req.OutputDir, "planning-result.json")},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: req.RunID, Module: "planning", OwnerType: "planning_session", OwnerID: req.SessionID,
			Phase: "4", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	for _, path := range result.Artifacts {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: req.RunID, Module: "planning", OwnerType: "planning_session", OwnerID: req.SessionID,
			Phase: "4", Role: "artifact", Kind: artifactreg.KindForPath(path), Path: path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	for _, agent := range result.Agents {
		path := filepath.Join(req.OutputDir, "agent-outputs", agent.Role+".json")
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: req.RunID, Module: "planning", OwnerType: "planning_agent", OwnerID: req.SessionID + ":" + agent.Role,
			Phase: "4", Role: "agent_output", Kind: "json", Path: path,
			Metadata: map[string]any{"role": agent.Role, "status": "COMPLETED", "provider": agent.Provider, "model": agent.Model},
		}); err != nil {
			return err
		}
	}
	return nil
}

func persistPlanningBlackboard(s *db.Store, runID int64, sessionID, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry struct {
			Role    string          `json:"role"`
			Kind    string          `json:"kind"`
			Title   string          `json:"title"`
			Summary string          `json:"summary"`
			Full    json.RawMessage `json:"full"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return err
		}
		if _, err := s.InsertPlanningBlackboard(db.PlanningBlackboardEntry{
			SessionID: sessionID,
			RunID:     runID,
			Role:      entry.Role,
			Kind:      entry.Kind,
			Title:     entry.Title,
			Summary:   entry.Summary,
			FullJSON:  string(entry.Full),
		}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

func exportPlanningTasks(s *db.Store, runID int64, outputDir string, graph taskplanning.TaskGraph) error {
	for _, task := range graph.Tasks {
		content := ""
		taskPath := filepath.Join(outputDir, "tasks", filepath.Clean(task.Repo), task.ID+".md")
		if body, err := os.ReadFile(taskPath); err == nil {
			content = string(body)
		} else {
			content = task.Objective
		}
		kind := task.Kind
		if strings.TrimSpace(kind) == "" {
			kind = "dev"
		}
		if err := s.UpsertTask(db.Task{
			RunID:     runID,
			Repo:      task.Repo,
			TaskID:    task.ID,
			Kind:      kind,
			Status:    "pending",
			Title:     task.Title,
			DependsOn: task.DependsOn,
			Content:   content,
		}); err != nil {
			return err
		}
	}
	return nil
}

func mirrorPlanningLegacyArtifacts(root, feature string, result taskplanning.Result) ([]string, error) {
	if result.TaskGraph == nil {
		return nil, nil
	}
	featureSlug := slugifyFeature(feature)
	if featureSlug == "" {
		featureSlug = "task-planning"
	}
	var artifacts []string
	planDir := projectPath(root, provider.Detect().PlanDir())
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return nil, err
	}
	masterPath := filepath.Join(planDir, "MASTER.md")
	if body, err := os.ReadFile(result.MasterPath); err == nil {
		if err := os.WriteFile(masterPath, body, 0o644); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, masterPath)
	}
	copiedPlans := map[string]bool{}
	for _, task := range result.TaskGraph.Tasks {
		repoSlug := slugify(task.Repo)
		if repoSlug == "" || task.Repo == "." {
			repoSlug = "root"
		}
		legacyDir := projectPath(root, filepath.Join(provider.Detect().TasksDir(), featureSlug, repoSlug))
		if err := os.MkdirAll(legacyDir, 0o755); err != nil {
			return nil, err
		}
		srcPlan := filepath.Join(result.OutputDir, "tasks", filepath.Clean(task.Repo), "PLAN.md")
		dstPlan := filepath.Join(legacyDir, "PLAN.md")
		if body, err := os.ReadFile(srcPlan); err == nil && !copiedPlans[dstPlan] {
			if err := os.WriteFile(dstPlan, body, 0o644); err != nil {
				return nil, err
			}
			artifacts = append(artifacts, dstPlan)
			copiedPlans[dstPlan] = true
		}
		srcTask := filepath.Join(result.OutputDir, "tasks", filepath.Clean(task.Repo), task.ID+".md")
		taskNum := strings.TrimPrefix(strings.ToLower(task.ID), "task-")
		dstTask := filepath.Join(legacyDir, fmt.Sprintf("task-%s-%s.md", taskNum, slugify(task.Title)))
		if body, err := os.ReadFile(srcTask); err == nil {
			if err := os.WriteFile(dstTask, body, 0o644); err != nil {
				return nil, err
			}
			artifacts = append(artifacts, dstTask)
		}
	}
	return artifacts, nil
}

type planningPromotion struct {
	Mode      string
	Repos     []string
	Artifacts []string
}

func promotePlanningArtifacts(root, outputDir, repo, feature string) (planningPromotion, error) {
	outputDir = filepath.Clean(outputDir)
	if strings.TrimSpace(repo) == "" {
		repos, err := detectPromotableRepos(outputDir)
		if err != nil {
			return planningPromotion{}, err
		}
		if len(repos) > 1 {
			return promoteAllPlanningRepos(root, outputDir, feature, repos)
		}
		repo = repos[0]
	}
	return promoteSinglePlanningRepo(root, outputDir, repo)
}

func promoteSinglePlanningRepo(root, outputDir, repo string) (planningPromotion, error) {
	repoDir := filepath.Join(outputDir, "tasks", filepath.Clean(repo))
	if repo == "." {
		repoDir = filepath.Join(outputDir, "tasks")
	}
	if _, err := os.Stat(repoDir); err != nil {
		return planningPromotion{}, fmt.Errorf("planning repo artifacts not found for %q: %w", repo, err)
	}

	var artifacts []string
	planDir := projectPath(root, provider.Detect().PlanDir())
	tasksDir := projectPath(root, provider.Detect().TasksDir())
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return planningPromotion{}, err
	}
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return planningPromotion{}, err
	}
	if src := filepath.Join(outputDir, "MASTER.md"); fileExists(src) {
		dst := filepath.Join(planDir, "MASTER.md")
		if err := copyFile(src, dst, 0o644); err != nil {
			return planningPromotion{}, err
		}
		artifacts = append(artifacts, dst)
	}
	if src := filepath.Join(repoDir, "PLAN.md"); fileExists(src) {
		dst := filepath.Join(planDir, "PLAN.md")
		if err := copyFile(src, dst, 0o644); err != nil {
			return planningPromotion{}, err
		}
		artifacts = append(artifacts, dst)
	}
	existing, _ := filepath.Glob(filepath.Join(tasksDir, "task-*.md"))
	for _, path := range existing {
		if err := os.Remove(path); err != nil {
			return planningPromotion{}, err
		}
	}
	entries, err := os.ReadDir(repoDir)
	if err != nil {
		return planningPromotion{}, err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "TASK-") || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		src := filepath.Join(repoDir, entry.Name())
		body, err := os.ReadFile(src)
		if err != nil {
			return planningPromotion{}, err
		}
		id := strings.TrimSuffix(entry.Name(), ".md")
		dst := filepath.Join(tasksDir, strings.ToLower(id)+"-"+slugify(planningTaskTitle(id, string(body)))+".md")
		if err := os.WriteFile(dst, body, 0o644); err != nil {
			return planningPromotion{}, err
		}
		artifacts = append(artifacts, dst)
	}
	return planningPromotion{Mode: "single_repo", Repos: []string{repo}, Artifacts: artifacts}, nil
}

func promoteAllPlanningRepos(root, outputDir, feature string, repos []string) (planningPromotion, error) {
	featureSlug := slugifyFeature(feature)
	if strings.TrimSpace(feature) == "" {
		featureSlug = slugifyFeature(filepath.Base(filepath.Clean(outputDir)))
	}
	if featureSlug == "" {
		featureSlug = "task-planning"
	}
	planDir := projectPath(root, provider.Detect().PlanDir())
	tasksDir := projectPath(root, provider.Detect().TasksDir())
	featureDir := filepath.Join(tasksDir, featureSlug)
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		return planningPromotion{}, err
	}
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		return planningPromotion{}, err
	}
	var artifacts []string
	if src := filepath.Join(outputDir, "MASTER.md"); fileExists(src) {
		dst := filepath.Join(planDir, "MASTER.md")
		if err := copyFile(src, dst, 0o644); err != nil {
			return planningPromotion{}, err
		}
		artifacts = append(artifacts, dst)
	}
	for _, repo := range repos {
		repoDir := filepath.Join(outputDir, "tasks", filepath.Clean(repo))
		if repo == "." {
			repoDir = filepath.Join(outputDir, "tasks")
		}
		if _, err := os.Stat(repoDir); err != nil {
			return planningPromotion{}, fmt.Errorf("planning repo artifacts not found for %q: %w", repo, err)
		}
		repoSlug := slugify(repo)
		if repoSlug == "" || repo == "." {
			repoSlug = "root"
		}
		dstRepoDir := filepath.Join(featureDir, repoSlug)
		if err := os.MkdirAll(dstRepoDir, 0o755); err != nil {
			return planningPromotion{}, err
		}
		if src := filepath.Join(repoDir, "PLAN.md"); fileExists(src) {
			dst := filepath.Join(dstRepoDir, "PLAN.md")
			if err := copyFile(src, dst, 0o644); err != nil {
				return planningPromotion{}, err
			}
			artifacts = append(artifacts, dst)
		}
		entries, err := os.ReadDir(repoDir)
		if err != nil {
			return planningPromotion{}, err
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasPrefix(entry.Name(), "TASK-") {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".md") && !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			src := filepath.Join(repoDir, entry.Name())
			dst := filepath.Join(dstRepoDir, entry.Name())
			if err := copyFile(src, dst, 0o644); err != nil {
				return planningPromotion{}, err
			}
			artifacts = append(artifacts, dst)
		}
	}
	return planningPromotion{Mode: "multi_repo_all", Repos: repos, Artifacts: artifacts}, nil
}

func detectPromotableRepos(outputDir string) ([]string, error) {
	tasksDir := filepath.Join(outputDir, "tasks")
	if fileExists(filepath.Join(tasksDir, "PLAN.md")) {
		return []string{"."}, nil
	}
	var repos []string
	err := filepath.WalkDir(tasksDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "PLAN.md" {
			return nil
		}
		repoDir := filepath.Dir(path)
		rel, err := filepath.Rel(tasksDir, repoDir)
		if err != nil {
			return err
		}
		if rel != "." {
			repos = append(repos, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(repos) == 0 {
		return nil, fmt.Errorf("no promotable planning repo found in %s", tasksDir)
	}
	sort.Strings(repos)
	return repos, nil
}

func promotionRecoveryCommands(outputDir, repo, feature string) []string {
	cmd := "codedungeon plan promote --from " + strconv.Quote(filepath.Clean(outputDir))
	if strings.TrimSpace(feature) != "" {
		cmd += " --feature " + strconv.Quote(feature)
	}
	if strings.TrimSpace(repo) != "" {
		cmd += " --repo " + strconv.Quote(repo)
	}
	return []string{cmd, "codedungeon plan status"}
}

func planningRecoveryCommands(req taskplanning.Request, promote bool, promoteRepo string) []string {
	cmd := "codedungeon plan run --prompt " + strconv.Quote(req.Prompt)
	if strings.TrimSpace(req.Mode) != "" {
		cmd += " --mode " + strconv.Quote(req.Mode)
	}
	if strings.TrimSpace(req.OutputDir) != "" {
		cmd += " --out " + strconv.Quote(req.OutputDir)
	}
	cmd += " --project-context <project-context-path-or-text>"
	if req.AutoRepair {
		cmd += " --auto-repair"
	}
	if promote {
		cmd += " --promote"
	}
	if strings.TrimSpace(promoteRepo) != "" {
		cmd += " --promote-repo " + strconv.Quote(promoteRepo)
	}
	return []string{"codedungeon plan status", cmd}
}

func planningTaskTitle(id, body string) string {
	scanner := bufio.NewScanner(strings.NewReader(body))
	if scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "#"))
		line = strings.TrimSpace(line)
		prefix := id + ":"
		if strings.HasPrefix(line, prefix) {
			title := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if title != "" {
				return title
			}
		}
	}
	return id
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func planningSessionByFlag(s *db.Store, sessionID string) (*db.PlanningSession, error) {
	var session *db.PlanningSession
	var err error
	if strings.TrimSpace(sessionID) != "" {
		session, err = s.PlanningSession(sessionID)
	} else {
		session, err = s.LatestPlanningSession()
	}
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("planning session not found")
	}
	return session, nil
}

func readPlanningRequest(path string) (taskplanning.Request, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return taskplanning.Request{}, err
	}
	var req taskplanning.Request
	if err := json.Unmarshal(body, &req); err != nil {
		return taskplanning.Request{}, err
	}
	return req, nil
}

func planningRound(role string) int {
	switch role {
	case "planning_evaluator":
		return 2
	case "task_splitter":
		return 3
	default:
		return 1
	}
}

func shaText(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}
