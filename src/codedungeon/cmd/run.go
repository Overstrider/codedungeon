package cmd

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/projectcontext"
	"github.com/loldinis/codedungeon/internal/provider"
	qamod "github.com/loldinis/codedungeon/internal/qa"
	"github.com/loldinis/codedungeon/internal/recovery"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

func RunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run",
		Short: "Run a CodeDungeon workflow under autonomous custody",
		RunE:  runStartE,
	}
	addRunStartFlags(c)
	c.AddCommand(runStartCmd())
	c.AddCommand(runStatusCmd())
	c.AddCommand(runAdvanceCmd())
	c.AddCommand(runUnlockCmd())
	c.AddCommand(runFinalizeCmd())
	return c
}

func runStartCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "start",
		Short: "Start an autonomous full/lite/oneshot/rules workflow",
		RunE:  runStartE,
	}
	addRunStartFlags(c)
	return c
}

func runStartE(c *cobra.Command, _ []string) error {
	mode := selectedRunMode(c)
	prompt, _ := c.Flags().GetString("prompt")
	dryRun, _ := c.Flags().GetBool("dry-run")
	if mode == "invalid" {
		return EmitErr("multiple mode flags supplied", "use exactly one of --full, --lite, --oneshot, --auto, --rules")
	}
	if mode == "auto" {
		mode = autoSelectMode(prompt)
	}
	if mode != "rules" && strings.TrimSpace(prompt) == "" {
		return EmitErr("prompt required", "pass --prompt")
	}
	fmt.Printf("CODEDUNGEON_MODE_SELECTED: %s - %s\n", mode, modeReason(mode, prompt))

	root := currentProjectRoot()
	rulesStatus := softRunProjectRulesStatus(root)

	s, err := OpenDB(c)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	defer s.Close()
	if err := s.Init(); err != nil {
		return EmitErr(err.Error(), "")
	}
	active, err := s.ActiveAnyRunSession()
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if active != nil {
		if dryRun {
			return EmitErr("autonomous session already running",
				fmt.Sprintf("run `codedungeon run unlock --reason \"...\"` before starting another workflow (session %s)", active.ID))
		}
		if canAttachRulesToActiveRun(s, active, rulesStatus, mode) {
			return emitActiveAgentFirstContract(root, s, active, rulesStatus)
		}
		if !canResumeAgentFirstRun(s, active, prompt, mode) {
			return EmitErr("autonomous session already running",
				fmt.Sprintf("run `codedungeon run unlock --reason \"...\"` before starting another workflow (session %s)", active.ID))
		}
	}

	branch := ""
	if mode != "rules" {
		branch = "feat/" + slugifyFeature(prompt)
	}
	runID, branch, resumed, err := resolveRunForStart(s, prompt, mode, branch)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if !resumed {
		if err := prepareRunForMode(s, runID, mode); err != nil {
			return EmitErr(err.Error(), "")
		}
	}
	runRow, err := s.GetRun(runID)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	sess, err := reusableAgentFirstSession(s, runID, resumed)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if sess == nil {
		token, err := randomHex(32)
		if err != nil {
			return EmitErr(err.Error(), "")
		}
		sessionID, err := randomHex(16)
		if err != nil {
			return EmitErr(err.Error(), "")
		}
		sessionStatus := runSessionWaitingForAgent
		sessionEvent := "session_started"
		if dryRun {
			sessionStatus = "DRY_RUN"
			sessionEvent = "session_dry_run"
		}
		sess = &db.RunSession{
			ID:          sessionID,
			RunID:       runID,
			Provider:    provider.Detect().Name(),
			Mode:        mode,
			TokenSHA256: hashSessionToken(token),
			Status:      sessionStatus,
		}
		if err := s.InsertRunSession(*sess); err != nil {
			return EmitErr(err.Error(), "")
		}
		_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: sessionEvent, Detail: "agent-first:" + mode})
	}
	contract, err := buildAgentFirstContract(root, s, runRow, sess, rulesStatus, resumed, dryRun, nil)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	return EmitJSON(contract)
}

func recoverAfterProviderChildFailure(root string, s *db.Store, runID int64, sessionID, token string, runnerAgentID int64, childErr error) (string, bool, error) {
	if childErr == nil {
		return "", false, nil
	}
	message := childErr.Error()
	if err := s.UpdateRunSessionStatus(sessionID, "FAILED", message); err != nil {
		return "", false, err
	}
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sessionID, Event: "session_failed", Detail: message})
	if err := recordRunnerAgentEnd(s, runID, runnerAgentID, sessionID, "FAILED", message); err != nil {
		return "", false, err
	}
	if err := abortOpenAgentRuns(s, runID, sessionID, runnerAgentID, "runner child failed before returning control", message); err != nil {
		return "", false, err
	}
	runRow, err := s.GetRun(runID)
	if err != nil {
		return "", false, err
	}
	if runRow == nil || strings.EqualFold(runRow.Mode, "RULES") {
		return "", false, nil
	}
	recoveredSession, err := startRecoveredFinalizationSession(s, runRow, "")
	if err != nil {
		return "", false, err
	}
	if recoveredSession == nil {
		return "", false, nil
	}
	if err := abortOpenAgentRuns(s, runID, recoveredSession.RecoveredFromID, 0, "stale agent closed during provider-child recovery", "final gates recovered after provider child failure"); err != nil {
		return "", false, err
	}
	report, err := finalizeRun(root, s, runRow, recoveredSession.ID, recoveredSession.Token, 0)
	if err != nil {
		_ = s.UpdateRunSessionStatus(recoveredSession.ID, "FAILED", err.Error())
		_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: recoveredSession.ID, Event: "report_failed", Detail: err.Error()})
		_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: recoveredSession.ID, Event: "session_recovery_blocked", Detail: recoveredSession.RecoveredFromID})
		return "", false, err
	}
	if err := completeOpenRunnerAgents(s, runID, recoveredSession.ID, 0, "final report rendered after provider-child recovery"); err != nil {
		return "", false, err
	}
	if err := s.UpdateRunSessionStatus(recoveredSession.ID, "READY_FOR_USER_REVIEW", ""); err != nil {
		return "", false, err
	}
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: recoveredSession.ID, Event: "session_recovered", Detail: recoveredSession.RecoveredFromID})
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: recoveredSession.ID, Event: "provider_child_recovered", Detail: message})
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: recoveredSession.ID, Event: "ready_for_user_review", Detail: runRow.Branch})
	return report, true, nil
}

