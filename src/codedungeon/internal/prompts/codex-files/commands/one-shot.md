# one-shot

Minimal CodeDungeon workflow for a small Codex task that still needs branch, commit, PR, and review.

Use when the request can be handled by one planner pass and one implementation pass. Do not split into task files, do not run the full phase pipeline, and do not call `codedungeon-loop`.

Steps:
- Validate setup with `./.codex/bin/codedungeon git guard --repo .` and stop on protected branches.
- Write a short plan to `.codedungeon/plans/one-shot/PLAN.md`.
- Create or switch to `feat/<slug>`.
- Implement directly from the plan with focused verification.
- Commit, push, and create or reuse a GitHub PR.
- Run `$code-review` against the PR.
- If review requests changes, fix directly and rerun review up to 3 cycles.

Return:
- `ONE_SHOT_COMPLETE`
- branch
- PR URL
- review verdict
- verification commands and results
- changed files and residual risks

Escalate to `$main-quest` when the request needs multi-repo coordination, explicit QA phases, task decomposition, or a final report.
