package taskexec

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	artifactreg "github.com/loldinis/codedungeon/internal/artifacts"
	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/taskplanning"
)

func Execute(ctx context.Context, req Request) (Result, error) {
	if err := normalizeRequest(&req); err != nil {
		return Result{}, err
	}
	task, err := readJSONFile[taskplanning.TaskSpec](req.TaskPath)
	if err != nil {
		return Result{}, err
	}
	if err := validateTask(task); err != nil {
		return Result{}, err
	}
	req.Config = EffectiveConfigForTask(req.Config, task)
	if shellGit, ok := req.Git.(ShellGit); ok {
		shellGit.Policy = SafetyPolicy{AllowedTools: req.Config.AllowedTools}
		req.Git = shellGit
	}
	store, err := db.Open(filepath.Join(req.Root, ".codedungeon", "codedungeon.db"))
	if err != nil {
		return Result{}, err
	}
	defer store.Close()
	if err := store.Init(); err != nil {
		return Result{}, err
	}
	if req.RunID == 0 {
		run, err := store.CurrentRun()
		if err != nil {
			return Result{}, err
		}
		if run != nil {
			req.RunID = run.ID
		}
	}

	sessionStore := NewSessionStore(store, filepath.Join(req.Root, ".codedungeon", "execute", "sessions"), req.Config.SessionTTLHours)
	session, err := resolveSession(sessionStore, req, task)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		OK:        true,
		SessionID: session.ID,
		Status:    StatusRunning,
		Task:      task,
		OutputDir: session.OutputDir,
		Metadata: map[string]any{
			"config": req.Config,
		},
	}
	if err := writeJSONFile(filepath.Join(session.OutputDir, "task.json"), task); err != nil {
		return failExecution(store, session.ID, result, err)
	}
	if err := registerExecutionSessionArtifacts(store, req.Root, req.RunID, *session, result.Status); err != nil {
		return failExecution(store, session.ID, result, err)
	}
	if req.DryRun {
		result.Status = StatusDryRun
		result.Artifacts = append(result.Artifacts, filepath.Join(session.OutputDir, "task.json"))
		result.Metadata["prompt"] = ExecutionPrompt(agentRequest(req, task, *session, 1, filepath.Join(session.OutputDir, "attempt-01", "execution-result.json")))
		if err := writeJSONFile(filepath.Join(session.OutputDir, "result.json"), result); err != nil {
			return failExecution(store, session.ID, result, err)
		}
		if err := registerExecutionResultArtifact(store, req.Root, req.RunID, *session, result.Status); err != nil {
			return failExecution(store, session.ID, result, err)
		}
		_ = store.UpdateExecutionSessionStatus(session.ID, StatusDryRun, "")
		_, _ = store.InsertExecutionTransition(db.ExecutionTransition{SessionID: session.ID, FromStatus: StatusRunning, ToStatus: StatusDryRun, Reason: "dry run"})
		return result, nil
	}

	policy := SafetyPolicy{AllowedTools: req.Config.AllowedTools}
	for attempt := session.Attempt + 1; attempt <= req.Config.MaxIterations; attempt++ {
		if err := store.UpdateExecutionSessionAttempt(session.ID, attempt); err != nil {
			return failExecution(store, session.ID, result, err)
		}
		attemptResult, err := runAttempt(ctx, req, task, *session, attempt, policy)
		result.Attempts = append(result.Attempts, attemptResult)
		result.ChangedFiles = attemptResult.ChangedFiles
		result.VerificationResults = attemptResult.VerificationResults
		if persistErr := persistAttempt(store, session.ID, attemptResult); persistErr != nil && err == nil {
			err = persistErr
		}
		if artifactErr := registerExecutionAttemptArtifacts(store, req.Root, req.RunID, *session, attemptResult); artifactErr != nil && err == nil {
			err = artifactErr
		}
		if err != nil {
			if attempt >= req.Config.MaxIterations {
				return failExecution(store, session.ID, result, err)
			}
			continue
		}
		if attemptResult.WorkerStatus == WorkerBlocked {
			result.Status = StatusBlocked
			result.FailureMessage = attemptResult.Summary
			_ = store.UpdateExecutionSessionStatus(session.ID, StatusBlocked, attemptResult.Summary)
			_, _ = store.InsertExecutionTransition(db.ExecutionTransition{SessionID: session.ID, FromStatus: StatusRunning, ToStatus: StatusBlocked, Reason: attemptResult.Summary})
			_ = writeJSONFile(filepath.Join(session.OutputDir, "result.json"), result)
			if err := registerExecutionResultArtifact(store, req.Root, req.RunID, *session, result.Status); err != nil {
				return failExecution(store, session.ID, result, err)
			}
			return result, nil
		}
		if attemptResult.WorkerStatus == WorkerPassed && attemptResult.VerificationStatus == "PASS" {
			if req.Config.AutoCommit {
				if err := req.Git.Commit(ctx, req.Root, commitMessage(task)); err != nil {
					return failExecution(store, session.ID, result, err)
				}
			}
			if req.Config.AutoTag {
				latest, err := req.Git.LatestSemverTag(ctx, req.Root)
				if err != nil {
					return failExecution(store, session.ID, result, err)
				}
				tag := NextPatchTag(latest)
				if err := req.Git.Tag(ctx, req.Root, tag, "CodeDungeon task execution "+task.ID); err != nil {
					return failExecution(store, session.ID, result, err)
				}
			}
			if req.Config.AutoPush {
				if err := req.Git.Push(ctx, req.Root); err != nil {
					return failExecution(store, session.ID, result, err)
				}
			}
			result.Status = StatusCompleted
			result.OK = true
			if err := writeJSONFile(filepath.Join(session.OutputDir, "result.json"), result); err != nil {
				return failExecution(store, session.ID, result, err)
			}
			if err := registerExecutionResultArtifact(store, req.Root, req.RunID, *session, result.Status); err != nil {
				return failExecution(store, session.ID, result, err)
			}
			_ = store.UpdateExecutionSessionStatus(session.ID, StatusCompleted, "")
			_, _ = store.InsertExecutionTransition(db.ExecutionTransition{SessionID: session.ID, FromStatus: StatusRunning, ToStatus: StatusCompleted, Reason: "verification passed"})
			return result, nil
		}
	}
	return failExecution(store, session.ID, result, fmt.Errorf("max execution attempts reached"))
}

