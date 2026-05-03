package recovery

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/loldinis/codedungeon/internal/db"
)

const DefaultRunStaleAfter = 24 * time.Hour

func InspectRunSession(store *db.Store, runID int64, now time.Time) (*RunRecoveryReport, error) {
	if now.IsZero() {
		now = time.Now()
	}
	sess, err := store.LatestRunSession(runID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return &RunRecoveryReport{Status: "no_session"}, nil
	}
	agents, err := store.AgentRuns(runID)
	if err != nil {
		return nil, err
	}
	events, err := store.RunEvents(runID)
	if err != nil {
		return nil, err
	}
	report := &RunRecoveryReport{
		Status:          sess.Status,
		EffectiveStatus: sess.Status,
		SessionID:       sess.ID,
		Session:         sess,
		Active:          sess.Status == "RUNNING",
	}
	if from := recoveredFromSessionID(events, sess.ID); from != "" && sess.Status == "READY_FOR_USER_REVIEW" {
		report.Recovered = true
		report.RecoveredFromSessionID = from
		report.RecoveredFailures = append(report.RecoveredFailures, from)
	}
	if sess.StartedAt > 0 {
		report.AgeSeconds = maxInt64(0, now.Unix()-sess.StartedAt)
	}
	for _, agent := range agents {
		if agent.Status == "RUNNING" && (sess.ID == "" || agent.SessionID == sess.ID) {
			report.OpenAgents++
		}
	}
	if report.Active && time.Duration(report.AgeSeconds)*time.Second > DefaultRunStaleAfter {
		report.Stale = true
		report.NextCommands = append(report.NextCommands, fmt.Sprintf("codedungeon run unlock --reason %q", "stale session "+sess.ID))
	}
	if report.Active {
		report.NextCommands = append(report.NextCommands, "codedungeon run status")
	}
	if isTerminalFailureStatus(sess.Status) {
		report.FailureKind = classifyRunFailureKind(sess.FailureMessage)
		report.BlockedReason = strings.TrimSpace(sess.FailureMessage)
		report.ResumeCommand = "codedungeon run finalize --dry-run"
		report.NextCommands = append(report.NextCommands, report.ResumeCommand)
		if report.FailureKind == "provider_rate_limit" {
			report.NextCommands = append(report.NextCommands, "codedungeon run finalize")
		}
	}
	return report, nil
}

func isTerminalFailureStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "FAILED", "ABORTED", "BLOCKED":
		return true
	default:
		return false
	}
}

func classifyRunFailureKind(message string) string {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "provider_rate_limit") || strings.Contains(lower, "rate_limit") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "out_of_credits") || strings.Contains(lower, "429"):
		return "provider_rate_limit"
	case strings.Contains(lower, "auth") || strings.Contains(lower, "api key") || strings.Contains(lower, "unauthorized"):
		return "provider_auth"
	case strings.Contains(lower, "context window") || strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context"):
		return "provider_context"
	case strings.Contains(lower, "review"):
		return "review_validation"
	case strings.Contains(lower, "verification") || strings.Contains(lower, "qa"):
		return "verification"
	case strings.Contains(lower, "git") || strings.Contains(lower, "branch"):
		return "git"
	case strings.TrimSpace(message) == "":
		return "unknown"
	default:
		return "provider_process"
	}
}

func recoveredFromSessionID(events []db.RunEvent, sessionID string) string {
	for i := len(events) - 1; i >= 0; i-- {
		event := events[i]
		if event.SessionID == sessionID && event.Event == "session_recovered" {
			return event.Detail
		}
	}
	return ""
}

