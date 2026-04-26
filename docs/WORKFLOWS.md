# CodeDungeon Workflows

CodeDungeon installs agent-facing workflows for both providers. Claude Code invokes them as slash commands. Codex invokes them as skills. The promoted surface is a single router:

- Claude Code: `/codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>`
- Codex: `$codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>`

Without a mode flag, the router behaves as `--auto` and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>` before following a workflow.

| Mode | Claude | Codex | Workflow | Use when |
|------|--------|-------|----------|----------|
| `--oneshot` | `/codedungeon --oneshot` | `$codedungeon --oneshot` | One Shot | Small scoped change that needs plan, code, branch, PR, and review without task splitting. |
| `--one-shot` | `/codedungeon --one-shot` | `$codedungeon --one-shot` | One Shot | Compatibility spelling for `--oneshot`. |
| `--lite` | `/codedungeon --lite` | `$codedungeon --lite` | Side Quest | Simple single-repo work with a prior plan under `.codedungeon/plans/*.md`. |
| `--full` | `/codedungeon --full` | `$codedungeon --full` | Main Quest | Complex features, multi-repo work, full phase lifecycle, QA, tests, and final report. |
| `--auto` | `/codedungeon --auto` | `$codedungeon --auto` | Router-selected | Explicit automatic selection. |
| `--rules` | `/codedungeon --rules` | `$codedungeon --rules` | Project Rules Discovery | Deep-read the repo, draft `.codedungeon/project-rules.md`, wait for user confirmation, then approve and compact rules. |
| Review | `/code-review` | `$code-review` | Code Review | Standalone adversarial review for the current branch or PR. |

Compatibility aliases remain installed and supported: `/one-shot`, `/side-quest`, `/main-quest` for Claude Code, and `$one-shot`, `$side-quest`, `$main-quest` for Codex.

Router validation:

- Multiple mode flags stop with usage guidance.
- Empty prompts stop with examples.
- `--rules` may run without a user prompt and must not be combined with another mode flag.
- `--lite` requires a prior plan in `.codedungeon/plans/*.md` or an explicit plan path in the prompt.
- Auto mode chooses `full` for complex, architectural, multi-repo, QA/test, or final-report work; `lite` when a plan exists and the prompt asks to execute, split, or continue simple planned work; and `oneshot` for small direct changes.

## Project Rules

Project Rules are the shared context layer that keeps planner, implementer, tester, and reviewer agents aligned.

Lifecycle:

1. Run `/codedungeon --rules` or `$codedungeon --rules`.
2. The agent reads repo docs/config/manifests/CI/test files and writes `.codedungeon/project-rules.md` with `Status: DRAFT`.
3. The user reviews or edits the draft.
4. After explicit confirmation, the agent runs `codedungeon rules approve` and `codedungeon rules compact`.
5. Workflows read `.codedungeon/project-rules.compact.md` and include the envelope below in plans, tasks, reviews, handoffs, and reports.

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Deterministic commands:

```bash
codedungeon rules status
codedungeon rules lint
codedungeon rules digest
codedungeon rules approve --by <name>
codedungeon rules compact
codedungeon rules gate --event Stop --mode warn
```

`--full` and `--lite` should block when approved rules are stale or still draft unless the user explicitly says to proceed with stale rules. `--oneshot` may continue with a warning for small direct fixes.

Optional hook enforcement:

```bash
codedungeon hooks install --provider codex --mode warn
codedungeon hooks install --provider claude --mode warn
```

Codex hooks gate prompt/tool/stop events. Claude hooks additionally describe task/subagent events such as `TaskCreated`, `TaskCompleted`, and `SubagentStop`. `warn` mode reports problems; `enforce` mode blocks completion claims that omit Project Rules status/digest or verification.

## Success Gate

CodeDungeon is PR-centered. Any workflow that writes code succeeds only when:

1. Build/check/test verification runs and passes.
2. The branch is pushed.
3. A GitHub PR exists or is reused.
4. Code review is posted to the PR.
5. The final review verdict is `APPROVED`.
6. The workflow returns the standard CodeDungeon PR Report with `Verification: PASS`.

If any step fails, the workflow must return `BLOCKED` or `MAX_CYCLES_REACHED`, never `COMPLETE`.

`APPROVED` does not replace verification. For Rust work, the verification gate includes `cargo check` and `cargo test`. If `Dockerfile` or `Containerfile` changes, the workflow must run `podman build` or return `BLOCKED` with the environment blocker.

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
- Verification: PASS - <commands/results> OR BLOCKED - <blocker>

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

`side-quest` is for compact work that still benefits from explicit task files. Through the unified router this is `/codedungeon --lite` or `$codedungeon --lite`, and it requires a prior plan.

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
