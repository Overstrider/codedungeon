package cmd

import (
	"fmt"
	"strings"

	"github.com/loldinis/codedungeon/internal/db"
)

const (
	runSessionWaitingForAgent = "WAITING_FOR_AGENT"
	runStatusActionRequired   = "ACTION_REQUIRED"
	runStatusReadyToFinalize  = "READY_TO_FINALIZE"
	runStatusReadyUserReview  = "READY_FOR_USER_REVIEW"
	runStatusCompleted        = "COMPLETED"
)

type agentFirstRunContract struct {
	OK           bool                   `json:"ok"`
	AgentFirst   bool                   `json:"agent_first"`
	Status       string                 `json:"status"`
	RunID        int64                  `json:"run_id"`
	SessionID    string                 `json:"session_id"`
	Mode         string                 `json:"mode"`
	Branch       string                 `json:"branch,omitempty"`
	Resumed      bool                   `json:"resumed,omitempty"`
	DryRun       bool                   `json:"dry_run,omitempty"`
	CurrentStep  agentFirstStep         `json:"current_step"`
	NextAction   agentFirstAction       `json:"next_action"`
	Blockers     []agentFirstBlocker    `json:"blockers,omitempty"`
	Evidence     []agentFirstEvidence   `json:"evidence,omitempty"`
	Timeline     []db.RunEvent          `json:"timeline,omitempty"`
	ProjectRules projectRulesStatus     `json:"project_rules"`
	Recovery     any                    `json:"recovery,omitempty"`
	Run          *db.Run                `json:"run,omitempty"`
	Session      *db.RunSession         `json:"session,omitempty"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

type agentFirstStep struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Module      string `json:"module"`
	Description string `json:"description"`
	Phase       string `json:"phase,omitempty"`
}

type agentFirstAction struct {
	Type        string `json:"type"`
	Command     string `json:"command,omitempty"`
	Description string `json:"description"`
	Expected    string `json:"expected,omitempty"`
}

type agentFirstBlocker struct {
	ID          string `json:"id"`
	Gate        string `json:"gate"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Recoverable bool   `json:"recoverable"`
	Command     string `json:"command,omitempty"`
}

type agentFirstEvidence struct {
	Kind string `json:"kind"`
	Path string `json:"path,omitempty"`
	Note string `json:"note,omitempty"`
}

var agentFirstStepOrder = []agentFirstStep{
	{ID: "project_rules", Name: "Project Rules", Module: "rules", Description: "Approve or refresh the compact project rules before broad planning."},
	{ID: "planning", Name: "Planning", Module: "planning", Description: "Create and promote a task graph or plan contract.", Phase: "4"},
	{ID: "execution", Name: "Execution", Module: "execution", Description: "Execute promoted task contracts with bounded write scope.", Phase: "5"},
	{ID: "code_review", Name: "Code Review", Module: "code_review", Description: "Run standalone review and post PR review evidence.", Phase: "5.5"},
	{ID: "qa", Name: "QA Verification", Module: "qa", Description: "Run deterministic verification after review fixes and capture evidence.", Phase: "6"},
	{ID: "finalization", Name: "Finalization", Module: "run", Description: "Enforce hard gates and render READY_FOR_USER_REVIEW.", Phase: "7"},
}

func buildAgentFirstContract(root string, s *db.Store, run *db.Run, sess *db.RunSession, rules projectRulesStatus, resumed, dryRun bool, recovery any) (agentFirstRunContract, error) {
	if run == nil {
		return agentFirstRunContract{}, fmt.Errorf("run is required")
	}
	if sess == nil {
		return agentFirstRunContract{}, fmt.Errorf("run session is required")
	}
	events, err := s.RunEvents(run.ID)
	if err != nil {
		return agentFirstRunContract{}, err
	}
	blockers := agentFirstBlockers(root, run.Mode, rules)
	step := agentFirstCurrentStep(run.Mode, rules, events)
	status := runStatusActionRequired
	if terminalStatus, terminalStep, ok := agentFirstTerminalState(sess); ok {
		status = terminalStatus
		step = terminalStep
		blockers = nil
	} else if step.ID == "complete" {
		status = runStatusCompleted
	} else if step.ID == "finalization" {
		ready, blocker := agentFirstFinalizationReadiness(root, s, run, sess)
		if blocker != nil {
			blockers = append(blockers, *blocker)
		}
		if ready && len(blockersForGate(blockers, "finalization")) == 0 {
			status = runStatusReadyToFinalize
		}
	}
	return agentFirstRunContract{
		OK:           true,
		AgentFirst:   true,
		Status:       status,
		RunID:        run.ID,
		SessionID:    sess.ID,
		Mode:         strings.ToLower(run.Mode),
		Branch:       run.Branch,
		Resumed:      resumed,
		DryRun:       dryRun,
		CurrentStep:  step,
		NextAction:   agentFirstNextAction(run, step),
		Blockers:     blockers,
		Evidence:     agentFirstEvidenceForStep(step),
		Timeline:     events,
		ProjectRules: rules,
		Recovery:     recovery,
	}, nil
}