func addRunStartFlags(c *cobra.Command) {
	c.Flags().Bool("full", false, "run full workflow")
	c.Flags().Bool("lite", false, "run lite workflow")
	c.Flags().Bool("oneshot", false, "run oneshot workflow")
	c.Flags().Bool("auto", false, "auto-select workflow")
	c.Flags().Bool("rules", false, "run Project Rules discovery")
	c.Flags().String("prompt", "", "workflow prompt")
	c.Flags().Bool("dry-run", false, "create and show runner plan without launching provider")
}

func enforceRunProjectRules(root, mode string) (projectRulesStatus, error) {
	st, err := computeProjectRulesStatus(root)
	if err != nil {
		return st, EmitErr("project-rules-gate: "+err.Error(), "")
	}
	if st.Status == "" {
		st.Status = "missing"
	}
	switch mode {
	case "full", "lite":
		if st.Status != "approved" {
			return st, EmitErr("project-rules-gate: PROJECT_RULES_STATUS "+st.Status,
				"run `$codedungeon --rules` and approve rules before full/lite workflows")
		}
	case "oneshot":
		if st.Status != "approved" {
			fmt.Printf("PROJECT_RULES_WARNING: PROJECT_RULES_STATUS %s; oneshot continues with explicit envelope\n", st.Status)
		}
	}
	return st, nil
}

func softRunProjectRulesStatus(root string) projectRulesStatus {
	st, err := computeProjectRulesStatus(root)
	if err != nil {
		return projectRulesStatus{Status: "missing", RulesDigest: "none", StaleReason: err.Error()}
	}
	if strings.TrimSpace(st.Status) == "" {
		st.Status = "missing"
	}
	if strings.TrimSpace(st.RulesDigest) == "" {
		st.RulesDigest = "none"
	}
	return st
}

func reusableAgentFirstSession(s *db.Store, runID int64, resumed bool) (*db.RunSession, error) {
	if !resumed {
		return nil, nil
	}
	latest, err := s.LatestRunSession(runID)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return nil, nil
	}
	if latest.Status == runSessionWaitingForAgent {
		return latest, nil
	}
	return nil, nil
}

func canResumeAgentFirstRun(s *db.Store, sess *db.RunSession, prompt, mode string) bool {
	if sess == nil || !strings.EqualFold(sess.Status, runSessionWaitingForAgent) || strings.EqualFold(mode, "rules") {
		return false
	}
	run, err := s.GetRun(sess.RunID)
	if err != nil || run == nil {
		return false
	}
	return strings.TrimSpace(run.Feature) == strings.TrimSpace(prompt) &&
		strings.EqualFold(strings.TrimSpace(run.Mode), strings.TrimSpace(mode))
}

func canAttachRulesToActiveRun(s *db.Store, sess *db.RunSession, rules projectRulesStatus, mode string) bool {
	if sess == nil || !strings.EqualFold(sess.Status, runSessionWaitingForAgent) || !strings.EqualFold(mode, "rules") {
		return false
	}
	run, err := s.GetRun(sess.RunID)
	if err != nil || run == nil {
		return false
	}
	events, err := s.RunEvents(run.ID)
	if err != nil {
		return false
	}
	return agentFirstCurrentStep(run.Mode, rules, events).ID == "project_rules"
}

func emitActiveAgentFirstContract(root string, s *db.Store, sess *db.RunSession, rules projectRulesStatus) error {
	run, err := s.GetRun(sess.RunID)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	if run == nil {
		return EmitErr("active run not found", "")
	}
	contract, err := buildAgentFirstContract(root, s, run, sess, rules, true, false, nil)
	if err != nil {
		return EmitErr(err.Error(), "")
	}
	return EmitJSON(contract)
}

func latestOrCreateAgentFirstSession(s *db.Store, run *db.Run) (*db.RunSession, error) {
	latest, err := s.LatestRunSession(run.ID)
	if err != nil {
		return nil, err
	}
	if latest != nil {
		return latest, nil
	}
	token, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	mode := strings.ToLower(run.Mode)
	if strings.TrimSpace(mode) == "" {
		mode = "full"
	}
	latest = &db.RunSession{
		ID:          sessionID,
		RunID:       run.ID,
		Provider:    provider.Detect().Name(),
		Mode:        mode,
		TokenSHA256: hashSessionToken(token),
		Status:      runSessionWaitingForAgent,
	}
	if err := s.InsertRunSession(*latest); err != nil {
		return nil, err
	}
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sessionID, Event: "session_started", Detail: "agent-first:" + mode})
	return latest, nil
}

func runProjectRulesEnvelope(st projectRulesStatus) string {
	digest := st.RulesDigest
	if digest == "" {
		digest = "none"
	}
	return fmt.Sprintf("PROJECT_RULES_STATUS: %s\nPROJECT_RULES_DIGEST: %s\nPROJECT_RULES_READ: yes\n",
		st.Status, digest)
}

func resolveRunForStart(s *db.Store, prompt, mode, branch string) (int64, string, bool, error) {
	if mode != "rules" {
		existing, err := s.FindRunByFeatureMode(prompt, strings.ToUpper(mode))
		if err != nil {
			return 0, "", false, err
		}
		if existing != nil {
			latest, err := s.LatestRunSession(existing.ID)
			if err != nil {
				return 0, "", false, err
			}
			if latest != nil {
				switch latest.Status {
				case "RUNNING":
					return 0, "", false, fmt.Errorf("autonomous session already running; run `codedungeon run unlock --reason \"...\"` before retrying session %s", latest.ID)
				case runSessionWaitingForAgent, "FAILED", "ABORTED":
					if existing.Branch != "" {
						branch = existing.Branch
					}
					return existing.ID, branch, true, nil
				}
			}
		}
	}
	runID, err := s.CreateRun(&db.Run{
		Feature:     prompt,
		Branch:      branch,
		Mode:        strings.ToUpper(mode),
		ProjectMode: "SINGLE",
	})
	return runID, branch, false, err
}

