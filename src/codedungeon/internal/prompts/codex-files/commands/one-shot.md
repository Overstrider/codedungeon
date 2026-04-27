# one-shot

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Minimal CodeDungeon workflow for a small Codex task that still needs branch, commit, PR, and review.

Use when the request can be handled by one planner pass and one implementation pass. Do not split into task files, do not run the full phase pipeline, and do not call `codedungeon-loop`.

## Evidence Gates

- Do not write review reports manually. Persona outputs must be real files such as `findings-saboteur.json`, declared in `review-manifest.json`, then aggregated with `./.codex/bin/codedungeon review run`.
- Do not write final reports manually. COMPLETE can only come from `codedungeon report render` after phase, review, git, and QA gates pass.
- Record every concrete build/check/test command with `./.codex/bin/codedungeon qa record --phase 6 --cmd "<cmd>" --status PASS|FAIL --log <path>`.
- Review is mandatory for code-writing workflows; do not treat `Review: APPROVED` as a substitute for `Verification: PASS`.

Steps:
- Validate setup, git repo state, `origin`, and `gh auth status` before editing.
- Write a short plan to `.codedungeon/plans/one-shot/PLAN.md`.
- Create or switch to `feat/<slug>`, then run `./.codex/bin/codedungeon git guard --repo .` before editing.
- Implement directly from the plan with focused verification.
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

Escalate to `$main-quest` when the request needs multi-repo coordination, explicit QA phases, task decomposition, or a final report.
