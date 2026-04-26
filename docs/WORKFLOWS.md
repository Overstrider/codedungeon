# CodeDungeon Workflows

CodeDungeon installs agent-facing workflows for both providers. Claude Code invokes them as slash commands. Codex invokes them as skills.

| Workflow | Claude | Codex | Use when |
|----------|--------|-------|----------|
| One Shot | `/one-shot` | `$one-shot` | Small scoped change that needs plan, code, branch, PR, and review without task splitting. |
| Side Quest | `/side-quest` | `$side-quest` | Simple single-repo work that should be split into a few explicit tasks before execution. |
| Main Quest | `/main-quest` | `$main-quest` | Complex features, multi-repo work, full phase lifecycle, QA, tests, and final report. |
| Code Review | `/code-review` | `$code-review` | Standalone adversarial review for the current branch or PR. |

## Success Gate

CodeDungeon is PR-centered. Any workflow that writes code succeeds only when:

1. The branch is pushed.
2. A GitHub PR exists or is reused.
3. Code review is posted to the PR.
4. The final review verdict is `APPROVED`.
5. The workflow returns the standard CodeDungeon PR Report.

If any step fails, the workflow must return `BLOCKED` or `MAX_CYCLES_REACHED`, never `COMPLETE`.

Standard final output:

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        COMPLETE|BLOCKED|MAX_CYCLES_REACHED
| Workflow      one-shot|side-quest|main-quest|codedungeon-loop
| PR            #<number> <url>
| Branch        <branch>
| Review        APPROVED|CHANGES_REQUESTED|MAX_CYCLES_REACHED|NOT_RUN
| Cycles        <n>/9 | last mode: full|reduced|not_run
+------------------------------------------------+

Summary
<1-line task/result summary>

Review
- Adversarial comments: <n>
- Last review marker: <provider marker or none>
- Remaining findings: <none or short list/count>

Work Done
- Tasks: <n>/<total or n/a>
- Changed files: <short summary or none>
- Verification: <commands/results or blocker>

PR
<url or "not created">

Next
<none or exact next human/agent action>
```

Review cycles:

- Cycles 1-3: full adversarial mode.
- Cycles 4-9: reduced mode. Personas remain enabled, but the agent uses fast model/effort and focuses on fixes or new diff since the previous cycle.

## One Shot

`one-shot` is the smallest PR-producing workflow. It is intended for requests that can be handled by one planner pass and one implementation pass.

Expected behavior:

1. Validate setup, git repo state, and `gh auth status`.
2. Write a short plan to `.codedungeon/plans/one-shot/PLAN.md`.
3. Create or switch to `feat/<slug>`.
4. Run `codedungeon git guard --repo .` after switching branches and before editing.
5. Implement directly from the plan without creating `.codedungeon/tasks/*`.
6. Run focused verification.
7. Commit, push, and reuse the current branch PR if it exists; otherwise create one.
8. Run code review and fix requested changes for up to 9 review cycles.

The branch-before-guard order is intentional. `codedungeon git guard` rejects protected branches such as `main`, so `one-shot` must create or switch to a feature branch before calling guard.

## Side Quest

`side-quest` is for compact work that still benefits from explicit task files.

Expected behavior:

1. Resolve a plan from `.codedungeon/plans/*.md`.
2. Write task state under `.codedungeon/tasks/side-quest/`.
3. Create or switch to `feat/<slug>`.
4. Execute tasks through the normal implementation and review loop.
5. Create or reuse a PR and run code review.

Use `one-shot` instead when task splitting would be overhead.

## Main Quest

`main-quest` runs the full phase workflow:

```text
0 -> 1 -> 2' -> 3.5 -> 4 -> 5 -> 5.5 -> 5.6 -> 6 -> 7
```

It stores phase state, plans, handoffs, tasks, reviews, and reports in `.codedungeon/` so interrupted work can resume and past PR work remains inspectable.

Use `side-quest` or `one-shot` instead when the work does not need the full architecture, QA, test decomposition, and final-report lifecycle.