func prepareRunForMode(s *db.Store, runID int64, mode string) error {
	switch mode {
	case "lite", "oneshot":
		for _, phase := range db.CanonicalPhases() {
			if phase == "7" {
				break
			}
			if err := s.SetPhaseStatus(runID, phase, "SKIPPED", fmt.Sprintf("%s compact workflow; final gates use QA, review, PR, and report evidence", mode), nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func runStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show latest autonomous runner session",
		RunE: func(c *cobra.Command, _ []string) error {
			root := currentProjectRoot()
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
			sess, err := s.LatestRunSession(run.ID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			rec, err := recovery.InspectRunSession(s, run.ID, time.Now())
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if sess == nil {
				return EmitJSON(map[string]any{"ok": true, "run": run, "session": sess, "recovery": rec})
			}
			contract, err := buildAgentFirstContract(root, s, run, sess, softRunProjectRulesStatus(root), false, false, rec)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			contract.Run = run
			contract.Session = sess
			return EmitJSON(contract)
		},
	}
}

func runAdvanceCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "advance",
		Short: "Record an agent-completed workflow step and return the next agent-first contract",
		RunE: func(c *cobra.Command, _ []string) error {
			step, _ := c.Flags().GetString("step")
			status, _ := c.Flags().GetString("status")
			summary, _ := c.Flags().GetString("summary")
			artifacts, _ := c.Flags().GetStringArray("artifact")
			step = strings.TrimSpace(step)
			status = strings.ToLower(strings.TrimSpace(status))
			if step == "" {
				return EmitErr("--step is required", "")
			}
			if status == "" {
				status = "completed"
			}
			if !validAgentFirstStepStatus(status) {
				return EmitErr("--status must be completed, blocked, or failed", "")
			}
			root := currentProjectRoot()
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "")
			}
			sess, err := latestOrCreateAgentFirstSession(s, run)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			detail := step
			if strings.TrimSpace(summary) != "" {
				detail += ": " + strings.TrimSpace(summary)
			}
			event := "step_" + status
			if _, err := s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sess.ID, Event: event, Detail: detail}); err != nil {
				return EmitErr(err.Error(), "")
			}
			for _, artifact := range artifacts {
				artifact = strings.TrimSpace(artifact)
				if artifact == "" {
					continue
				}
				_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sess.ID, Event: "step_artifact", Detail: step + ": " + artifact})
			}
			if err := advanceAgentFirstPhaseLedger(s, run, step, status, summary, artifacts); err != nil {
				return EmitErr(err.Error(), "")
			}
			if completesAgentFirstRun(run, step, status) {
				if err := s.UpdateRunSessionStatus(sess.ID, runStatusCompleted, ""); err != nil {
					return EmitErr(err.Error(), "")
				}
				sess.Status = runStatusCompleted
				_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sess.ID, Event: "session_completed", Detail: step})
			}
			contract, err := buildAgentFirstContract(root, s, run, sess, softRunProjectRulesStatus(root), false, false, nil)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(contract)
		},
	}
	c.Flags().String("step", "", "workflow step id, such as planning, execution, qa, code_review, or finalization")
	c.Flags().String("status", "completed", "completed, blocked, or failed")
	c.Flags().String("summary", "", "short agent summary for the step")
	c.Flags().StringArray("artifact", nil, "artifact path produced by the step; repeatable")
	return c
}

func validAgentFirstStepStatus(status string) bool {
	switch status {
	case "completed", "blocked", "failed":
		return true
	default:
		return false
	}
}

func completesAgentFirstRun(run *db.Run, step, status string) bool {
	return run != nil &&
		strings.EqualFold(run.Mode, "RULES") &&
		strings.EqualFold(step, "project_rules") &&
		strings.EqualFold(status, "completed")
}

func advanceAgentFirstPhaseLedger(s *db.Store, run *db.Run, step, status, summary string, artifacts []string) error {
	if run == nil || !strings.EqualFold(run.Mode, "FULL") || !strings.EqualFold(status, "completed") {
		return nil
	}
	cleanArtifacts := compactNonEmptyStrings(artifacts)
	stepSummary := strings.TrimSpace(summary)
	if stepSummary == "" {
		stepSummary = step + " completed by agent-first run"
	}
	type phaseUpdate struct {
		phase   string
		summary string
		promise string
	}
	var updates []phaseUpdate
	switch strings.ToLower(strings.TrimSpace(step)) {
	case "planning":
		for _, phase := range []string{"0", "1", "2'", "3.5", "4"} {
			updates = append(updates, phaseUpdate{
				phase:   phase,
				summary: "Agent-first planning completed: " + stepSummary,
				promise: "PHASE_" + phasePromiseID(phase) + "_COMPLETE: agent-first planning contract recorded.",
			})
		}
	case "execution":
		updates = append(updates, phaseUpdate{phase: "5", summary: "Agent-first execution completed: " + stepSummary, promise: "PHASE_5_COMPLETE: implementation evidence recorded."})
	case "code_review":
		updates = append(updates,
			phaseUpdate{phase: "5.5", summary: "Agent-first review completed: " + stepSummary, promise: "PHASE_55_COMPLETE: review evidence recorded."},
			phaseUpdate{phase: "5.6", summary: "Agent-first review fix loop completed: " + stepSummary, promise: "PHASE_56_COMPLETE: no blocking review findings remain."},
		)
	case "qa":
		updates = append(updates, phaseUpdate{phase: "6", summary: "Agent-first QA completed: " + stepSummary, promise: "PHASE_6_COMPLETE: verification evidence recorded."})
	}
	for _, update := range updates {
		if err := autoDonePhase(s, run.ID, update.phase, update.summary, cleanArtifacts, update.promise); err != nil {
			return err
		}
	}
	return nil
}

func compactNonEmptyStrings(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func phasePromiseID(phase string) string {
	return strings.NewReplacer("'", "", ".", "").Replace(phase)
}

func runFinalizeCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "finalize",
		Short: "Close final gates, render report, and clean stale agent telemetry",
		RunE: func(c *cobra.Command, _ []string) error {
			dryRun, _ := c.Flags().GetBool("dry-run")
			root := currentProjectRoot()
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			if err := s.Init(); err != nil {
				return EmitErr(err.Error(), "")
			}
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "")
			}
			sessionID := ""
			token := ""
			if sess, err := s.ActiveRunSession(run.ID); err != nil {
				return EmitErr(err.Error(), "")
			} else if sess != nil {
				if err := requireAutonomousCustody(s, run.ID, "run finalize"); err != nil {
					return err
				}
				sessionID = sess.ID
				token = os.Getenv(envSessionToken)
			} else if sess, err := s.LatestRunSession(run.ID); err != nil {
				return EmitErr(err.Error(), "")
			} else if sess != nil && sess.Status == runSessionWaitingForAgent {
				sessionID = sess.ID
			}
			if dryRun {
				plan, err := prepareFinalization(root, s, run, sessionID, token, 0)
				if err != nil {
					return EmitJSON(map[string]any{
						"ok":            false,
						"dry_run":       true,
						"run_id":        run.ID,
						"blocker":       err.Error(),
						"next_commands": finalizeDiagnosticCommands(err),
					})
				}
				return EmitJSON(map[string]any{
					"ok":             true,
					"dry_run":        true,
					"run_id":         run.ID,
					"planned_phases": finalizationPhaseNames(plan.phases),
					"report_path":    plan.reportPath,
				})
			}
			recoveredSession, err := startRecoveredFinalizationSession(s, run, sessionID)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if recoveredSession != nil {
				sessionID = recoveredSession.ID
				token = recoveredSession.Token
				if err := abortOpenAgentRuns(s, run.ID, recoveredSession.RecoveredFromID, 0, "stale agent closed during recovered finalization", "final gates recovered after session failure"); err != nil {
					return EmitErr(err.Error(), "")
				}
			}
			report, err := finalizeRun(root, s, run, sessionID, token, 0)
			if err != nil {
				_ = abortOpenAgentRuns(s, run.ID, sessionID, 0, "finalize failed before READY_FOR_USER_REVIEW", err.Error())
				if recoveredSession != nil {
					_ = s.UpdateRunSessionStatus(sessionID, "FAILED", err.Error())
					_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sessionID, Event: "report_failed", Detail: err.Error()})
				}
				return EmitCustodyErr("run finalize failed: "+err.Error(), runnerRecoveryCommands(err))
			}
			if sessionID != "" {
				if err := completeOpenRunnerAgents(s, run.ID, sessionID, 0, "final report rendered"); err != nil {
					return EmitErr(err.Error(), "")
				}
				if err := s.UpdateRunSessionStatus(sessionID, "READY_FOR_USER_REVIEW", ""); err != nil {
					return EmitErr(err.Error(), "")
				}
				if recoveredSession != nil {
					_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sessionID, Event: "session_recovered", Detail: recoveredSession.RecoveredFromID})
				}
				_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sessionID, Event: "ready_for_user_review", Detail: run.Branch})
			}
			fmt.Print(report)
			return nil
		},
	}
	c.Flags().Bool("dry-run", false, "diagnose finalization gates without mutating run state")
	return c
}

