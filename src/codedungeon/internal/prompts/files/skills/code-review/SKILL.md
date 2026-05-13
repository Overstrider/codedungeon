---
name: code-review
description: Run standalone Claude-native CodeDungeon PR review evidence.
---

# code-review

Use only the project-local CLI:

```bash
./.claude/bin/codedungeon
```

Before reviewing or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present.

Project Rules envelope required in review evidence and final handoffs:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

<workflow_contract>
Run the standalone module: `./.claude/bin/codedungeon code-review --url <PR URL> --project-context <path-or-text> --task-context <path-or-text> --out .codedungeon/code-review --post`.
</workflow_contract>

<evidence_gates>
Do not write review reports manually. The standalone module is the final adjudicator; empty template reviews, missing adjudicator decisions, or legacy `review run` output are invalid as final approval evidence.
Treat missing build/check/test evidence as BLOCKING because APPROVED does not replace verification.
</evidence_gates>

<output_contract>
Post only the concise final adjudication from `codedungeon code-review`. Never publish per-persona approvals as the PR verdict.
</output_contract>
