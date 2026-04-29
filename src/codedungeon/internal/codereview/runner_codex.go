package codereview

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type CodexRunner struct {
	WorkDir string
}

func (r CodexRunner) RunPersona(ctx context.Context, req Request, persona string, outPath string) error {
	return r.runCodex(ctx, personaPrompt(req, persona, outPath))
}

func (r CodexRunner) RunAdjudicator(ctx context.Context, req Request, personas []PersonaReview, outPath string) error {
	return r.runCodex(ctx, adjudicatorPrompt(req, personas, outPath))
}

func (r CodexRunner) runCodex(ctx context.Context, prompt string) error {
	workDir := r.WorkDir
	if strings.TrimSpace(workDir) == "" {
		workDir = "."
	}
	cmd := exec.CommandContext(ctx, "codex", "exec", "--cd", workDir, "--dangerously-bypass-approvals-and-sandbox", "--enable", "multi_agent_v2", "-")
	cmd.Stdin = strings.NewReader(prompt)
	var stderr bytes.Buffer
	cmd.Stdout = os.Stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("codex reviewer failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func personaPrompt(req Request, persona, outPath string) string {
	return fmt.Sprintf(`You are the standalone CodeDungeon code-review module.

Role/persona: %s

Objective: review the target using the project context and task context. Write ONLY strict JSON to:
%s

Hard rules:
- Do not return an empty/template review.
- If there is any blocking defect, set verdict to CHANGES_REQUESTED and include findings.
- If there are no findings, set verdict to APPROVED and provide a substantive approval_rationale plus at least two risks_considered.
- reviewed_files must be > 0 and reviewed_scope must name concrete files or diff sections.
- Include model/provider/session_id when available.
- Include this Project Rules envelope exactly:
  status=%s digest=%s read=%s

JSON schema:
{
  "persona": %q,
  "verdict": "APPROVED|CHANGES_REQUESTED",
  "model": "model-name",
  "provider": "codex",
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
      "evidence_quote": "substantive source/diff evidence",
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
		persona, req.ProjectRules.Status, req.ProjectRules.Digest, req.ProjectRules.Read,
		req.URL, req.TargetContext, req.ProjectContext, req.TaskContext)
}

func adjudicatorPrompt(req Request, personas []PersonaReview, outPath string) string {
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
  "provider": "codex",
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
`, filepath.Clean(outPath), req.URL, b.String())
}
