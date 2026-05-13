---
name: codedungeon-loop
description: Execute promoted CodeDungeon task files through the Claude-native implementation loop.
---

# codedungeon-loop

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before executing tasks, reviewing, or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope required in tasks, reviews, handoffs, and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Read `.codedungeon/commands/codedungeon-loop.md` for the full editable playbook. Execute tasks from `PLAN.md`, create or reuse the PR, run review cycles, and record execution plus review evidence.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. Run `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context "$TASK_DIR/PLAN.md" --out .codedungeon/code-review --post`.
Run concrete verification before reporting READY_FOR_USER_REVIEW and finalize with `./.claude/bin/codedungeon run finalize`.
</evidence_gates>

<output_contract>
Return the CodeDungeon PR Report shape. Include review verdict, cycle count, verification result, and Project Rules fields.
</output_contract>
