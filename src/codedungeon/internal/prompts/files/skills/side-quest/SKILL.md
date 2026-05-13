---
name: side-quest
description: Execute a reviewed plan through the lightweight Claude-native CodeDungeon workflow.
---

# side-quest

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before planning, executing, reviewing, or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope required in plans, tasks, reviews, handoffs, and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Start or resume durable state with `./.claude/bin/codedungeon run --lite --prompt "<prompt>"`. Decompose the selected `.codedungeon/plans/*.md` plan into `.codedungeon/tasks/side-quest`, execute the task loop, and record milestones with `run advance`.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. Run `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post`.
Run QA through `./.claude/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`, then `./.claude/bin/codedungeon run finalize`.
</evidence_gates>

<output_contract>
READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`. Return the CodeDungeon PR Report shape and include the Project Rules envelope.
</output_contract>