func agentFirstTerminalState(sess *db.RunSession) (string, agentFirstStep, bool) {
	if sess == nil {
		return "", agentFirstStep{}, false
	}
	switch strings.ToUpper(strings.TrimSpace(sess.Status)) {
	case runStatusReadyUserReview:
		return runStatusReadyUserReview, agentFirstStep{ID: "ready_for_user_review", Name: "Ready For User Review", Module: "run", Description: "Finalization completed and the PR is ready for user review.", Phase: "7"}, true
	case runStatusCompleted:
		return runStatusCompleted, agentFirstStep{ID: "complete", Name: "Complete", Module: "run", Description: "Workflow completed."}, true
	case "FAILED":
		return "FAILED", agentFirstStep{ID: "failed", Name: "Failed", Module: "run", Description: "Workflow failed and needs recovery."}, true
	case "ABORTED":
		return "ABORTED", agentFirstStep{ID: "aborted", Name: "Aborted", Module: "run", Description: "Workflow was aborted."}, true
	}
	return "", agentFirstStep{}, false
}

func agentFirstFinalizationReadiness(root string, s *db.Store, run *db.Run, sess *db.RunSession) (bool, *agentFirstBlocker) {
	sessionID := ""
	if sess != nil {
		sessionID = sess.ID
	}
	if _, err := prepareFinalization(root, s, run, sessionID, "", 0); err != nil {
		return false, &agentFirstBlocker{
			ID:          "finalization_preflight",
			Gate:        "finalization",
			Severity:    "hard",
			Message:     "Finalization preflight is not satisfied: " + err.Error(),
			Recoverable: true,
			Command:     "codedungeon run finalize --dry-run",
		}
	}
	return true, nil
}

func agentFirstCurrentStep(mode string, rules projectRulesStatus, events []db.RunEvent) agentFirstStep {
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	completed := agentFirstCompletedSteps(events)
	if normalizedMode == "rules" {
		if completed["project_rules"] {
			return agentFirstStep{ID: "complete", Name: "Complete", Module: "rules", Description: "Project Rules workflow completed."}
		}
		return agentFirstStepByID("project_rules")
	}
	if (normalizedMode == "full" || normalizedMode == "lite") && strings.ToLower(rules.Status) != "approved" {
		return agentFirstStepByID("project_rules")
	}
	for _, step := range agentFirstStepOrder {
		if step.ID == "project_rules" {
			continue
		}
		if !completed[step.ID] {
			return step
		}
	}
	return agentFirstStepByID("finalization")
}

func agentFirstCompletedSteps(events []db.RunEvent) map[string]bool {
	completed := map[string]bool{}
	for _, event := range events {
		if event.Event != "step_completed" {
			continue
		}
		step := normalizeAgentFirstStepID(strings.SplitN(event.Detail, ":", 2)[0])
		completed[step] = true
	}
	return completed
}

func normalizeAgentFirstStepID(step string) string {
	return strings.ToLower(strings.TrimSpace(step))
}

func agentFirstStepByID(id string) agentFirstStep {
	for _, step := range agentFirstStepOrder {
		if step.ID == id {
			return step
		}
	}
	return agentFirstStep{ID: id, Name: id, Module: id, Description: "Run the next CodeDungeon workflow step."}
}

