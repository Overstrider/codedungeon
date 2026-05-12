# CodeDungeon Workflows

CodeDungeon installs agent-facing workflows for both providers. Claude Code invokes them as slash commands. Codex invokes them as skills. The promoted surface is a single router:

- Claude Code: `/codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>`
- Codex: `$codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>`

Without a mode flag, the router behaves as `--auto` and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>` before following a workflow. Code-writing modes delegate to `codedungeon run --full|--lite|--oneshot --prompt "<prompt>"`; the runner creates or resumes durable workflow state and returns an agent-first JSON contract with `current_step`, `blockers`, `timeline`, and `next_action`. The current agent executes the step with native tools, then records progress with `codedungeon run advance`.

`codedungeon kernel` is the read-only capability manifest for agents. It emits
JSON that names the Codex and Claude provider surfaces, workflow modes,
deterministic modules, completion gates, SQLite/FTS5 state, project-local
configuration scope, and license. Agents should use it when they need to inspect
the workflow kernel contract instead of inferring capabilities from prose.

Mid-flow gates are soft: missing Project Rules, GitHub PR readiness, failed QA,
or review changes are returned as structured blockers and next actions. Final
delivery is still hard-gated: `codedungeon run finalize` is the only command
that can emit `READY_FOR_USER_REVIEW`.

| Mode | Claude | Codex | Workflow | Use when |
|------|--------|-------|----------|----------|
| `--oneshot` | `/codedungeon --oneshot` | `$codedungeon --oneshot` | One Shot | Small scoped change that needs plan, code, branch, PR, and review without task splitting. |
| `--one-shot` | `/codedungeon --one-shot` | `$codedungeon --one-shot` | One Shot | Compatibility spelling for `--oneshot`. |
| `--lite` | `/codedungeon --lite` | `$codedungeon --lite` | Side Quest | Simple single-repo work with a prior plan under `.codedungeon/plans/*.md`. |
| `--full` | `/codedungeon --full` | `$codedungeon --full` | Main Quest | Complex features, multi-repo work, full phase lifecycle, QA, tests, and final report. |
| `--auto` | `/codedungeon --auto` | `$codedungeon --auto` | Router-selected | Explicit automatic selection. |
| `--rules` | `/codedungeon --rules` | `$codedungeon --rules` | Project Rules Discovery | Deep-read the repo, draft `.codedungeon/project-rules.md`, wait for user confirmation, then approve and compact rules. |
| Task Maker | `/task-maker` | `$task-maker` | Task Maker | Clarify a rough request, persist a minimal design, and prepare a reviewed English run-full prompt before a full run. |
| Review | `/code-review` | `$code-review` | Code Review | Standalone adversarial review for the current branch or PR. |
| Execute | n/a | n/a | Implementation Executor | Standalone `codedungeon execute task --task task.json` runner for one task contract at a time. |

Compatibility aliases remain installed and supported: `/one-shot`, `/side-quest`, `/main-quest` for Claude Code, and `$one-shot`, `$side-quest`, `$main-quest` for Codex.

Router validation:

- Multiple mode flags stop with usage guidance.
- Empty prompts stop with examples.
- `--rules` may run without a user prompt and must not be combined with another mode flag.
- `--lite` requires a prior plan in `.codedungeon/plans/*.md` or an explicit plan path in the prompt.
- Auto mode chooses `full` for complex, architectural, multi-repo, QA/test, or final-report work; `lite` when a plan exists and the prompt asks to execute, split, or continue simple planned work; and `oneshot` for small direct changes.

## Task Maker

Task Maker is available in both provider packs. Invoke `$task-maker` in Codex or `/task-maker` in Claude Code when the user has a rough request and wants help shaping it before `$codedungeon --full` or `/codedungeon --full`.

The command or skill stays in the user's language during clarification, asks one material question per turn, and records assumptions for minor ambiguity. After the user confirms, it writes a request JSON and runs the renderer with the provider surface:

```bash
codedungeon task-maker render --surface codex --input .codedungeon/task-maker/sessions/<session>/request.json --out .codedungeon/task-maker/sessions/<session> --print
codedungeon task-maker render --surface claude --input .codedungeon/task-maker/sessions/<session>/request.json --out .codedungeon/task-maker/sessions/<session> --print
```

The renderer writes `request.json`, `design.md`, `prompt.txt`, and `output.md`. The printed output always contains `# Task Maker Output`, a minimal design, a concise English run-full prompt, and a provider-native command: `$codedungeon --full "<prompt>"` for Codex or `/codedungeon --full "<prompt>"` for Claude Code. Task Maker must not start the run-full workflow automatically; the user decides whether to run the prompt after review.

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

`--full` and `--lite` return stale, draft, or missing rules as soft blockers in
the agent-first run contract. Agents should refresh or approve rules before
broad planning, but the hard stop is finalization: final reports must include
the Project Rules envelope and cannot claim READY_FOR_USER_REVIEW with missing
rules evidence.

Optional hook enforcement:

```bash
codedungeon hooks install --provider codex --mode warn
codedungeon hooks install --provider claude --mode warn
```

Codex hooks gate prompt/tool/stop events. Claude hooks additionally describe task/subagent events such as `TaskCreated`, `TaskCompleted`, and `SubagentStop`. `warn` mode reports problems; `enforce` mode blocks completion claims that omit Project Rules status/digest or verification.

## Success Gate

CodeDungeon is PR-centered and requires GitHub plus an authenticated GitHub CLI before completion. Agent-first runs may surface missing `git remote get-url origin` or `gh auth status` as structured finalization blockers while planning/execution continues, but there is no local-only READY_FOR_USER_REVIEW path. A code-writing workflow succeeds only when:

1. Final build/check/test verification is produced by the QA module. Agents may run explicit checks with `codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"` and subsequent `codedungeon qa run --phase 6 --cmd "<cmd>"`; `codedungeon run finalize` also invokes `codedungeon qa run --auto` behavior automatically when the active Phase 6 ledger is missing or failing.
2. The branch is pushed.
3. A GitHub PR exists or is reused.
4. Standalone CodeDungeon code review runs with `codedungeon code-review --url <pr-url> --project-context <path> --task-context <path> --out .codedungeon/code-review --post`, writes persona/adjudication artifacts, records review evidence, and posts the concise final review comment to the PR.
5. The final review verdict is `APPROVED`.
6. `codedungeon run finalize` closes eligible final phases, verifies gates, generates the final report from DB evidence, records Phase 7, and leaves the PR open for human review.

If any step fails, the workflow must return `BLOCKED` or `MAX_CYCLES_REACHED`, never `READY_FOR_USER_REVIEW`.

Lower-level `codedungeon review run` and `codedungeon review post` remain available for legacy adversarial pipeline evidence, but they are not the normal final approval surface for current workflows.

Agents must not write review reports or final reports manually. Phase gates consume the DB evidence written by `codedungeon code-review --post`, `codedungeon qa run`, `codedungeon git verify`, and `codedungeon run finalize`. QA evidence is session-scoped under `.codedungeon/qa/sessions/<qa-session-id>/` with request/result JSON, preflight data, logs, checks, findings, and summaries. `codedungeon report render` remains a lower-level renderer, not the normal workflow completion command.

Runtime evidence is also indexed in the artifact registry. Use `codedungeon artifacts list --latest-run` to inspect evidence rows, `codedungeon artifacts verify --latest-run` to detect missing or drifted files, and `codedungeon artifacts backfill --run <run-id>` to index evidence from older or interrupted runs.

## Agent Telemetry

CodeDungeon records informational agent telemetry in the project DB. Agent-first workflows should record every phase agent, worker, review persona, validator, classifier, and stack-specialist delegation with `codedungeon trace agent-start` before spawning and `codedungeon trace agent-end` after return.

Telemetry is not a readiness gate. Missing or open telemetry appears as a warning in final reports and in `codedungeon observe report`, but it does not replace QA, review, PR, or report evidence. Use `codedungeon observe agents` for JSON and `codedungeon observe report` for an audit-friendly Markdown timeline.

`--lite` and `--oneshot` are compact workflows: the runner marks the pre-report phase ledger as skipped, then enforces readiness through QA, review, PR, and report evidence. `--full` keeps the full ordered phase lifecycle.

## Implementation Executor

`codedungeon execute task --task <task.json>` is the deterministic Ralphloop module under the higher-level workflows. It accepts one `taskplanning.TaskSpec` JSON contract plus project context, creates a durable execution session, and runs a Codex-first worker loop with per-attempt git snapshots, declared verification commands, and JSON evidence under `.codedungeon/execute/sessions/<session-id>/`.

Session behavior is explicit: resume uses `--resume <id>`, never an implicit “continue latest”; sessions expire after 24 hours by default; `--reset-session` reopens an expired or failed session and records a transition. `execute status --session <id>` reports session, attempts, and transition history. `execute rollback --session <id> --to before|attempt-N --confirm` prints the rollback target for manual recovery; it does not silently reset the repository.

`.ralphrc` can override executor defaults: `session_ttl_hours`, `max_iterations`, `timeout_seconds`, `runner`, `auto_commit`, `auto_push`, `auto_tag`, `verbose`, and `allowed_tools`. Environment variables prefixed with `CODEDUNGEON_EXEC_` override config. Auto-commit is enabled only after verification passes; auto-push and semver tag creation are opt-in.

`APPROVED` does not replace verification. For Rust work, the verification gate includes `cargo check` and `cargo test`. If `Dockerfile` or `Containerfile` changes, the workflow must run `podman build` or return `BLOCKED` with the environment blocker. If a command is recorded multiple times in the Phase 6 ledger, the latest record for that exact command wins.

Standard final output:

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        READY_FOR_USER_REVIEW|BLOCKED|MAX_CYCLES_REACHED
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
- Telemetry: OK|WARN - <agent timeline summary>

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

1. Validate setup, GitHub remote, git repo state, and `gh auth status`.
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

Phase 5 requires approved standalone code-review evidence and a pushed GitHub PR branch. Phase 6 requires a passing active verification ledger produced by `codedungeon qa run`; when `codedungeon run finalize` finds the ledger missing or failing, it runs workflow QA automatically in `auto` mode and writes the Phase 6 evidence itself. Explicit reruns can still use `--fresh` to supersede earlier records. Phase 7 is closed by `codedungeon run finalize` after prior phases, review evidence, recorded PR review-post evidence, verification ledger, `codedungeon git verify`, and report rendering all pass. `codedungeon git verify` rejects arbitrary marker comments and merged PRs.

`codedungeon qa detect-framework --path .` detects single projects and common monorepos. For monorepos it returns `framework: monorepo` with component commands in `components` and `run_cmds`.

Standalone QA commands:

```text
codedungeon qa run --auto
codedungeon qa run --cmd "go test ./..."
codedungeon qa preflight --mode e2e
codedungeon qa status --latest
codedungeon qa report --latest
```

Playwright is treated as an external dependency. If E2E mode requires it and the project lacks Playwright, QA returns `BLOCKED` with an install hint instead of reporting a code failure.

Use `side-quest` or `one-shot` instead when the work does not need the full architecture, QA, test decomposition, and final-report lifecycle.