func AbortActiveRunSession(store *db.Store, runID int64, reason string, opts AbortOptions) (*RunRecoveryReport, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return nil, fmt.Errorf("reason is required")
	}
	sess, err := store.ActiveRunSession(runID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return &RunRecoveryReport{Status: "no_active_session"}, nil
	}
	if err := store.UpdateRunSessionStatus(sess.ID, "ABORTED", reason); err != nil {
		return nil, err
	}
	_, _ = store.InsertRunEvent(db.RunEvent{RunID: runID, SessionID: sess.ID, Event: "session_aborted", Detail: reason})
	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		summary = "session aborted by recovery unlock"
	}
	aborted, err := AbortOpenAgentRuns(store, runID, AgentAbortOptions{
		SessionID:               sess.ID,
		ExcludeAgentID:          opts.ExcludeAgentID,
		IncludeAutonomousRunner: opts.IncludeAutonomousRunner,
		Summary:                 summary,
		ErrorMessage:            reason,
	})
	if err != nil {
		return nil, err
	}
	return &RunRecoveryReport{
		Status:        "ABORTED",
		SessionID:     sess.ID,
		Active:        false,
		AbortedAgents: aborted,
		NextCommands: []string{
			"codedungeon run status",
			"codedungeon observe",
		},
	}, nil
}

func AbortOpenAgentRuns(store *db.Store, runID int64, opts AgentAbortOptions) (int, error) {
	agents, err := store.AgentRuns(runID)
	if err != nil {
		return 0, err
	}
	summary := firstNonEmpty(opts.Summary, "stale agent closed by recovery")
	detail := firstNonEmpty(opts.ErrorMessage, summary)
	aborted := 0
	for _, agent := range agents {
		if agent.Status != "RUNNING" || agent.ID == opts.ExcludeAgentID {
			continue
		}
		if opts.SessionID != "" && agent.SessionID != opts.SessionID {
			continue
		}
		if agent.Role == "autonomous-runner" && !opts.IncludeAutonomousRunner {
			continue
		}
		if err := store.FinishAgentRun(agent.ID, "ABORTED", summary, agent.ArtifactPath, detail); err != nil {
			return aborted, err
		}
		_, _ = store.InsertAgentEvent(db.AgentEvent{
			RunID:      runID,
			AgentRunID: agent.ID,
			SessionID:  agent.SessionID,
			Phase:      agent.Phase,
			Event:      "agent_aborted",
			Detail:     detail,
		})
		aborted++
	}
	return aborted, nil
}

func InspectExecutionSession(store *db.Store, sessionID, root string, now time.Time) (*ExecutionRecoveryReport, error) {
	if now.IsZero() {
		now = time.Now()
	}
	sess, err := store.ExecutionSession(sessionID)
	if err != nil {
		return nil, err
	}
	if sess == nil {
		return nil, fmt.Errorf("execution session not found: %s", sessionID)
	}
	attempts, err := store.ExecutionAttempts(sessionID)
	if err != nil {
		return nil, err
	}
	report := &ExecutionRecoveryReport{
		SessionID:     sess.ID,
		Status:        sess.Status,
		Session:       sess,
		RollbackHints: rollbackHints(sess.ID, attempts),
		CleanupHints:  []CleanupHint{cleanupHintForSession(*sess, root)},
	}
	if sess.StartedAt > 0 {
		report.AgeSeconds = maxInt64(0, now.Unix()-sess.StartedAt)
	}
	if sess.ExpiresAt > 0 {
		report.ExpiresInSeconds = sess.ExpiresAt - now.Unix()
		report.Expired = now.Unix() > sess.ExpiresAt
	}
	report.Stale = isActiveExecutionStatus(sess.Status) && report.Expired
	if report.Stale {
		report.NextCommands = append(report.NextCommands,
			fmt.Sprintf("codedungeon execute task --task %s --resume %s --reset-session", shellArg(sess.TaskPath), shellArg(sess.ID)),
		)
	}
	if len(report.RollbackHints) > 0 {
		report.NextCommands = append(report.NextCommands,
			fmt.Sprintf("codedungeon execute rollback --session %s --to before --confirm", shellArg(sess.ID)),
		)
	}
	if len(report.CleanupHints) > 0 && report.CleanupHints[0].SafeToDelete {
		report.NextCommands = append(report.NextCommands, "codedungeon cleanup --sessions --dry-run")
	}
	return report, nil
}

