=== CODEDUNGEON READY_FOR_USER_REVIEW (BOOTSTRAP) ===

Project: {{.Feature}}
Mode: BOOTSTRAP (created from scratch)
PROJECT_RULES_STATUS: {{.ProjectRulesStatus}}
PROJECT_RULES_DIGEST: {{.ProjectRulesDigest}}
PROJECT_RULES_READ: {{.ProjectRulesRead}}
Stack: {{.Stack}} ({{.Lang}})

Plans:
  Architecture: .codedungeon/plan/arcplan.md
  Domain plans: {{.DomainPlans}}
  QA plans: {{.QAPlans}}

Dev Results:
  . - {{.Status}} - PR #{{.PRNumber}}

PR Report:
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        READY_FOR_USER_REVIEW
| Workflow      main-quest
| PR            #{{.PRNumber}} {{.PRURL}}
| Branch        {{.Branch}}
| Review        {{.Status}}
| Cycles        {{.ReviewCycles}}/9 | last mode: {{.ReviewMode}}
+------------------------------------------------+

Summary
{{.Feature}}

Review
- Adversarial comments: {{.AdvReviewCount}}
- Last review marker: {{.ReviewMarker}}
- Remaining findings: {{.RemainingFindings}}

Work Done
- Tasks: {{.DevTasks}} dev, {{.TestTasks}} test
- Changed files: {{.ChangedFiles}}
- Verification: {{.TestResult}}
- Telemetry: {{.AgentTelemetry}}

PR
{{.PRURL}}

Next
{{.NextAction}}

Test Results:
  .: {{.TestResult}}

Pipeline phases:
  Phase 0: Bootstrap detection -> stack selection -> git init
  Phase 1: architect planner -> project architecture from scratch
  Phase 2: domain planner -> creation plan
  Phase 3.5: QA planner -> QA test strategy
  Phase 4: task architect -> MASTER.md + dev tasks + test tasks
  Phase 5: codedungeon-loop -> project scaffolded + PR created
  Phase 6: codedungeon-test-loop -> tests executed
  Phase 7: Final report

Next steps:
  1. Review the PR
  2. Merge to main
  3. Run cartographer to generate CODEBASE_MAP.md for future iterations
  4. Continue development with $codedungeon --full
