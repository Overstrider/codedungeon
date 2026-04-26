# side-quest

Use for simple single-repo changes that should be split into a few explicit tasks before implementation.

Steps:
- Resolve or write a short plan under `.codedungeon/plans/`.
- Create `.codedungeon/tasks/side-quest/PLAN.md` plus focused `TASK-NNN.md` files.
- Create or switch to `feat/<slug>` and verify with `./.codex/bin/codedungeon git guard --repo .`.
- Execute tasks in order with focused verification.
- Commit, push, and create or reuse a GitHub PR.
- Run `$code-review`; fix requested changes and rerun review for up to 9 cycles.
- Use full review mode for cycles 1-3, then reduced mode for cycles 4-9: keep personas, use fast model/effort, and focus on fixes/new diff.
- Return the standard CodeDungeon PR Report. `COMPLETE` requires pushed branch, PR URL, adversarial review comment, and `APPROVED` verdict.

Return format:

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        COMPLETE|BLOCKED|MAX_CYCLES_REACHED
| Workflow      side-quest
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
- Tasks: <n>/<total>
- Changed files: <short summary or none>
- Verification: <commands/results or blocker>

PR
<url or "not created">

Next
<none or exact next human/agent action>
```

Use `$one-shot` when task splitting is unnecessary. Use `$main-quest` for full phase lifecycle work.