type recoveredFinalizationSession struct {
	ID              string
	Token           string
	RecoveredFromID string
}

func startRecoveredFinalizationSession(s *db.Store, run *db.Run, currentSessionID string) (*recoveredFinalizationSession, error) {
	if currentSessionID != "" || run == nil {
		return nil, nil
	}
	latest, err := s.LatestRunSession(run.ID)
	if err != nil {
		return nil, err
	}
	if latest == nil || !recoverableRunSessionStatus(latest.Status) {
		return nil, nil
	}
	sessionID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	token, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	mode := latest.Mode
	if strings.TrimSpace(mode) == "" {
		mode = strings.ToLower(run.Mode)
	}
	providerName := latest.Provider
	if strings.TrimSpace(providerName) == "" {
		providerName = provider.Detect().Name()
	}
	if err := s.InsertRunSession(db.RunSession{
		ID:          sessionID,
		RunID:       run.ID,
		Provider:    providerName,
		Mode:        mode,
		TokenSHA256: hashSessionToken(token),
		Status:      "RUNNING",
	}); err != nil {
		return nil, err
	}
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: run.ID, SessionID: sessionID, Event: "session_recovery_started", Detail: latest.ID})
	return &recoveredFinalizationSession{ID: sessionID, Token: token, RecoveredFromID: latest.ID}, nil
}

func recoverableRunSessionStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FAILED", "ABORTED":
		return true
	default:
		return false
	}
}

func finalizationPhaseNames(phases []finalizablePhase) []string {
	out := make([]string, 0, len(phases))
	for _, phase := range phases {
		out = append(out, phase.phase)
	}
	return out
}

func finalizeDiagnosticCommands(err error) []string {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "review"):
		return []string{"codedungeon code-review --url <pr-url> --project-context .codedungeon/project-context.md --task-context .codedungeon/plan/PLAN.md --post"}
	case strings.Contains(msg, "verification"):
		return []string{"codedungeon qa run --phase 6 --cmd-file <verify-command-file> --fresh"}
	case strings.Contains(msg, "branch"):
		return []string{"git push -u origin <branch>"}
	case strings.Contains(msg, "phase"):
		return []string{"codedungeon status", "codedungeon observe"}
	default:
		return []string{"codedungeon observe", "codedungeon run finalize --dry-run"}
	}
}

func runnerRecoveryCommands(err error) []string {
	cmds := []string{"codedungeon observe", "codedungeon run finalize --dry-run"}
	cmds = append(cmds, finalizeDiagnosticCommands(err)...)
	return uniqueStrings(cmds)
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func runUnlockCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "unlock",
		Short: "Abort a stale/crashed autonomous session",
		RunE: func(c *cobra.Command, _ []string) error {
			reason, _ := c.Flags().GetString("reason")
			if strings.TrimSpace(reason) == "" {
				return EmitErr("--reason is required", "")
			}
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
			rec, err := recovery.AbortActiveRunSession(s, run.ID, reason, recovery.AbortOptions{
				IncludeAutonomousRunner: true,
				Summary:                 "manual recovery unlock",
			})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if rec.Status == "no_active_session" {
				agentFirstRec, err := abortWaitingAgentFirstSession(s, run.ID, reason)
				if err != nil {
					return EmitErr(err.Error(), "")
				}
				if agentFirstRec != nil {
					rec = agentFirstRec
				}
			}
			return EmitJSON(map[string]any{
				"ok":         true,
				"session_id": rec.SessionID,
				"status":     rec.Status,
				"recovery":   rec,
			})
		},
	}
	c.Flags().String("reason", "", "why the session is being aborted")
	return c
}

func abortWaitingAgentFirstSession(s *db.Store, runID int64, reason string) (*recovery.RunRecoveryReport, error) {
	latest, err := s.LatestRunSession(runID)
	if err != nil {
		return nil, err
	}
	if latest == nil || latest.Status != runSessionWaitingForAgent {
		return nil, nil
	}
	if err := s.UpdateRunSessionStatus(latest.ID, "ABORTED", reason); err != nil {
		return nil, err
	}
	_, _ = s.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: latest.ID, Event: "session_aborted", Detail: reason})
	return &recovery.RunRecoveryReport{
		Status:    "ABORTED",
		SessionID: latest.ID,
		Active:    false,
		NextCommands: []string{
			"codedungeon run status",
			"codedungeon run --full --prompt <prompt>",
		},
	}, nil
}

func selectedRunMode(c *cobra.Command) string {
	flags := []struct {
		name string
		mode string
	}{
		{"full", "full"},
		{"lite", "lite"},
		{"oneshot", "oneshot"},
		{"auto", "auto"},
		{"rules", "rules"},
	}
	selected := "auto"
	count := 0
	for _, f := range flags {
		if ok, _ := c.Flags().GetBool(f.name); ok {
			selected = f.mode
			count++
		}
	}
	if count > 1 {
		return "invalid"
	}
	return selected
}

func autoSelectMode(prompt string) string {
	lower := strings.ToLower(prompt)
	if strings.Contains(lower, "plan") || strings.Contains(lower, "phase") || strings.Contains(lower, "multi") {
		return "full"
	}
	if strings.Contains(lower, ".codedungeon/plans") {
		return "lite"
	}
	return "oneshot"
}

