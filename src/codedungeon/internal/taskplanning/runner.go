package taskplanning

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/loldinis/codedungeon/internal/clauderuntime"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

type Runner interface {
	RunPlanningAgent(ctx context.Context, req AgentRequest) error
}

type FilesRunner struct {
	InputDir string
}

func (r FilesRunner) RunPlanningAgent(_ context.Context, req AgentRequest) error {
	if strings.TrimSpace(r.InputDir) == "" {
		return fmt.Errorf("files runner input dir is required")
	}
	src := filepath.Join(r.InputDir, req.Role+".json")
	body, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read fixture for role %s: %w", req.Role, err)
	}
	if err := os.MkdirAll(filepath.Dir(req.OutputPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(req.OutputPath, body, 0o644)
}

type CodexRunner struct {
	WorkDir string
	Runner  tooladapter.CommandRunner
}

func (r CodexRunner) RunPlanningAgent(ctx context.Context, req AgentRequest) error {
	if codexSandboxNetworkDisabled() {
		return fmt.Errorf("codex-runner-requires-unsandboxed-exec: CODEX_SANDBOX_NETWORK_DISABLED=1 blocks nested Codex agents; run `codedungeon plan run --runner codex` outside the Codex sandbox or approve escalated execution; use `--runner files` for deterministic E2E")
	}
	workDir := strings.TrimSpace(r.WorkDir)
	if workDir == "" {
		workDir = "."
	}
	var stderr bytes.Buffer
	runner := r.Runner
	if runner == nil {
		runner = tooladapter.NewSystemRunner()
	}
	_, err := runner.Run(ctx, tooladapter.Command{
		Dir:    workDir,
		Name:   "codex",
		Args:   []string{"exec", "--cd", workDir, "--dangerously-bypass-approvals-and-sandbox", "--enable", "multi_agent_v2", "-"},
		Stdin:  planningPrompt(req),
		Stdout: os.Stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("codex planning agent %s failed: %w: %s", req.Role, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type ClaudeRunner struct {
	WorkDir string
	Model   string
	Runner  tooladapter.CommandRunner
}

func (r ClaudeRunner) RunPlanningAgent(ctx context.Context, req AgentRequest) error {
	workDir := strings.TrimSpace(r.WorkDir)
	if workDir == "" {
		workDir = "."
	}
	model := strings.TrimSpace(r.Model)
	if model == "" {
		model = "claude-sonnet-4-6"
	}
	var stderr bytes.Buffer
	runner := r.Runner
	if runner == nil {
		runner = tooladapter.NewSystemRunner()
	}
	_, err := runner.Run(ctx, tooladapter.Command{
		Dir:  workDir,
		Name: "claude",
		Args: []string{
			"--setting-sources", "project,local",
			"--strict-mcp-config",
			"-p", "Read the CodeDungeon planning prompt from stdin and execute it.",
			"--output-format", "stream-json",
			"--verbose",
			"--dangerously-skip-permissions",
			"--model", model,
		},
		Stdin:  planningPromptForProvider(req, "claude", model),
		Env:    clauderuntime.ModelEnv(model),
		Stdout: os.Stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("claude planning agent %s failed: %w: %s", req.Role, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func codexSandboxNetworkDisabled() bool {
	value := strings.TrimSpace(os.Getenv("CODEX_SANDBOX_NETWORK_DISABLED"))
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func planningPrompt(req AgentRequest) string {
	return planningPromptForProvider(req, "codex", "model-name")
}

func planningPromptForProvider(req AgentRequest, providerName, model string) string {
	providerName = strings.TrimSpace(providerName)
	if providerName == "" {
		providerName = "codex"
	}
	model = strings.TrimSpace(model)
	if model == "" {
		model = "model-name"
	}
	return fmt.Sprintf(`You are a CodeDungeon task-planning swarm agent.

Role: %s
Session: %s
Round: %d

Write ONLY strict JSON to:
%s

Required JSON shape:
{
  "role": %q,
  "agent_name": "short-name",
  "provider": %q,
  "model": %q,
  "session_id": "session-or-run-id",
  "confidence": 0.75,
  "summary": "concrete planning result",
  "verdict": "PASS|NEEDS_USER_INPUT|FAIL for planning_evaluator only",
  "score": 0.0,
  "questions": [{"question":"...", "impact":"...", "material": true}],
  "proposals": [{"title":"...", "summary":"...", "files":["..."], "owner_role":"..."}],
  "risks": [{"title":"...", "impact":"...", "mitigation":"...", "severity":"P1|P2"}],
  "claims": [{"kind":"decision|risk|constraint", "title":"...", "summary":"..."}],
  "project_rules": {"status": %q, "digest": %q, "read": %q},
  "task_graph": {
    "version": 1,
    "tasks": [
      {
        "id": "TASK-001",
        "repo": "repo-name",
        "kind": "dev|test|fix",
        "title": "small task",
        "objective": "one responsibility",
        "context": ["relevant context"],
        "write_scope": ["path/or/module"],
        "depends_on": [],
        "wave": 1,
        "parallel_group": "group-name",
        "owner_role": "backend|frontend|qa|docs",
        "acceptance_criteria": ["testable criterion"],
        "verification_commands": ["command"],
        "risk_notes": ["risk"]
      }
    ]
  }
}

Rules:
- Non-splitter roles should omit task_graph unless they have a concrete full graph.
- planning_evaluator must set verdict and questions. Set NEEDS_USER_INPUT only for material ambiguity.
- task_splitter must provide task_graph.
- Tasks must be simple enough for weaker worker agents and must declare dependencies and parallel waves.
- Do not edit source code.

Context packet:
%s
`, req.Role, req.SessionID, req.Round, filepath.Clean(req.OutputPath), req.Role, providerName, model,
		req.ProjectRules.Status, req.ProjectRules.Digest, req.ProjectRules.Read, req.ContextPacket)
}