func normalizeRequest(req *Request) error {
	req.Root = strings.TrimSpace(req.Root)
	if req.Root == "" {
		cwd, _ := os.Getwd()
		req.Root = cwd
	}
	req.TaskPath = strings.TrimSpace(req.TaskPath)
	if req.TaskPath == "" {
		return fmt.Errorf("task path is required")
	}
	if len(strings.Fields(req.ProjectContext)) < 8 {
		return fmt.Errorf("project_context is required and must be substantive")
	}
	if req.Config.Runner == "" {
		cfg, err := LoadConfig(req.Root)
		if err != nil {
			return err
		}
		req.Config = cfg
	} else {
		req.Config = normalizeConfig(req.Config)
	}
	if req.Runner == nil {
		switch req.Config.Runner {
		case "codex", "":
			req.Runner = CodexRunner{WorkDir: req.Root}
		default:
			return fmt.Errorf("unknown execution runner %q", req.Config.Runner)
		}
	}
	policy := SafetyPolicy{AllowedTools: req.Config.AllowedTools}
	if req.Git == nil {
		req.Git = ShellGit{Policy: policy}
	}
	if req.Verifier == nil {
		req.Verifier = ShellVerifier{}
	}
	return nil
}

func validateTask(task taskplanning.TaskSpec) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("task id is required")
	}
	if strings.TrimSpace(task.Repo) == "" {
		return fmt.Errorf("task repo is required")
	}
	if strings.TrimSpace(task.Objective) == "" {
		return fmt.Errorf("task objective is required")
	}
	if len(task.WriteScope) == 0 {
		return fmt.Errorf("task write_scope is required")
	}
	if len(task.AcceptanceCriteria) == 0 {
		return fmt.Errorf("task acceptance_criteria is required")
	}
	if len(task.VerificationCommands) == 0 {
		return fmt.Errorf("task verification_commands is required")
	}
	return nil
}

