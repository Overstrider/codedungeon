---
name: codedungeon-test-loop
description: Run the Claude-native CodeDungeon test and QA loop.
---

# codedungeon-test-loop

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before executing tests or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope required in test handoffs and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Read `.codedungeon/commands/codedungeon-test-loop.md` for the full editable playbook. Use CodeDungeon QA sessions for deterministic verification and keep evidence under `.codedungeon/qa`.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. If a PR review is needed, run `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context "$TASK_DIR/PLAN.md" --out .codedungeon/code-review --post`.
Run `./.claude/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"` and finalize only through `./.claude/bin/codedungeon run finalize`.
</evidence_gates>

<output_contract>
Report PASS, FAIL, or BLOCKED with concrete commands, logs, artifacts, and Project Rules fields. READY_FOR_USER_REVIEW can only come from finalization.
</output_contract>
