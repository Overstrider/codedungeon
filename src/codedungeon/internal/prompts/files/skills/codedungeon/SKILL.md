---
name: codedungeon
description: Route Claude Code requests through the project-local CodeDungeon workflow router.
---

# codedungeon

Use the project-local CLI for all workflow state:

```bash
./.claude/bin/codedungeon
```

Before planning, executing, reviewing, or reporting completion, run
`./.claude/bin/codedungeon rules status` and read
`.codedungeon/project-rules.compact.md` when present.

<workflow_contract>
Run `/codedungeon --rules` for Project Rules discovery, then dispatch with `/codedungeon --full`, `/codedungeon --lite`, `/codedungeon --oneshot`, or `/codedungeon --auto`. The router starts or resumes `./.claude/bin/codedungeon run --full|--lite|--oneshot --prompt "<prompt>"` and returns `current_step`, `blockers`, `timeline`, and `next_action`.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. Use `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post`.
Do not write final reports manually. Run QA through `./.claude/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`, then run `./.claude/bin/codedungeon run finalize`.
</evidence_gates>

<output_contract>
READY_FOR_USER_REVIEW can only come from CodeDungeon finalization. Every plan, task, review, handoff, and final report must include PROJECT_RULES_STATUS, PROJECT_RULES_DIGEST, and PROJECT_RULES_READ.
</output_contract>