func resolveSession(sessions *SessionStore, req Request, task taskplanning.TaskSpec) (*db.ExecutionSession, error) {
	if req.ResumeID != "" {
		return sessions.Resume(req.ResumeID, req.ResetSession)
	}
	if req.SessionID != "" {
		return sessions.Resume(req.SessionID, req.ResetSession)
	}
	return sessions.Start(StartSessionRequest{
		RunID:     req.RunID,
		TaskID:    task.ID,
		TaskPath:  req.TaskPath,
		Provider:  req.Config.Runner,
		OutputDir: filepath.Join(req.Root, ".codedungeon", "execute", "sessions", "exec-"+shortHash(task.ID+time.Now().String())),
	})
}

func runAttempt(ctx context.Context, req Request, task taskplanning.TaskSpec, session db.ExecutionSession, attempt int, policy SafetyPolicy) (AttemptResult, error) {
	attemptDir := filepath.Join(session.OutputDir, fmt.Sprintf("attempt-%02d", attempt))
	resultPath := filepath.Join(attemptDir, "execution-result.json")
	if err := os.MkdirAll(attemptDir, 0o755); err != nil {
		return AttemptResult{}, err
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, time.Duration(req.Config.TimeoutSeconds)*time.Second)
	defer cancel()
	headBefore, err := req.Git.Head(timeoutCtx, req.Root)
	if err != nil {
		return AttemptResult{}, err
	}
	backupRef := fmt.Sprintf("codedungeon/backup/%s/attempt-%02d", session.ID, attempt)
	if err := req.Git.CreateBackupRef(timeoutCtx, req.Root, backupRef, headBefore); err != nil {
		return AttemptResult{}, err
	}
	agentReq := agentRequest(req, task, session, attempt, resultPath)
	agentReq.OutputDir = attemptDir
	worker, err := req.Runner.RunTask(timeoutCtx, agentReq)
	if err != nil {
		return AttemptResult{Attempt: attempt, HeadBefore: headBefore, BackupRef: backupRef, WorkerStatus: StatusFailed, ErrorMessage: err.Error()}, err
	}
	changed, _ := req.Git.ChangedFiles(timeoutCtx, req.Root)
	diff, _ := req.Git.Diff(timeoutCtx, req.Root)
	diffPath := filepath.Join(attemptDir, "diff.patch")
	_ = os.WriteFile(diffPath, []byte(diff), 0o644)
	headAfter, _ := req.Git.Head(timeoutCtx, req.Root)
	verificationStatus := "SKIPPED"
	var verifications []VerificationResult
	if strings.EqualFold(worker.Status, WorkerPassed) {
		verifications, err = req.Verifier.Verify(timeoutCtx, VerifyRequest{
			Root:     req.Root,
			Repo:     task.Repo,
			Phase:    "6",
			Commands: task.VerificationCommands,
			Policy:   policy,
		})
		verificationStatus = verificationStatusFromResults(verifications, err)
	}
	out := AttemptResult{
		Attempt:             attempt,
		ProviderSessionID:   worker.SessionID,
		HeadBefore:          headBefore,
		HeadAfter:           headAfter,
		BackupRef:           backupRef,
		DiffPath:            diffPath,
		ChangedFiles:        firstNonEmptySlice(worker.ChangedFiles, changed),
		WorkerStatus:        firstNonEmpty(worker.Status, WorkerPassed),
		VerificationStatus:  verificationStatus,
		Summary:             worker.Summary,
		VerificationResults: verifications,
	}
	if err != nil {
		out.ErrorMessage = err.Error()
	}
	return out, err
}

func agentRequest(req Request, task taskplanning.TaskSpec, session db.ExecutionSession, attempt int, resultPath string) AgentRequest {
	workspacePolicy := req.WorkspacePolicy
	if strings.TrimSpace(workspacePolicy) == "" {
		workspacePolicy = DefaultWorkspacePolicy(task)
	}
	return AgentRequest{
		SessionID:       session.ID,
		Attempt:         attempt,
		Root:            req.Root,
		OutputDir:       filepath.Dir(resultPath),
		ResultPath:      resultPath,
		ProjectContext:  req.ProjectContext,
		WorkspacePolicy: workspacePolicy,
		Task:            task,
		Config:          req.Config,
	}
}

