package taskexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loldinis/codedungeon/internal/tooladapter"
)

type FilesRunner struct {
	InputDir string
}

func (r FilesRunner) RunTask(_ context.Context, req AgentRequest) (AgentResult, error) {
	if strings.TrimSpace(r.InputDir) == "" {
		return AgentResult{}, fmt.Errorf("files runner input dir is required")
	}
	src := filepath.Join(r.InputDir, "execution-result.json")
	result, err := readJSONFile[AgentResult](src)
	if err != nil {
		return AgentResult{}, err
	}
	if result.Status == "" {
		result.Status = WorkerPassed
	}
	return result, nil
}

type CodexRunner struct {
	WorkDir string
	Runner  tooladapter.CommandRunner
}

func (r CodexRunner) RunTask(ctx context.Context, req AgentRequest) (AgentResult, error) {
	workDir := strings.TrimSpace(r.WorkDir)
	if workDir == "" {
		workDir = req.Root
	}
	if workDir == "" {
		workDir = "."
	}
	if err := os.MkdirAll(req.OutputDir, 0o755); err != nil {
		return AgentResult{}, err
	}
	lastMessage := filepath.Join(req.OutputDir, fmt.Sprintf("attempt-%02d-last-message.txt", req.Attempt))
	var stderr bytes.Buffer
	runner := r.Runner
	if runner == nil {
		runner = tooladapter.NewSystemRunner()
	}
	_, err := runner.Run(ctx, tooladapter.Command{
		Dir:    workDir,
		Name:   "codex",
		Args:   []string{"exec", "--cd", workDir, "--sandbox", "workspace-write", "--enable", "multi_agent_v2", "--output-last-message", lastMessage, "-"},
		Stdin:  ExecutionPrompt(req),
		Stdout: os.Stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return AgentResult{}, fmt.Errorf("codex task worker failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	result, err := readJSONFile[AgentResult](req.ResultPath)
	if err == nil {
		if result.Status == "" {
			result.Status = WorkerPassed
		}
		return result, nil
	}
	body, _ := os.ReadFile(lastMessage)
	return AgentResult{Status: WorkerPassed, Summary: strings.TrimSpace(string(body))}, nil
}

func ExecutionPrompt(req AgentRequest) string {
	return fmt.Sprintf(`You are CodeDungeon Implementation Executor worker.

Task: %s - %s
Attempt: %d
Session: %s

Hard rules:
- Execute exactly one task. Do not broaden scope.
- Search before assuming missing code.
- No placeholders or stubs.
- If unrelated tests fail, fix them or report BLOCKED with exact failure.
- Document test intent near new tests when helpful.
- Do not run destructive commands: rm -rf, git reset --hard, git clean, Remove-Item -Recurse -Force.
- Stay inside write scope.
- Write strict JSON result to: %s

Required JSON:
{
  "status": "PASS|CHANGES_REQUESTED|BLOCKED",
  "summary": "what changed",
  "session_id": "provider session id if available",
  "changed_files": ["path"],
  "risks": ["remaining risk"]
}

Project context:
%s

Workspace policy:
%s

Task contract:
Repo: %s
Kind: %s
Objective: %s
Context: %s
Write scope: %s
Acceptance criteria: %s
Verification commands: %s
Risk notes: %s
`, req.Task.ID, req.Task.Title, req.Attempt, req.SessionID, filepath.Clean(req.ResultPath),
		req.ProjectContext, req.WorkspacePolicy, req.Task.Repo, req.Task.Kind, req.Task.Objective,
		strings.Join(req.Task.Context, "\n- "), strings.Join(req.Task.WriteScope, "\n- "),
		strings.Join(req.Task.AcceptanceCriteria, "\n- "), strings.Join(req.Task.VerificationCommands, "\n- "),
		strings.Join(req.Task.RiskNotes, "\n- "))
}
