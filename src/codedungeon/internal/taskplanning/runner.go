package taskplanning

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	var stderr, stdout bytes.Buffer
	runner := r.Runner
	if runner == nil {
		runner = tooladapter.NewSystemRunner()
	}
	_, err := runner.Run(ctx, tooladapter.Command{
		Dir:  workDir,
		Name: "codex",
		Args: []string{"exec", "--cd", workDir, "--dangerously-bypass-approvals-and-sandbox", "--enable", "multi_agent_v2", "-"},
		Stdin: planningPrompt(req),
		// Capture stdout (still echo) so we can recover the agent JSON ourselves
		// and surface codex errors emitted on stdout.
		Stdout: io.MultiWriter(os.Stdout, &stdout),
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("codex planning agent %s failed: %w: %s", req.Role, err, strings.TrimSpace(stderr.String()))
	}

	// Don't trust that codex wrote OutputPath — extract the agent JSON from stdout
	// and write it ourselves; fail loudly (not silently) if there's nothing usable.
	if jsonFileLooksValid(req.OutputPath) {
		return nil
	}
	if agentJSON, perr := extractAgentJSONFromStream(strings.TrimSpace(stdout.String())); perr == nil && agentJSON != "" {
		return writeAgentOutput(req.OutputPath, agentJSON)
	}
	return fmt.Errorf("codex planning agent %s produced no usable output: no JSON in stdout and %s was not written", req.Role, req.OutputPath)
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
	// Capture stdout while still echoing it so the user sees streaming progress.
	var stdout bytes.Buffer
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
		Stdout: io.MultiWriter(os.Stdout, &stdout),
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("claude planning agent %s failed: %w: %s", req.Role, err, strings.TrimSpace(stderr.String()))
	}

	// Caminho feliz: o `claude` escreveu o OutputPath direto. Em ambientes
	// headless/nested isso frequentemente não acontece, então não dependemos disso —
	// extraímos o JSON do agente do stream-json capturado e gravamos nós mesmos.
	if jsonFileLooksValid(req.OutputPath) {
		return nil
	}

	captured := strings.TrimSpace(stdout.String())
	agentJSON, perr := extractAgentJSONFromStream(captured)
	if perr == nil && agentJSON != "" {
		if werr := writeAgentOutput(req.OutputPath, agentJSON); werr != nil {
			return werr
		}
		return nil
	}

	// Falha alto, não mudo: explica exatamente o que aconteceu (esp. nested).
	hint := ""
	if runningInsideClaude() {
		hint = " (detected nested Claude session via CLAUDECODE; nested planning agents may not emit output — run `codedungeon plan run` outside the Claude session, or use `--runner files` for deterministic E2E)"
	}
	return fmt.Errorf("claude planning agent %s produced no usable output: could not find the agent JSON in the stream-json and %s was not written%s", req.Role, req.OutputPath, hint)
}

// runningInsideClaude reports whether we're executing inside a Claude Code session,
// where spawning a nested `claude` CLI may not return the expected stream output.
// Mirrors the spirit of codexSandboxNetworkDisabled() for the Claude provider.
func runningInsideClaude() bool {
	for _, key := range []string{"CLAUDECODE", "CLAUDE_CODE", "CLAUDE_CODE_ENTRYPOINT"} {
		if strings.TrimSpace(os.Getenv(key)) != "" {
			return true
		}
	}
	return false
}

// jsonFileLooksValid reports whether path exists with non-empty, valid JSON content.
func jsonFileLooksValid(path string) bool {
	body, err := os.ReadFile(path)
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return false
	}
	return json.Valid(body)
}

// writeAgentOutput writes the extracted agent JSON to path (creating dirs).
func writeAgentOutput(path, agentJSON string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(agentJSON), 0o644)
}

// extractAgentJSONFromStream parses the `claude --output-format stream-json` NDJSON
// (one JSON event per line), reconstructs the assistant's final text, and isolates
// the JSON object the planning agent was asked to emit. Tolerant of format
// variations: it also accepts a final `type:"result"` event carrying the text, and
// falls back to scanning the whole capture for a balanced JSON object.
func extractAgentJSONFromStream(stream string) (string, error) {
	stream = strings.TrimSpace(stream)
	if stream == "" {
		return "", fmt.Errorf("empty stream")
	}

	var assistantText strings.Builder
	var resultText string

	for _, line := range strings.Split(stream, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue // not a JSON event line; ignore
		}
		switch ev["type"] {
		case "assistant":
			// {"type":"assistant","message":{"content":[{"type":"text","text":"..."}]}}
			if msg, ok := ev["message"].(map[string]any); ok {
				assistantText.WriteString(textFromContent(msg["content"]))
			}
		case "content_block_delta":
			// {"type":"content_block_delta","delta":{"type":"text_delta","text":"..."}}
			if delta, ok := ev["delta"].(map[string]any); ok {
				if t, ok := delta["text"].(string); ok {
					assistantText.WriteString(t)
				}
			}
		case "result":
			// {"type":"result","result":"..."} — final aggregated text on some versions.
			if t, ok := ev["result"].(string); ok {
				resultText = t
			}
		}
	}

	for _, candidate := range []string{assistantText.String(), resultText, stream} {
		if obj := firstBalancedJSONObject(candidate); obj != "" {
			return obj, nil
		}
	}
	return "", fmt.Errorf("no valid JSON object found in stream")
}

// textFromContent concatenates the "text" fields from a message content array.
func textFromContent(content any) string {
	arr, ok := content.([]any)
	if !ok {
		return ""
	}
	var b strings.Builder
	for _, item := range arr {
		if m, ok := item.(map[string]any); ok {
			if t, ok := m["text"].(string); ok {
				b.WriteString(t)
			}
		}
	}
	return b.String()
}

// firstBalancedJSONObject returns the first top-level balanced `{...}` substring of s
// that is valid JSON, or "" if none. Respects strings/escapes so braces inside
// string literals don't break the balance count.
func firstBalancedJSONObject(s string) string {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return ""
	}
	for start >= 0 {
		depth := 0
		inStr := false
		esc := false
		for i := start; i < len(s); i++ {
			c := s[i]
			if inStr {
				switch {
				case esc:
					esc = false
				case c == '\\':
					esc = true
				case c == '"':
					inStr = false
				}
				continue
			}
			switch c {
			case '"':
				inStr = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					candidate := s[start : i+1]
					if json.Valid([]byte(candidate)) {
						return candidate
					}
					break // try next '{'
				}
			}
		}
		next := strings.IndexByte(s[start+1:], '{')
		if next < 0 {
			return ""
		}
		start = start + 1 + next
	}
	return ""
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
