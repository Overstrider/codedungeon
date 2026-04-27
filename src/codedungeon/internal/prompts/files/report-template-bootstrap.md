=== MAIN QUEST COMPLETE (BOOTSTRAP) ===

Project: {{.Feature}}
Mode: BOOTSTRAP (created from scratch)
Stack: {{.Stack}} ({{.Lang}})

Plans:
  Architecture: .codedungeon/plan/arcplan.md
  Domain plans: {{.DomainPlans}}
  QA plans: {{.QAPlans}}

Dev Results:
  . — {{.Status}} — PR #{{.PRNumber}}

PR Report:
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        {{.Status}}
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

PR
{{.PRURL}}

Next
{{.NextAction}}

Test Results:
  .: {{.TestResult}}

Pipeline phases:
  Phase 0: Bootstrap detection → stack selection → git init
  Phase 1: dragon-architect-planner → project architecture from scratch
  Phase 2: domain planner → creation plan
  Phase 3: {{.Lang}}-specialist → enriched with idiomatic patterns
  Phase 3.5: basilisk-planner-qa → QA test strategy
  Phase 4: spider-architect-task → MASTER.md + {{.DevTasks}} dev tasks + {{.TestTasks}} test tasks
  Phase 5: codedungeon-loop → project scaffolded + PR created
  Phase 6: codedungeon-test-loop → tests executed
  Phase 7: Final report (this)

Next steps:
  1. Review the PR (it contains the entire project scaffold)
  2. Merge to main
  3. Run cartographer to generate CODEBASE_MAP.md for future iterations
  4. Continue development with /main-quest
