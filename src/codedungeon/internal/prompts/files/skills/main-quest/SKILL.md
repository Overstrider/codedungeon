---
name: main-quest
description: Run the full Claude-native CodeDungeon workflow for complex implementation work.
---

# main-quest

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before planning, executing, reviewing, or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope required in plans, tasks, reviews, phase handoffs, and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Start or resume durable state with `./.claude/bin/codedungeon run --full --prompt "<prompt>"`. Follow the returned `current_step` and `next_action`; record coarse milestones with `run advance --step planning|execution|code_review|qa`.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. For each PR, run `./.claude/bin/codedungeon code-review --out .codedungeon/code-review/<repo> --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/tasks/<feature>/<repo>/PLAN.md --post`.
Run verification with `./.claude/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`, then use `./.claude/bin/codedungeon run finalize`.
</evidence_gates>

<output_contract>
READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`. Preserve the Project Rules envelope in every plan, task, review, phase handoff, and final report.
</output_contract>