func modeReason(mode, prompt string) string {
	switch mode {
	case "full":
		return "full phase lifecycle requested or selected"
	case "lite":
		return "planned lightweight workflow selected"
	case "oneshot":
		return "small direct workflow selected"
	case "rules":
		return "project rules discovery selected"
	default:
		return "automatic selection"
	}
}

func verifyGitHubPREnvironment(root string) error {
	if out, errb, err := run(root, "git", "remote", "get-url", "origin"); err != nil || strings.TrimSpace(out) == "" {
		return EmitErr("GitHub origin remote required", strings.TrimSpace(errb))
	}
	if _, errb, err := run(root, "gh", "auth", "status"); err != nil {
		return EmitErr("GitHub CLI authentication required", strings.TrimSpace(errb))
	}
	return nil
}

func randomHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func slugifyFeature(prompt string) string {
	lower := strings.ToLower(prompt)
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := strings.Trim(re.ReplaceAllString(lower, "-"), "-")
	if slug == "" {
		return "codedungeon-run"
	}
	if len(slug) > 48 {
		slug = strings.Trim(slug[:48], "-")
	}
	return slug
}

func autonomousChildPrompt(mode, prompt, branch string) string {
	return autonomousChildPromptForProvider(provider.Detect().Name(), mode, prompt, branch)
}

func autonomousChildPromptForProvider(providerName, mode, prompt, branch string) string {
	var skill string
	switch mode {
	case "full":
		skill = childWorkflowPath(providerName, "main-quest")
	case "lite":
		skill = childWorkflowPath(providerName, "side-quest")
	case "oneshot":
		skill = childWorkflowPath(providerName, "one-shot")
	case "rules":
		skill = childWorkflowPath(providerName, "codedungeon")
	}
	cmdName := codedungeonCommandForProvider(providerName)
	return fmt.Sprintf(`You are the CodeDungeon autonomous child runner.

Load and follow %s. The parent agent is no longer in control of workflow steps.

Required custody:
- Use only the project-local codedungeon binary for workflow evidence.
- A run and custody session already exist. Do not run phase init or create another run.
- Do not manually write review reports, final reports, persona evidence, or DB rows.
- Do not write review reports manually.
- Do not write final reports manually.
- Code review is isolated. Run the standalone module with %s code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post; do not use legacy review run as final approval evidence.
- Run verification through %s qa run --phase 6 --fresh.
- Finalize with %s run finalize. READY_FOR_USER_REVIEW can only come from codedungeon run finalize.
- Do not merge pull requests.
- Finish with the PR open for human review and merge.

Agent telemetry:
- Record every subagent, persona, validator, classifier, worker, or phase delegation.
- Before spawning it, run: %s trace agent-start --phase "<phase>" --role "<role>" --agent-type "<agent_type>" --agent-name "<name>" --model "<model>" --reasoning-effort "<effort>" --task "<artifact-or-task>" --input-summary "<short summary>".
- Save the returned agent_run_id. After the agent returns, run: %s trace agent-end --id "<agent_run_id>" --status COMPLETED|FAILED|ABORTED --summary "<short result>" --artifact "<primary artifact>" --error "<failure message if any>".
- Telemetry is informational and does not replace phase, QA, review, PR, or report gates.

Workflow mode: %s
Target branch: %s
User prompt:
%s
`, skill, cmdName, cmdName, cmdName, cmdName, cmdName, mode, branch, prompt)
}

func childWorkflowPath(providerName, workflow string) string {
	switch providerName {
	case "codex", "codex-cli":
		return ".agents/skills/" + workflow + "/SKILL.md"
	default:
		return ".codedungeon/commands/" + workflow + ".md"
	}
}

type finalizablePhase struct {
	phase     string
	summary   string
	artifacts []string
	promise   string
}

type finalizationPlan struct {
	phases     []finalizablePhase
	report     string
	repos      []reportRepo
	reportPath string
}

func finalizeRun(root string, s *db.Store, run *db.Run, sessionID, token string, excludeAgentID int64) (string, error) {
	if run == nil {
		return "", fmt.Errorf("no active run")
	}
	if err := ensureWorkflowQA(root, s, run); err != nil {
		return "", err
	}
	plan, err := prepareFinalization(root, s, run, sessionID, token, excludeAgentID)
	if err != nil {
		return "", err
	}
	if err := commitFinalization(root, s, run, sessionID, excludeAgentID, plan); err != nil {
		return "", err
	}
	return plan.report, nil
}

func ensureWorkflowQA(root string, s *db.Store, run *db.Run) error {
	records, err := s.VerificationRecords(run.ID, "6")
	if err != nil {
		return err
	}
	if err := validateVerificationRecords(records); err == nil {
		return nil
	}
	result, err := qamod.Run(context.Background(), qamod.Request{
		Root:       root,
		RunID:      run.ID,
		Entrypoint: qamod.EntrypointWorkflow,
		Mode:       qamod.ModeAuto,
		Phase:      "6",
		Fresh:      true,
		Store:      s,
	})
	if err != nil {
		return fmt.Errorf("workflow-qa: %w", err)
	}
	if result.Status != qamod.StatusPass {
		return fmt.Errorf("workflow-qa: %s (%s)", result.Status, result.SummaryPath)
	}
	return nil
}

func prepareFinalization(root string, s *db.Store, run *db.Run, sessionID, token string, excludeAgentID int64) (*finalizationPlan, error) {
	if err := validateFinalizationProjectRules(root); err != nil {
		return nil, err
	}
	var planned []finalizablePhase
	var virtualPhases []db.Phase
	if err := withFinalReportEnv(root, run.ID, sessionID, token, func() error {
		var err error
		planned, virtualPhases, err = planFinalizablePhases(s, run)
		return err
	}); err != nil {
		return nil, err
	}
	agents, err := s.AgentRuns(run.ID)
	if err != nil {
		return nil, err
	}
	virtualAgents := virtualFinalizationAgents(agents, sessionID, excludeAgentID)
	if err := withFinalReportEnv(root, run.ID, sessionID, token, func() error {
		return validateReportGatesWithPhases(s, run, virtualPhases, false, true)
	}); err != nil {
		return nil, fmt.Errorf("report-gate: %w", err)
	}
	var report string
	var repos []reportRepo
	if err := withFinalReportEnv(root, run.ID, sessionID, token, func() error {
		var err error
		report, repos, err = buildReportSnapshot(s, run, false, virtualPhases, virtualAgents)
		return err
	}); err != nil {
		return nil, err
	}
	return &finalizationPlan{
		phases:     planned,
		report:     report,
		repos:      repos,
		reportPath: filepath.Join(root, codedungeonDir, "reports", fmt.Sprintf("run-%d.md", run.ID)),
	}, nil
}

