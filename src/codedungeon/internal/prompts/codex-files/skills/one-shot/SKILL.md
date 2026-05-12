---
name: one-shot
description: "Run a minimal CodeDungeon workflow for one small Codex task: plan, implement, create PR, and run review without task splitting."
---

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. Missing, draft, or stale rules are soft blockers while the agent is shaping work, but finalization must not claim READY_FOR_USER_REVIEW without the required Project Rules envelope.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

# one-shot

Use for small, well-scoped changes that still need branch, commit, PR, and adversarial review.

This workflow is agent-first. Start or resume durable state with:

```bash
./.codex/bin/codedungeon run --oneshot --prompt "<prompt>"
```

## Evidence Gates

- Do not write review reports manually. Code review is a standalone module: run `./.codex/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post`; legacy `review run` is not final approval evidence.
- Do not write final reports manually. READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`, which closes eligible final phases, cleans stale telemetry, and renders the report after phase, review, git, and QA gates pass.
- Start final verification with `./.codex/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`; execute subsequent concrete build/check/test commands with `./.codex/bin/codedungeon qa run --phase 6 --cmd "<cmd>"`.
- Review is mandatory for code-writing workflows; do not treat `Review: APPROVED` as a substitute for `Verification: PASS`.

## Agent Telemetry

- Before each planner, review, or fix subagent spawn, run `./.codex/bin/codedungeon trace agent-start` with phase, role, agent type, model, effort, task path, and summary.
- After the subagent returns, run `./.codex/bin/codedungeon trace agent-end` with the returned `agent_run_id`, terminal status, result summary, artifact path, and error when present.
- Telemetry is informational and must not replace QA, review, PR, or report evidence gates.

Workflow:
- Validate setup and local git repo state before editing. Treat missing `origin` or `gh auth status` as finalization blockers surfaced by `codedungeon run status` / `codedungeon run finalize --dry-run`, not as local planning or implementation blockers.
- Write a short plan to `.codedungeon/plans/one-shot/PLAN.md`.
- Record planning with `./.codex/bin/codedungeon run advance --step planning --status completed --summary "one-shot plan written" --artifact .codedungeon/plans/one-shot/PLAN.md`.
- Create or reuse a `feat/<slug>` branch, then run `./.codex/bin/codedungeon git guard --repo .` before editing.
- Implement directly from the plan; do not create `.codedungeon/tasks/*`.
- Record execution with `./.codex/bin/codedungeon run advance --step execution --status completed --summary "one-shot implementation complete"`.
- Run focused verification through `./.codex/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`, then record QA with `./.codex/bin/codedungeon run advance --step qa --status completed --summary "verification recorded" --artifact .codedungeon/qa`.
- Commit, push, and reuse the current branch PR when it exists; otherwise create one.
- Run `$code-review` against the PR.
- Record approved review with `./.codex/bin/codedungeon run advance --step code_review --status completed --summary "review approved" --artifact .codedungeon/code-review`.
- Review posting is handled by `codedungeon code-review --post`; arbitrary marker comments do not satisfy `git verify`.
- If review requests changes, fix directly and rerun review up to 9 cycles.
- Use full review mode for cycles 1-3, then reduced mode for cycles 4-9: keep personas, use fast model/effort, and focus on fixes/new diff.
- Run `./.codex/bin/codedungeon run finalize`.
- Return the standard CodeDungeon PR Report. `READY_FOR_USER_REVIEW` is valid only after `codedungeon run finalize` succeeds. Do not merge; the user performs final review and merge.

Return:
- CodeDungeon PR Report block:

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        READY_FOR_USER_REVIEW|BLOCKED|MAX_CYCLES_REACHED
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
- Last review marker: CodeDungeon Code Review|none
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
