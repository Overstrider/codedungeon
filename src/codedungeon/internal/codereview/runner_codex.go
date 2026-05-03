package codereview

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/loldinis/codedungeon/internal/clauderuntime"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

type CodexRunner struct {
	WorkDir string
	Runner  tooladapter.CommandRunner
}

func (r CodexRunner) RunPersona(ctx context.Context, req Request, persona string, outPath string) error {
	return r.runCodex(ctx, personaPrompt(req, persona, outPath, "codex"))
}

func (r CodexRunner) RunAdjudicator(ctx context.Context, req Request, personas []PersonaReview, outPath string) error {
	return r.runCodex(ctx, adjudicatorPrompt(req, personas, outPath, "codex"))
}

func (r CodexRunner) runCodex(ctx context.Context, prompt string) error {
	workDir := r.WorkDir
	if strings.TrimSpace(workDir) == "" {
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
		Stdin:  prompt,
		Stdout: os.Stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return fmt.Errorf("codex reviewer failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

type ClaudeRunner struct {
	WorkDir                 string
	Model                   string
	Runner                  tooladapter.CommandRunner
	Timeout                 time.Duration
	CompletionCheckInterval time.Duration
}

func (r ClaudeRunner) RunPersona(ctx context.Context, req Request, persona string, outPath string) error {
	return r.runClaude(ctx, personaPrompt(req, persona, outPath, "claude"), personaCompletionCheck(outPath, persona, req.ProjectRules))
}

func (r ClaudeRunner) RunAdjudicator(ctx context.Context, req Request, personas []PersonaReview, outPath string) error {
	return r.runClaude(ctx, adjudicatorPrompt(req, personas, outPath, "claude"), decisionCompletionCheck(outPath, personas))
}

func (r ClaudeRunner) runClaude(ctx context.Context, prompt string, completionCheck func() (bool, error)) error {
	workDir := r.WorkDir
	if strings.TrimSpace(workDir) == "" {
		workDir = "."
	}
	var stdout boundedBuffer
	stdout.limit = 64 * 1024
	var stderr boundedBuffer
	stderr.limit = 64 * 1024
	runner := r.Runner
	if runner == nil {
		runner = tooladapter.NewSystemRunner()
	}
	result, err := runner.Run(ctx, tooladapter.Command{
		Dir:                     workDir,
		Name:                    "claude",
		Args:                    claudeReviewArgs(r.Model),
		Stdin:                   prompt,
		Env:                     clauderuntime.ModelEnv(r.Model),
		Timeout:                 r.timeout(),
		CompletionCheck:         completionCheck,
		CompletionCheckInterval: r.completionCheckInterval(),
		Stdout:                  io.MultiWriter(os.Stdout, &stdout),
		Stderr:                  &stderr,
	})
	if err != nil {
		stdoutText := strings.TrimSpace(firstNonEmptyString(stdout.String(), result.Stdout))
		stderrText := strings.TrimSpace(firstNonEmptyString(stderr.String(), result.Stderr))
		if providerErr := classifyClaudeProviderFailure(r.Model, stdoutText, stderrText, err); providerErr.Kind != "" {
			return providerErr
		}
		return fmt.Errorf("claude reviewer failed: %w: %s", err, strings.TrimSpace(stderrText))
	}
	return nil
}

type boundedBuffer struct {
	buf   bytes.Buffer
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.limit <= 0 || b.buf.Len() < b.limit {
		remaining := b.limit - b.buf.Len()
		if b.limit <= 0 || remaining > len(p) {
			remaining = len(p)
		}
		if remaining > 0 {
			_, _ = b.buf.Write(p[:remaining])
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string { return b.buf.String() }

func classifyClaudeProviderFailure(model, stdoutText, stderrText string, err error) ProviderFailureError {
	combined := strings.TrimSpace(stdoutText + "\n" + stderrText)
	message := extractClaudeErrorMessage(combined)
	if message == "" {
		message = combined
	}
	lower := strings.ToLower(message + "\n" + combined)
	kind := FailureProviderProcess
	switch {
	case strings.Contains(lower, "rate_limit") || strings.Contains(lower, "rate limit") || strings.Contains(lower, "out_of_credits") || strings.Contains(lower, "429"):
		kind = FailureProviderRateLimit
	case strings.Contains(lower, "auth") || strings.Contains(lower, "api key") || strings.Contains(lower, "unauthorized"):
		kind = FailureProviderAuth
	case strings.Contains(lower, "context window") || strings.Contains(lower, "context length") || strings.Contains(lower, "maximum context"):
		kind = FailureProviderContext
	}
	providerErr := providerFailure(kind, "claude", model, message, err)
	providerErr.RetryAfter = extractRetryAfter(message + "\n" + combined)
	return providerErr
}

func extractClaudeErrorMessage(output string) string {
	var messages []string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "{") {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			continue
		}
		if msg := stringFromAny(payload["message"]); msg != "" {
			messages = append(messages, msg)
		}
		if errValue, ok := payload["error"]; ok {
			switch v := errValue.(type) {
			case string:
				if strings.TrimSpace(v) != "" {
					messages = append(messages, strings.TrimSpace(v))
				}
			case map[string]any:
				if typ := stringFromAny(v["type"]); typ != "" {
					messages = append(messages, typ)
				}
				if msg := stringFromAny(v["message"]); msg != "" {
					messages = append(messages, msg)
				}
			}
		}
	}
	return strings.Join(messages, " ")
}

func stringFromAny(value any) string {
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

var retryAfterRE = regexp.MustCompile(`(?i)(?:reset(?:s| at)?|retry(?: after)?)[^0-9A-Z]*(\d{4}-\d{2}-\d{2}T[0-9:.]+Z|[0-9]+s?)`)

func extractRetryAfter(text string) string {
	match := retryAfterRE.FindStringSubmatch(text)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func claudeReviewArgs(model string) []string {
	args := []string{
		"--setting-sources", "project,local",
		"--strict-mcp-config",
		"--no-session-persistence",
		"-p", "Read the CodeDungeon code-review prompt from stdin and write the requested JSON files.",
		"--output-format", "stream-json",
		"--verbose",
		"--dangerously-skip-permissions",
	}
	if model := strings.TrimSpace(model); model != "" {
		args = append(args, "--model", model)
	}
	return args
}

func (r ClaudeRunner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}
	if seconds, ok := positiveEnvInt("CODEDUNGEON_CLAUDE_REVIEW_TIMEOUT_SECONDS"); ok {
		return time.Duration(seconds) * time.Second
	}
	return 45 * time.Minute
}

func (r ClaudeRunner) completionCheckInterval() time.Duration {
	if r.CompletionCheckInterval > 0 {
		return r.CompletionCheckInterval
	}
	if millis, ok := positiveEnvInt("CODEDUNGEON_CLAUDE_REVIEW_ARTIFACT_POLL_MS"); ok {
		return time.Duration(millis) * time.Millisecond
	}
	return 2 * time.Second
}

func positiveEnvInt(name string) (int, bool) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, false
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func personaCompletionCheck(path, persona string, rules ProjectRulesEnvelope) func() (bool, error) {
	started := time.Now()
	return func() (bool, error) {
		if !artifactUpdatedSince(path, started) {
			return false, nil
		}
		review, err := readPersonaReview(path)
		if err != nil {
			return false, nil
		}
		if strings.TrimSpace(review.Persona) != strings.TrimSpace(persona) {
			return false, nil
		}
		if err := ValidatePersonaReview(review, rules); err != nil {
			return false, nil
		}
		return true, nil
	}
}

func decisionCompletionCheck(path string, personas []PersonaReview) func() (bool, error) {
	started := time.Now()
	findings := collectFindings(personas)
	return func() (bool, error) {
		if !artifactUpdatedSince(path, started) {
			return false, nil
		}
		decision, err := readDecision(path)
		if err != nil {
			return false, nil
		}
		if err := ValidateDecision(decision, personas, findings); err != nil {
			return false, nil
		}
		return true, nil
	}
}

func artifactUpdatedSince(path string, started time.Time) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return !info.ModTime().Before(started)
}

func personaPrompt(req Request, persona, outPath, runnerProvider string) string {
	return fmt.Sprintf(`You are the standalone CodeDungeon code-review module.

Role/persona: %s

Objective: review the target using the project context and task context. Write ONLY strict JSON to:
%s

Hard rules:
- Do not return an empty/template review.
- The findings array is strictly for actionable/blocking defects.
- If there is any actionable/blocking defect, set verdict to CHANGES_REQUESTED and include findings.
- If there are no actionable/blocking defects, set verdict to APPROVED and provide a substantive approval_rationale plus at least two risks_considered.
- APPROVED requires "findings": [] exactly.
- Do not put non-blocking observations, defense-in-depth notes, or informational risks in findings; put them in risks_considered when approving.
- reviewed_files must be > 0 and reviewed_scope must name concrete files or diff sections.
- Include model/provider/session_id when available.
- Include this Project Rules envelope exactly:
  status=%s digest=%s read=%s

JSON schema:
{
  "persona": %q,
  "verdict": "APPROVED|CHANGES_REQUESTED",
  "model": "model-name",
  "provider": %q,
  "session_id": "session-or-run-id",
  "reviewed_files": 1,
  "reviewed_scope": ["path or diff section"],
  "approval_rationale": "required when findings is empty; detailed, concrete, not generic",
  "risks_considered": ["risk one", "risk two"],
  "verification_checked": ["command or evidence checked"],
  "project_rules": {"status": %q, "digest": %q, "read": %q},
  "findings": [
    {
      "severity": "P0|P1|P2",
      "file": "path",
      "line_start": 1,
      "line_end": 1,
      "title": "defect",
      "evidence_quote": "exact source/diff excerpt; short verbatim quotes are allowed when the cited line is short",
      "suggested_fix": "minimal fix direction"
    }
  ]
}

Target URL:
%s

Target context:
%s

Project context:
%s

Task context:
%s
`, persona, filepath.Clean(outPath), req.ProjectRules.Status, req.ProjectRules.Digest, req.ProjectRules.Read,
		persona, runnerProvider, req.ProjectRules.Status, req.ProjectRules.Digest, req.ProjectRules.Read,
		req.URL, req.TargetContext, req.ProjectContext, req.TaskContext)
}

func adjudicatorPrompt(req Request, personas []PersonaReview, outPath, runnerProvider string) string {
	var b strings.Builder
	for _, persona := range personas {
		fmt.Fprintf(&b, "- %s: %s, findings=%d, rationale=%s\n", persona.Persona, persona.Verdict, len(persona.Findings), persona.ApprovalRationale)
	}
	return fmt.Sprintf(`You are the standalone CodeDungeon final code-review adjudicator.

Objective: read all persona outcomes and write ONLY strict JSON to:
%s

Hard rules:
- You are the only component allowed to declare the final APPROVED verdict.
- Do not approve if any persona is CHANGES_REQUESTED.
- Do not approve if any finding remains.
- When verdict is CHANGES_REQUESTED, describe no-finding personas as "reported no blocking findings"; do not describe them as approvals.
- If approving, approval_rationale must be substantive and explain why the review is complete.

JSON schema:
{
  "verdict": "APPROVED|CHANGES_REQUESTED",
  "decided_by": "code-review-adjudicator",
  "model": "model-name",
  "provider": %q,
  "approval_rationale": "substantive final decision rationale",
  "persona_verdicts": {
    "saboteur": "APPROVED|CHANGES_REQUESTED",
    "newhire": "APPROVED|CHANGES_REQUESTED",
    "security": "APPROVED|CHANGES_REQUESTED",
    "spec": "APPROVED|CHANGES_REQUESTED",
    "tests": "APPROVED|CHANGES_REQUESTED"
  }
}

Target URL:
%s

Persona outcomes:
%s
`, filepath.Clean(outPath), runnerProvider, req.URL, b.String())
}