func validateFinalizationProjectRules(root string) error {
	st, err := computeProjectRulesStatus(root)
	if err != nil {
		return fmt.Errorf("project-rules-gate: %w", err)
	}
	if !st.OK || !strings.EqualFold(st.Status, "approved") || len(st.Missing) > 0 || strings.TrimSpace(st.RulesDigest) == "" {
		reason := fallback(st.StaleReason, strings.Join(st.Missing, ", "))
		if reason != "" {
			return fmt.Errorf("project-rules-gate: PROJECT_RULES_STATUS %s (%s)", fallback(st.Status, "missing"), reason)
		}
		return fmt.Errorf("project-rules-gate: PROJECT_RULES_STATUS %s", fallback(st.Status, "missing"))
	}
	return nil
}

func planFinalizablePhases(s *db.Store, run *db.Run) ([]finalizablePhase, []db.Phase, error) {
	phases, err := s.AllPhases(run.ID)
	if err != nil {
		return nil, nil, err
	}
	if err := phasesBeforeFiveComplete(phases); err != nil {
		return nil, nil, err
	}
	reviewEvidence, err := s.LatestReviewEvidence(run.ID)
	if err != nil {
		return nil, nil, err
	}
	if err := validateReviewEvidence(reviewEvidence); err != nil {
		return nil, nil, err
	}
	if err := validateBranchPushed(run.Branch); err != nil {
		return nil, nil, fmt.Errorf("phase-5-gate: %w", err)
	}
	records, err := s.VerificationRecords(run.ID, "6")
	if err != nil {
		return nil, nil, err
	}
	if err := validateVerificationRecords(records); err != nil {
		return nil, nil, err
	}

	var planned []finalizablePhase
	if phaseNeedsFinalization(phases, "5") {
		planned = append(planned, finalizablePhase{
			phase:     "5",
			summary:   "Implementation and adversarial review completed with APPROVED verdict.",
			artifacts: []string{reviewEvidence.ReviewJSONPath, reviewEvidence.ReviewDir},
			promise:   "PHASE_5_COMPLETE: implementation and review evidence are approved.",
		})
	}
	if phaseNeedsFinalization(phases, "5.5") {
		planned = append(planned, finalizablePhase{
			phase:     "5.5",
			summary:   "Review evidence consolidated and approved.",
			artifacts: []string{reviewEvidence.ReviewJSONPath},
			promise:   "PHASE_55_COMPLETE: review evidence is recorded.",
		})
	}
	if phaseNeedsFinalization(phases, "5.6") {
		planned = append(planned, finalizablePhase{
			phase:     "5.6",
			summary:   "Review fix loop converged with approved final evidence.",
			artifacts: []string{reviewEvidence.ReviewJSONPath},
			promise:   "PHASE_56_COMPLETE: no blocking review findings remain.",
		})
	}
	if phaseNeedsFinalization(phases, "6") {
		planned = append(planned, finalizablePhase{
			phase:     "6",
			summary:   "Verification ledger is PASS.",
			artifacts: latestVerificationLogPaths(records),
			promise:   "PHASE_6_COMPLETE: verification ledger is PASS.",
		})
	}
	return planned, applyVirtualFinalizedPhases(phases, planned), nil
}

func phasesBeforeFiveComplete(phases []db.Phase) error {
	for _, phase := range phases {
		if phase.Phase == "5" {
			return nil
		}
		if phase.Status != "DONE" && phase.Status != "SKIPPED" {
			return fmt.Errorf("phase %s is %s", phase.Phase, phase.Status)
		}
	}
	return fmt.Errorf("phase 5 not found")
}

func phaseNeedsFinalization(phases []db.Phase, phase string) bool {
	for _, current := range phases {
		if current.Phase != phase {
			continue
		}
		return current.Status != "DONE" && current.Status != "SKIPPED"
	}
	return false
}

func applyVirtualFinalizedPhases(phases []db.Phase, planned []finalizablePhase) []db.Phase {
	out := make([]db.Phase, len(phases))
	copy(out, phases)
	plannedByPhase := map[string]finalizablePhase{}
	for _, phase := range planned {
		plannedByPhase[phase.phase] = phase
	}
	for i := range out {
		plannedPhase, ok := plannedByPhase[out[i].Phase]
		if !ok {
			continue
		}
		out[i].Status = "DONE"
		out[i].Notes = plannedPhase.summary
		out[i].Artifacts = append([]string(nil), plannedPhase.artifacts...)
	}
	return out
}

func virtualFinalizationAgents(agents []db.AgentRun, sessionID string, excludeAgentID int64) []db.AgentRun {
	out := make([]db.AgentRun, len(agents))
	copy(out, agents)
	for i := range out {
		if out[i].Status != "RUNNING" {
			continue
		}
		if sessionID != "" && out[i].SessionID != sessionID {
			continue
		}
		switch out[i].Role {
		case "autonomous-runner":
			out[i].Status = "COMPLETED"
			out[i].OutputSummary = "final report rendered"
		default:
			if out[i].ID == excludeAgentID {
				continue
			}
			out[i].Status = "ABORTED"
			out[i].OutputSummary = "stale agent closed during finalization"
			out[i].ErrorMessage = "final gates are being evaluated"
		}
	}
	return out
}

func commitFinalization(root string, s *db.Store, run *db.Run, sessionID string, excludeAgentID int64, plan *finalizationPlan) error {
	if err := abortOpenAgentRuns(s, run.ID, sessionID, excludeAgentID, "stale agent closed during finalization", "final gates are being evaluated"); err != nil {
		return err
	}
	if err := writeReportMemoryFiles(root, run, plan.repos, plan.report); err != nil {
		return err
	}
	phase7Handoff, err := prepareFinalReportPhaseHandoff(root, s, run.ID, plan.reportPath)
	if err != nil {
		return err
	}
	if err := commitFinalizationState(s, run.ID, plan, phase7Handoff); err != nil {
		return err
	}
	if err := registerFinalizationArtifacts(root, s, run.ID, plan, phase7Handoff); err != nil {
		return err
	}
	proposal, err := projectcontext.ProposeFromRun(root, projectcontext.NewSQLiteStore(s), run.ID)
	if err != nil {
		return fmt.Errorf("project-context-proposal: %w", err)
	}
	_, _ = s.InsertRunEvent(db.RunEvent{
		RunID:     run.ID,
		SessionID: sessionID,
		Event:     "project_context_proposal_created",
		Detail:    fmt.Sprintf("proposal:%d %s", proposal.ID, projectcontext.ProposalRel),
	})
	return nil
}

