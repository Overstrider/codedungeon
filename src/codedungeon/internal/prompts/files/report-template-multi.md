=== LOLDINIS DEV CYCLE COMPLETE ===

Feature: {{.Feature}}
Mode: {{.Mode}}

Plans:
  Architecture: .codedungeon/plan/arcplan.md
  Domain plans: {{.DomainPlans}}
  QA plans: {{.QAPlans}}

Dev Results:
{{- range .Repos }}
  {{.Name}} — {{.Verdict}} — PR #{{.PRNumber}}
{{- end }}

PR Reports:
{{- range .Repos }}
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        {{.Verdict}}
| Workflow      main-quest
| PR            #{{.PRNumber}} {{.PRURL}}
| Branch        {{.Branch}}
| Review        {{.Verdict}}
| Cycles        {{.ReviewCycles}}/9 | last mode: {{.ReviewMode}}
+------------------------------------------------+

Summary
{{.Name}}: {{.Summary}}

Review
- Adversarial comments: {{.AdvReviewCount}}
- Last review marker: {{$.ReviewMarker}}
- Remaining findings: {{.RemainingFindings}}

Work Done
- Tasks: {{.TasksCompleted}}
- Changed files: {{.ChangedFiles}}
- Verification: {{.Verification}}

PR
{{.PRURL}}

Next
{{.NextAction}}
{{- end }}

Test Results:
{{- range .Repos }}
  {{.Name}}:
    Integration: {{.IntegrationResult}}
    API: {{.APIResult}}
    E2E: {{.E2EResult}}
{{- end }}

Code bugs found by tests: {{.TestBugsFound}} (all auto-fixed via dev loop re-entry)

Pipeline phases:
  Phase 0: Validation + codebase mapping + test auth check
  Phase 1: dragon-architect-planner → arcplan.md
  Phase 2: domain planners (parallel) → {{.DomainPlanCount}} domain plans
  Phase 3: lang-specialists (parallel) → plans enriched
  Phase 3.5: basilisk-planner-qa (parallel) → QA plans + Definition of Done
  Phase 4: spider-architect-task → MASTER.md + dev tasks + test tasks
  Phase 5: codedungeon-loop per repo → code + PRs + /code-review (full cycles 1-3, reduced cycles 4-9)
  Phase 6: codedungeon-test-loop per repo → integration + API + E2E tests
  Phase 7: Final report (this)

Next steps:
  1. Review the PRs
  2. Merge in order: {{.ExecutionOrder}}
  3. Deploy
