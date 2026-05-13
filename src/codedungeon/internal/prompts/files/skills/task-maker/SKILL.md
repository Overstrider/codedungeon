---
name: task-maker
description: Clarify an ambiguous request and render a reviewed Claude-native full-workflow prompt.
---

# task-maker

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before rendering workflow guidance, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope to preserve for downstream workflows:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Ask one material question per turn, persist the request under `.codedungeon/task-maker/sessions/<session>/`, then render with `./.claude/bin/codedungeon task-maker render --surface claude --input .codedungeon/task-maker/sessions/<session>/request.json --out .codedungeon/task-maker/sessions/<session> --print`.
</workflow_contract>

<evidence_gates>
Do not dispatch `/codedungeon --full` automatically. The reviewed prompt should tell the user to use `codedungeon code-review` inside the normal workflow gates.
</evidence_gates>

<output_contract>
Final output is English and includes the rendered prompt, command, minimal design, Project Rules envelope requirements, and the explicit next action for user approval.
</output_contract>
