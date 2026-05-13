---
name: one-shot
description: Run a minimal Claude-native CodeDungeon workflow for one narrow implementation task.
---

# one-shot

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before planning, executing, reviewing, or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope required in plans, reviews, handoffs, and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Start or resume with `./.claude/bin/codedungeon run --oneshot --prompt "<prompt>"`. Write `.codedungeon/plans/one-shot/PLAN.md`, implement the narrow change, reuse or create a PR, and record planning, execution, code_review, and qa milestones.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. Run `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/plans/one-shot/PLAN.md --out .codedungeon/code-review --post`.
After review approval, run `./.claude/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`, then `./.claude/bin/codedungeon run finalize`.
</evidence_gates>

<output_contract>
READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`. Return the CodeDungeon PR Report shape with Project Rules fields.
</output_contract>