func persistAttempt(store *db.Store, sessionID string, attempt AttemptResult) error {
	now := time.Now().Unix()
	_, err := store.InsertExecutionAttempt(db.ExecutionAttempt{
		SessionID:          sessionID,
		Attempt:            attempt.Attempt,
		ProviderSessionID:  attempt.ProviderSessionID,
		HeadBefore:         attempt.HeadBefore,
		HeadAfter:          attempt.HeadAfter,
		BackupRef:          attempt.BackupRef,
		DiffPath:           attempt.DiffPath,
		ChangedFiles:       attempt.ChangedFiles,
		WorkerStatus:       attempt.WorkerStatus,
		VerificationStatus: attempt.VerificationStatus,
		Summary:            attempt.Summary,
		ResultJSON:         "",
		ErrorMessage:       attempt.ErrorMessage,
		StartedAt:          now,
		FinishedAt:         now,
	})
	return err
}

func registerExecutionSessionArtifacts(store *db.Store, root string, runID int64, session db.ExecutionSession, status string) error {
	registry := artifactreg.NewRegistry(store, root)
	meta := map[string]any{"task_id": session.TaskID, "status": status}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"directory", "directory", session.OutputDir},
		{"task", "json", filepath.Join(session.OutputDir, "task.json")},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "execution", OwnerType: "execution_session", OwnerID: session.ID,
			Phase: "5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	return nil
}

func registerExecutionResultArtifact(store *db.Store, root string, runID int64, session db.ExecutionSession, status string) error {
	registry := artifactreg.NewRegistry(store, root)
	return artifactreg.RegisterIfExists(registry, artifactreg.Record{
		RunID: runID, Module: "execution", OwnerType: "execution_session", OwnerID: session.ID,
		Phase: "5", Role: "result", Kind: "json", Path: filepath.Join(session.OutputDir, "result.json"),
		Metadata: map[string]any{"task_id": session.TaskID, "status": status},
	})
}

func registerExecutionAttemptArtifacts(store *db.Store, root string, runID int64, session db.ExecutionSession, attempt AttemptResult) error {
	registry := artifactreg.NewRegistry(store, root)
	ownerID := fmt.Sprintf("%s:%02d", session.ID, attempt.Attempt)
	attemptDir := filepath.Join(session.OutputDir, fmt.Sprintf("attempt-%02d", attempt.Attempt))
	meta := map[string]any{
		"session_id":          session.ID,
		"attempt":             attempt.Attempt,
		"worker_status":       attempt.WorkerStatus,
		"verification_status": attempt.VerificationStatus,
	}
	for _, item := range []struct {
		role string
		kind string
		path string
	}{
		{"directory", "directory", attemptDir},
		{"diff", "patch", attempt.DiffPath},
		{"worker_result", "json", filepath.Join(attemptDir, "execution-result.json")},
	} {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "execution", OwnerType: "execution_attempt", OwnerID: ownerID,
			Phase: "5", Role: item.role, Kind: item.kind, Path: item.path, Metadata: meta,
		}); err != nil {
			return err
		}
	}
	for _, result := range attempt.VerificationResults {
		if err := artifactreg.RegisterIfExists(registry, artifactreg.Record{
			RunID: runID, Module: "execution", OwnerType: "execution_attempt", OwnerID: ownerID,
			Phase: "6", Role: "verification_log", Kind: "log", Path: result.LogPath,
			Metadata: map[string]any{"command": result.Command, "status": result.Status},
		}); err != nil {
			return err
		}
	}
	return nil
}

func failExecution(store *db.Store, sessionID string, result Result, err error) (Result, error) {
	result.OK = false
	result.Status = StatusFailed
	result.FailureMessage = err.Error()
	if result.OutputDir != "" {
		_ = writeJSONFile(filepath.Join(result.OutputDir, "result.json"), result)
	}
	_ = store.UpdateExecutionSessionStatus(sessionID, StatusFailed, err.Error())
	_, _ = store.InsertExecutionTransition(db.ExecutionTransition{SessionID: sessionID, FromStatus: StatusRunning, ToStatus: StatusFailed, Reason: err.Error()})
	return result, err
}

func verificationStatusFromResults(results []VerificationResult, err error) string {
	if err != nil {
		return "FAIL"
	}
	for _, result := range results {
		if result.Status != "PASS" {
			return "FAIL"
		}
	}
	return "PASS"
}

func commitMessage(task taskplanning.TaskSpec) string {
	title := strings.TrimSpace(task.Title)
	if title == "" {
		title = task.ID
	}
	return "feat: " + title
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptySlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			return value
		}
	}
	return nil
}

func shortHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}