func BuildRollbackPlan(store *db.Store, sessionID, to string) (*RollbackPlan, error) {
	if strings.TrimSpace(sessionID) == "" {
		return nil, fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(to) == "" {
		to = "before"
	}
	attempts, err := store.ExecutionAttempts(sessionID)
	if err != nil {
		return nil, err
	}
	if len(attempts) == 0 {
		return nil, fmt.Errorf("no execution attempts for session: %s", sessionID)
	}
	for _, hint := range rollbackHints(sessionID, attempts) {
		if hint.Name == to {
			plan := RollbackPlan(hint)
			return &plan, nil
		}
	}
	return nil, fmt.Errorf("rollback target %q not found for session: %s", to, sessionID)
}

func ExecutionCleanupCandidates(store *db.Store, runID int64, root string) ([]CleanupHint, error) {
	sessions, err := store.ExecutionSessions(runID)
	if err != nil {
		return nil, err
	}
	out := make([]CleanupHint, 0, len(sessions))
	for _, sess := range sessions {
		if strings.TrimSpace(sess.OutputDir) == "" {
			continue
		}
		out = append(out, cleanupHintForSession(sess, root))
	}
	return out, nil
}

func rollbackHints(sessionID string, attempts []db.ExecutionAttempt) []RollbackHint {
	var out []RollbackHint
	if len(attempts) == 0 {
		return out
	}
	if attempts[0].HeadBefore != "" {
		out = append(out, newRollbackHint("before", sessionID, 0, attempts[0].HeadBefore, attempts[0].BackupRef))
	}
	for _, attempt := range attempts {
		if attempt.HeadBefore == "" {
			continue
		}
		out = append(out, newRollbackHint(fmt.Sprintf("attempt-%d", attempt.Attempt), sessionID, attempt.Attempt, attempt.HeadBefore, attempt.BackupRef))
	}
	return out
}

func newRollbackHint(name, sessionID string, attempt int, target, backupRef string) RollbackHint {
	return RollbackHint{
		Name:            name,
		SessionID:       sessionID,
		Attempt:         attempt,
		Target:          target,
		BackupRef:       backupRef,
		Command:         "git reset --hard " + target,
		RequiresConfirm: true,
		Destructive:     true,
	}
}

func cleanupHintForSession(sess db.ExecutionSession, root string) CleanupHint {
	hint := CleanupHint{
		Kind:      "execution_session_output",
		SessionID: sess.ID,
		Status:    sess.Status,
		Path:      sess.OutputDir,
		Reason:    "not safe to delete",
	}
	if strings.TrimSpace(sess.OutputDir) == "" {
		hint.Reason = "session has no output directory"
		return hint
	}
	if !isTerminalExecutionStatus(sess.Status) {
		hint.Reason = "session is not terminal"
		return hint
	}
	if !pathInside(filepath.Join(root, ".codedungeon", "execute", "sessions"), sess.OutputDir) {
		hint.Reason = "output directory is outside .codedungeon/execute/sessions"
		return hint
	}
	hint.SafeToDelete = true
	hint.Reason = "terminal execution session output"
	hint.Command = "codedungeon cleanup --sessions --dry-run"
	return hint
}

func isActiveExecutionStatus(status string) bool {
	return strings.EqualFold(status, "RUNNING")
}

func isTerminalExecutionStatus(status string) bool {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "COMPLETED", "FAILED", "BLOCKED", "DRY_RUN", "ABORTED":
		return true
	default:
		return false
	}
}

func pathInside(base, target string) bool {
	base = strings.TrimSpace(base)
	target = strings.TrimSpace(target)
	if base == "" || target == "" {
		return false
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absBase, absTarget)
	if err != nil {
		return false
	}
	return rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func shellArg(v string) string {
	if strings.TrimSpace(v) == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t\"'") {
		return strconv.Quote(v)
	}
	return v
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