func withFinalReportEnv(root string, runID int64, sessionID, token string, fn func() error) error {
	oldWD, wdErr := os.Getwd()
	if wdErr == nil {
		if err := os.Chdir(root); err != nil {
			return err
		}
		defer func() { _ = os.Chdir(oldWD) }()
	}
	oldRunID, oldSessionID, oldToken := os.Getenv(envRunID), os.Getenv(envSessionID), os.Getenv(envSessionToken)
	_ = os.Setenv(envRunID, fmt.Sprintf("%d", runID))
	_ = os.Setenv(envSessionID, sessionID)
	_ = os.Setenv(envSessionToken, token)
	defer func() {
		restoreEnv(envRunID, oldRunID)
		restoreEnv(envSessionID, oldSessionID)
		restoreEnv(envSessionToken, oldToken)
	}()
	return fn()
}

func autoDonePhase(s *db.Store, runID int64, phase, summary string, artifacts []string, promise string) error {
	current, err := s.GetPhase(runID, phase)
	if err != nil {
		return err
	}
	if current == nil || current.Status == "DONE" || current.Status == "SKIPPED" {
		return nil
	}
	if err := s.SetPhaseStatus(runID, phase, "DONE", summary, artifacts); err != nil {
		return err
	}
	handoff := &db.Handoff{
		RunID:     runID,
		Phase:     phase,
		Summary:   summary,
		Artifacts: artifacts,
		NextInput: "codedungeon run finalize",
		Promise:   promise,
	}
	handoff.RenderedMD = renderHandoff(handoff, "DONE")
	return s.UpsertHandoff(handoff)
}

func markFinalReportPhaseDone(s *db.Store, runID int64, reportPath string) error {
	return markFinalReportPhaseDoneAtRoot(currentProjectRoot(), s, runID, reportPath)
}

func markFinalReportPhaseDoneAtRoot(root string, s *db.Store, runID int64, reportPath string) error {
	handoff, err := prepareFinalReportPhaseHandoff(root, s, runID, reportPath)
	if err != nil || handoff == nil {
		return err
	}
	if err := s.SetPhaseStatus(runID, "7", "DONE", handoff.Summary, handoff.Artifacts); err != nil {
		return err
	}
	return s.UpsertHandoff(handoff)
}

func prepareFinalReportPhaseHandoff(root string, s *db.Store, runID int64, reportPath string) (*db.Handoff, error) {
	current, err := s.GetPhase(runID, "7")
	if err != nil {
		return nil, err
	}
	if current == nil || current.Status == "DONE" || current.Status == "SKIPPED" {
		return nil, nil
	}
	summary := "Final report rendered with READY_FOR_USER_REVIEW."
	artifacts := []string{reportPath}
	handoff := &db.Handoff{
		RunID:     runID,
		Phase:     "7",
		Summary:   summary,
		Artifacts: artifacts,
		NextInput: "human review and merge",
		Promise:   "PHASE_7_COMPLETE: READY_FOR_USER_REVIEW report rendered.",
	}
	handoff.RenderedMD = renderHandoff(handoff, "DONE")
	outPath := projectPath(root, filepath.Join(provider.Detect().StateDir(), "phase-7-output.md"))
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(outPath, []byte(handoff.RenderedMD), 0o644); err != nil {
		return nil, err
	}
	return handoff, nil
}