func agentFirstNextAction(run *db.Run, step agentFirstStep) agentFirstAction {
	mode := strings.ToLower(run.Mode)
	prompt := shellQuote(run.Feature)
	switch step.ID {
	case "project_rules":
		return agentFirstAction{Type: "agent", Command: "codedungeon rules status", Description: "Refresh Project Rules with the provider-native `$codedungeon --rules` or `/codedungeon --rules` flow, then approve and compact them.", Expected: "PROJECT_RULES_STATUS approved"}
	case "planning":
		return agentFirstAction{Type: "command", Command: fmt.Sprintf("codedungeon plan run --prompt %s --mode %s --project-context .codedungeon/project-rules.compact.md --promote", prompt, fallback(mode, "full")), Description: "Create or refresh the task graph, then promote executable task contracts.", Expected: "planning session completed and promoted"}
	case "execution":
		return agentFirstAction{Type: "command", Command: "codedungeon execute run --tasks .codedungeon/tasks --project-context .codedungeon/project-rules.compact.md", Description: "Execute promoted tasks. The current agent owns edits; the CLI records task/session evidence.", Expected: "execution task results recorded"}
	case "qa":
		return agentFirstAction{Type: "command", Command: "codedungeon qa run --phase 6 --auto --fresh", Description: "Run deterministic verification and capture QA evidence.", Expected: "QA status PASS or structured blocker"}
	case "code_review":
		return agentFirstAction{Type: "command", Command: "codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/plan/PLAN.md --out .codedungeon/code-review --post", Description: "Run standalone CodeDungeon review and post review evidence.", Expected: "review verdict APPROVED"}
	case "finalization":
		return agentFirstAction{Type: "command", Command: "codedungeon run finalize", Description: "Enforce hard final gates and render the final report.", Expected: "READY_FOR_USER_REVIEW"}
	case "complete":
		return agentFirstAction{Type: "none", Command: "codedungeon run status", Description: "Workflow is complete; start the next workflow when ready.", Expected: runStatusCompleted}
	case "ready_for_user_review":
		return agentFirstAction{Type: "none", Command: "codedungeon run status", Description: "Final report is rendered and the PR is ready for user review.", Expected: runStatusReadyUserReview}
	case "failed":
		return agentFirstAction{Type: "command", Command: "codedungeon run finalize --dry-run", Description: "Inspect recovery blockers before retrying finalization.", Expected: "structured recovery blocker or ready plan"}
	case "aborted":
		return agentFirstAction{Type: "command", Command: "codedungeon run status", Description: "Inspect aborted workflow state before starting another run.", Expected: "terminal run status"}
	default:
		return agentFirstAction{Type: "inspect", Command: "codedungeon run status", Description: "Inspect workflow state.", Expected: "next action identified"}
	}
}

func agentFirstBlockers(root, mode string, rules projectRulesStatus) []agentFirstBlocker {
	var blockers []agentFirstBlocker
	normalizedMode := strings.ToLower(strings.TrimSpace(mode))
	if (normalizedMode == "full" || normalizedMode == "lite") && strings.ToLower(rules.Status) != "approved" {
		blockers = append(blockers, agentFirstBlocker{
			ID:          "project_rules",
			Gate:        "planning",
			Severity:    "soft",
			Message:     "Project Rules are " + fallback(rules.Status, "missing") + "; broad workflows should refresh and approve them before planning.",
			Recoverable: true,
			Command:     "codedungeon rules status",
		})
	}
	if blocker := githubPREnvironmentBlocker(root); blocker != nil {
		blockers = append(blockers, *blocker)
	}
	return blockers
}

func githubPREnvironmentBlocker(root string) *agentFirstBlocker {
	if out, errb, err := run(root, "git", "remote", "get-url", "origin"); err != nil || strings.TrimSpace(out) == "" {
		detail := strings.TrimSpace(errb)
		if detail == "" && err != nil {
			detail = err.Error()
		}
		return &agentFirstBlocker{
			ID:          "github_pr_environment",
			Gate:        "finalization",
			Severity:    "soft",
			Message:     "GitHub origin remote is required before finalization. " + strings.TrimSpace(detail),
			Recoverable: true,
			Command:     "git remote add origin <github-url>",
		}
	}
	if _, errb, err := run(root, "gh", "auth", "status"); err != nil {
		detail := strings.TrimSpace(errb)
		if detail == "" {
			detail = err.Error()
		}
		return &agentFirstBlocker{
			ID:          "github_pr_environment",
			Gate:        "finalization",
			Severity:    "soft",
			Message:     "GitHub CLI authentication is required before finalization. " + strings.TrimSpace(detail),
			Recoverable: true,
			Command:     "gh auth login",
		}
	}
	return nil
}

func blockersForGate(blockers []agentFirstBlocker, gate string) []agentFirstBlocker {
	var out []agentFirstBlocker
	for _, blocker := range blockers {
		if blocker.Gate == gate {
			out = append(out, blocker)
		}
	}
	return out
}

func agentFirstEvidenceForStep(step agentFirstStep) []agentFirstEvidence {
	switch step.ID {
	case "project_rules":
		return []agentFirstEvidence{{Kind: "project_rules", Path: ".codedungeon/project-rules.compact.md"}}
	case "planning":
		return []agentFirstEvidence{{Kind: "planning", Path: ".codedungeon/plan/PLAN.md"}, {Kind: "tasks", Path: ".codedungeon/tasks/"}}
	case "execution":
		return []agentFirstEvidence{{Kind: "execution", Path: ".codedungeon/execute/sessions/"}}
	case "qa":
		return []agentFirstEvidence{{Kind: "qa", Path: ".codedungeon/qa/sessions/"}}
	case "code_review":
		return []agentFirstEvidence{{Kind: "review", Path: ".codedungeon/code-review/"}}
	case "finalization":
		return []agentFirstEvidence{{Kind: "report", Path: ".codedungeon/reports/"}}
	case "complete":
		return []agentFirstEvidence{{Kind: "project_rules", Path: ".codedungeon/project-rules.compact.md"}}
	case "ready_for_user_review":
		return []agentFirstEvidence{{Kind: "report", Path: ".codedungeon/reports/"}}
	default:
		return nil
	}
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
