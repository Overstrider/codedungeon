---
name: cleanup-tasks
description: Clean stale CodeDungeon task artifacts without deleting runtime history.
---

# cleanup-tasks

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before cleanup or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope for cleanup reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Inspect stale plans, tasks, QA sessions, and review evidence before cleanup. Use the editable `.codedungeon/commands/cleanup-tasks.md` playbook for project-specific details.
</workflow_contract>

<evidence_gates>
Do not delete `.codedungeon/codedungeon.db` or runtime history unless the user explicitly asks. Preserve standalone `codedungeon code-review` and QA evidence needed by finalization.
</evidence_gates>

<output_contract>
Report what was kept, archived, or removed. Include Project Rules status, digest, and read flags when cleanup is part of an active workflow.
</output_contract>
