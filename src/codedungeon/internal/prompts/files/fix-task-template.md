# TASK-{{.Num}}: Fix: {{.Title}}

## Meta
lang: {{.Lang}}
depends: none
priority: high
estimated_complexity: low

## Context
Adversarial review (cycle {{.Cycle}}) flagged this as ACTIONABLE (not a design decision).
Severity: {{.Severity}}
Flagged by: {{.FlaggedBy}}
Evidence quote (verbatim from code):
> {{.EvidenceQuote}}

Classifier rationale: "{{.ClassifierRationale}}"{{if .ClassifierConfidence}} (confidence: {{.ClassifierConfidence}}){{end}}
Suggested fix: "{{.SuggestedFix}}"

If you believe this is actually a design decision and should NOT be fixed, document it in REVIEW.md § "Repo-Specific Known Patterns" or an ADR under docs/adrs/ with explicit reasoning. Re-running /code-review will then classify it as design_decision and stop blocking. Do NOT simply skip the fix without documenting.

## Detailed Requirements
- {{.SuggestedFix}}

## Files
- MODIFY: {{.File}} (lines {{.LineStart}}-{{.LineEnd}}) — {{.Title}}

## Done when
- The specific adversarial review finding is resolved.

## Review checklist
- [ ] Finding `{{.File}}:{{.LineStart}}-{{.LineEnd}}` is addressed
- [ ] No regressions introduced
- [ ] `/code-review` re-run returns APPROVED or the finding becomes a documented design decision