func commitFinalizationState(s *db.Store, runID int64, plan *finalizationPlan, phase7Handoff *db.Handoff) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	sum := sha256.Sum256([]byte(plan.report))
	if _, err := tx.Exec(`
        INSERT INTO report_evidence(run_id, report_path, sha256, created_at)
        VALUES (?,?,?,?)`, runID, plan.reportPath, hex.EncodeToString(sum[:]), time.Now().Unix()); err != nil {
		return err
	}
	for _, phase := range plan.phases {
		if err := setPhaseStatusTx(tx, runID, phase.phase, "DONE", phase.summary, phase.artifacts); err != nil {
			return err
		}
		handoff := &db.Handoff{
			RunID:     runID,
			Phase:     phase.phase,
			Summary:   phase.summary,
			Artifacts: phase.artifacts,
			NextInput: "codedungeon run finalize",
			Promise:   phase.promise,
		}
		handoff.RenderedMD = renderHandoff(handoff, "DONE")
		if err := upsertHandoffTx(tx, handoff); err != nil {
			return err
		}
	}
	if phase7Handoff != nil {
		if err := setPhaseStatusTx(tx, runID, "7", "DONE", phase7Handoff.Summary, phase7Handoff.Artifacts); err != nil {
			return err
		}
		if err := upsertHandoffTx(tx, phase7Handoff); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func registerFinalizationArtifacts(root string, s *db.Store, runID int64, plan *finalizationPlan, phase7Handoff *db.Handoff) error {
	registry := artifactreg.NewRegistry(s, root)
	reportEvidence, _ := s.LatestReportEvidence(runID)
	ownerID := fmt.Sprintf("run-%d", runID)
	sum := ""
	if reportEvidence != nil {
		ownerID = fmt.Sprintf("%d", reportEvidence.ID)
		sum = reportEvidence.SHA256
	}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"report", "markdown", plan.reportPath},
		{"memory", "markdown", filepath.Join(root, codedungeonDir, "memory", "runs", fmt.Sprintf("run-%d.md", runID))},
		{"phase_output", "markdown", projectPath(root, filepath.Join(provider.Detect().StateDir(), "phase-7-output.md"))},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "report", OwnerType: "report_evidence", OwnerID: ownerID,
			Phase: "7", Role: item.role, Kind: item.kind, Path: item.path,
			Metadata: map[string]any{"sha256": sum},
		}); err != nil {
			return err
		}
	}
	for _, phase := range plan.phases {
		for _, path := range phase.artifacts {
			if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
				RunID: runID, Module: "phase", OwnerType: "phase", OwnerID: fmt.Sprintf("%d:%s", runID, phase.phase),
				Phase: phase.phase, Role: "artifact", Kind: artifactreg.KindForPathAtRoot(root, path), Path: path,
				Metadata: map[string]any{"summary": phase.summary},
			}); err != nil {
				return err
			}
		}
	}
	if phase7Handoff != nil {
		for _, path := range phase7Handoff.Artifacts {
			if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
				RunID: runID, Module: "handoff", OwnerType: "handoff", OwnerID: fmt.Sprintf("%d:%s", runID, phase7Handoff.Phase),
				Phase: phase7Handoff.Phase, Role: "artifact", Kind: artifactreg.KindForPathAtRoot(root, path), Path: path,
				Metadata: map[string]any{"summary": phase7Handoff.Summary},
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

func setPhaseStatusTx(tx *sql.Tx, runID int64, phase, status, notes string, artifacts []string) error {
	var artJSON string
	if len(artifacts) > 0 {
		b, _ := json.Marshal(artifacts)
		artJSON = string(b)
	}
	_, err := tx.Exec(`UPDATE phases SET status=?, notes=?, artifacts=?, finished_at=? WHERE run_id=? AND phase=?`,
		status, nullableString(notes), nullableString(artJSON), time.Now().Unix(), runID, phase)
	return err
}

func upsertHandoffTx(tx *sql.Tx, h *db.Handoff) error {
	dec, _ := json.Marshal(h.Decisions)
	art, _ := json.Marshal(h.Artifacts)
	tr, _ := json.Marshal(h.Traps)
	oq, _ := json.Marshal(h.OpenQuestions)
	_, err := tx.Exec(`
        INSERT INTO handoffs(run_id, phase, summary, decisions, artifacts, traps, open_questions, next_input, promise, rendered_md, created_at)
        VALUES (?,?,?,?,?,?,?,?,?,?,?)
        ON CONFLICT(run_id, phase) DO UPDATE SET
          summary=excluded.summary,
          decisions=excluded.decisions,
          artifacts=excluded.artifacts,
          traps=excluded.traps,
          open_questions=excluded.open_questions,
          next_input=excluded.next_input,
          promise=excluded.promise,
          rendered_md=excluded.rendered_md,
          created_at=excluded.created_at`,
		h.RunID, h.Phase, nullableString(h.Summary), string(dec), string(art), string(tr), string(oq),
		nullableString(h.NextInput), nullableString(h.Promise), h.RenderedMD, time.Now().Unix())
	return err
}

func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}

func latestVerificationLogPaths(records []db.VerificationRecord) []string {
	var out []string
	for _, record := range latestVerificationRecords(records) {
		if record.LogPath != "" {
			out = append(out, record.LogPath)
		}
	}
	return out
}

func abortOpenAgentRuns(s *db.Store, runID int64, sessionID string, excludeAgentID int64, summary, errorMessage string) error {
	_, err := recovery.AbortOpenAgentRuns(s, runID, recovery.AgentAbortOptions{
		SessionID:      sessionID,
		ExcludeAgentID: excludeAgentID,
		Summary:        summary,
		ErrorMessage:   errorMessage,
	})
	return err
}

func completeOpenRunnerAgents(s *db.Store, runID int64, sessionID string, excludeAgentID int64, summary string) error {
	agents, err := s.AgentRuns(runID)
	if err != nil {
		return err
	}
	for _, agent := range agents {
		if agent.Status != "RUNNING" || agent.ID == excludeAgentID || agent.Role != "autonomous-runner" {
			continue
		}
		if sessionID != "" && agent.SessionID != sessionID {
			continue
		}
		if err := s.FinishAgentRun(agent.ID, "COMPLETED", summary, agent.ArtifactPath, ""); err != nil {
			return err
		}
		_, _ = s.InsertAgentEvent(db.AgentEvent{
			RunID:      runID,
			AgentRunID: agent.ID,
			SessionID:  agent.SessionID,
			Phase:      agent.Phase,
			Event:      "agent_completed",
			Detail:     summary,
		})
	}
	return nil
}

func recordRunnerAgentStart(s *db.Store, runID int64, sessionID, mode string) (int64, error) {
	id, err := s.StartAgentRun(db.AgentRun{
		RunID:        runID,
		SessionID:    sessionID,
		Role:         "autonomous-runner",
		AgentType:    "codedungeon-runner",
		AgentName:    "codedungeon run --" + mode,
		InputSummary: "autonomous child custody session",
	})
	if err != nil {
		return 0, err
	}
	_, _ = s.InsertAgentEvent(db.AgentEvent{
		RunID:      runID,
		AgentRunID: id,
		SessionID:  sessionID,
		Event:      "agent_started",
		Detail:     "autonomous-runner " + mode,
	})
	return id, nil
}

func recordRunnerAgentEnd(s *db.Store, runID, agentID int64, sessionID, status, summary string) error {
	if agentID == 0 {
		return nil
	}
	if err := s.FinishAgentRun(agentID, status, summary, "", summaryForError(status, summary)); err != nil {
		return err
	}
	_, _ = s.InsertAgentEvent(db.AgentEvent{
		RunID:      runID,
		AgentRunID: agentID,
		SessionID:  sessionID,
		Event:      "agent_" + strings.ToLower(status),
		Detail:     summary,
	})
	return nil
}

func summaryForError(status, summary string) string {
	if status == "FAILED" || status == "ABORTED" {
		return summary
	}
	return ""
}

var providerChildExecutor = executeProviderChild

func executeProviderChild(root, mode, prompt string, runID int64, sessionID, token string) error {
	p := provider.Detect()
	env := []string{
		envRunID + "=" + fmt.Sprintf("%d", runID),
		envSessionID + "=" + sessionID,
		envSessionToken + "=" + token,
	}
	return tooladapter.NewProviderRunner(nil).Run(context.Background(), tooladapter.ProviderRunRequest{
		Provider:          p.Name(),
		Root:              root,
		Model:             providerChildModel(root, mode, p),
		Prompt:            prompt,
		Env:               env,
		OutputLastMessage: filepath.Join(root, p.StateDir(), "runner-last-message.txt"),
		Stream:            true,
	})
}

func renderFinalReport(root string, runID int64, sessionID, token string) (string, error) {
	oldWD, wdErr := os.Getwd()
	if wdErr == nil {
		if err := os.Chdir(root); err != nil {
			return "", err
		}
		defer func() { _ = os.Chdir(oldWD) }()
	}
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
	if run == nil || run.ID != runID {
		return "", fmt.Errorf("active run mismatch while rendering final report")
	}
	oldRunID, oldSessionID, oldToken := os.Getenv(envRunID), os.Getenv(envSessionID), os.Getenv(envSessionToken)
	_ = os.Setenv(envRunID, fmt.Sprintf("%d", runID))
	_ = os.Setenv(envSessionID, sessionID)
	_ = os.Setenv(envSessionToken, token)
	defer func() {
		restoreEnv(envRunID, oldRunID)
		restoreEnv(envSessionID, oldSessionID)
		restoreEnv(envSessionToken, oldToken)
	}()
	if err := requireAutonomousCustody(s, run.ID, "report render"); err != nil {
		return "", err
	}
	return renderReport(s, run, false)
}

func restoreEnv(key, value string) {
	if value == "" {
		_ = os.Unsetenv(key)
		return
	}
	_ = os.Setenv(key, value)
}
