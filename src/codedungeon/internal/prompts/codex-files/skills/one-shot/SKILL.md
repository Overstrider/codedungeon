---
name: one-shot
description: Run a minimal CodeDungeon workflow for one small Codex task: plan, implement, create PR, and run review without task splitting.
---

# one-shot

Use for small, well-scoped changes that still need branch, commit, PR, and adversarial review.

Workflow:
- Validate setup, git repo state, `origin`, and `gh auth status` before editing.
- Write a short plan to `.codedungeon/plans/one-shot/PLAN.md`.
- Create or reuse a `feat/<slug>` branch, then run `./.codex/bin/codedungeon git guard --repo .` before editing.
- Implement directly from the plan; do not create `.codedungeon/tasks/*`.
- Run focused verification.
- Commit, push, and reuse the current branch PR when it exists; otherwise create one.
- Run `$code-review` against the PR.
- If review requests changes, fix directly and rerun review up to 9 cycles.
- Use full review mode for cycles 1-3, then reduced mode for cycles 4-9: keep personas, use fast model/effort, and focus on fixes/new diff.
- Return the standard CodeDungeon PR Report. `COMPLETE` requires pushed branch, PR URL, adversarial review comment, and `APPROVED` verdict.

Return:
- CodeDungeon PR Report block:

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        COMPLETE|BLOCKED|MAX_CYCLES_REACHED
| Workflow      one-shot
| PR            #<number> <url>
| Branch        <branch>
| Review        APPROVED|CHANGES_REQUESTED|MAX_CYCLES_REACHED|NOT_RUN
| Cycles        <n>/9 | last mode: full|reduced|not_run
+------------------------------------------------+

Summary
<1-line task/result summary>

Review
- Adversarial comments: <n>
- Last review marker: Codex Adversarial Code Review|none
- Remaining findings: <none or short list/count>

Work Done
- Tasks: n/a
- Changed files: <short summary or none>
- Verification: <commands/results or blocker>

PR
<url or "not created">

Next
<none or exact next human/agent action>
```

Use `$side-quest` when there is already a plan that should be split into tasks. Use `$main-quest` for full phase lifecycle work.
