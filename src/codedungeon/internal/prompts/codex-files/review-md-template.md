## Codex Adversarial Code Review

**Verdict**: {{.Verdict}}
**Personas**: {{.PersonasStr}}
**Validator**: {{.ValidatorModel}}
**Classifier**: {{.ClassifierModel}}
{{- if .StackSpecialist }}
**Stack rubric**: {{.StackSpecialist}}
{{- end }}

**Gate**: APPROVED requires zero actionable findings across ALL severities (P0 + P1 + P2).
Findings documented as design decisions in project rules, REVIEW.md, ADRs, or the spec do not block.

---

### Actionable - must fix before merge
{{- if not .Actionable }}

_None._
{{- end }}
{{- range .Actionable }}

**[{{.Severity}}] {{.Title}}** - `{{.File}}:{{.LineStart}}-{{.LineEnd}}` - flagged by: {{.FlaggedByStr}}
> {{.EvidenceQuote}}

{{.WhyItMattersOrScenario}}

**Suggested fix**: {{.SuggestedFix}}
{{- end }}

---

### Accepted design decisions (disclosed, not blocking)
{{- if not .DesignDecisions }}

_None._
{{- end }}
{{- range .DesignDecisions }}

**[{{.Severity}}] {{.Title}}** - `{{.File}}:{{.LineStart}}`
> {{.EvidenceQuote}}

Classified as design decision because:
> {{.ClassifierEvidenceQuote}} _(source: {{.ClassifierEvidenceSource}})_

Rationale: {{.ClassifierRationale}}
{{- end }}

---

### Summary

- Actionable: {{.Tally.Actionable.P0}} P0 / {{.Tally.Actionable.P1}} P1 / {{.Tally.Actionable.P2}} P2
- Accepted design decisions: {{.Tally.DesignDecisions}}
- Dropped by validator: {{.Tally.Dropped}}
{{- if .Tally.SuppressedNits }}
- Suppressed nits: {{.Tally.SuppressedNits}}
{{- end }}

<!-- bughunter-severity: {{.TallyJSON}} -->
