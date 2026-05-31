# CodeDungeon Workflows

Deep reference for modes, gates, and the deterministic modules. For the overview and command
table see [`../README.md`](../README.md); for the authoritative machine contract run
`codedungeon kernel`.

Router (both providers): `/codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>`
(Codex: `$codedungeon …`). No flag = `--auto`, which prints `CODEDUNGEON_MODE_SELECTED:
<mode> - <reason>`. Code modes delegate to `codedungeon run …`, which returns the agent-first
JSON contract (`current_step`, `blockers`, `timeline`, `next_action`); the agent executes and
records with `codedungeon run advance`.

Router validation: multiple mode flags or an empty prompt (except `--rules`) stop with usage;
`--lite` requires a plan in `.codedungeon/plans/*.md`; auto picks `full` for complex/
multi-repo/QA work, `lite` when a plan exists, else `oneshot`.

Mid-flow gates are **soft** (missing rules, PR readiness, failed QA → structured blockers).
Final delivery is **hard**: only `codedungeon run finalize` emits `READY_FOR_USER_REVIEW`.

## Task Maker

`/task-maker` (`$task-maker`) shapes a rough request before a full run. It clarifies (one
question per turn, in the user's language), then renders artifacts and prints a ready command
**without starting the run**:

```bash
codedungeon task-maker render --surface claude --input <session>/request.json --out <session> --print
```

Writes `request.json`, `design.md`, `prompt.txt`, `output.md` under
`.codedungeon/task-maker/sessions/<session>/`; the output ends with `/codedungeon --full "<prompt>"`.

## Project Rules

The shared context layer aligning planner/implementer/tester/reviewer agents.

1. `/codedungeon --rules` → agent reads docs/config/CI/tests, writes `.codedungeon/project-rules.md` (`Status: DRAFT`).
2. User reviews/edits.
3. On confirmation: `codedungeon rules approve` + `codedungeon rules compact` → `.codedungeon/project-rules.compact.md`.
4. Workflows read the compact rules and carry the envelope in every plan/task/review/report:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <from `rules status`, or none>
PROJECT_RULES_READ: yes|no
```

Commands: `rules status | lint | digest | approve --by <name> | compact | gate --event <e> --mode <warn|enforce>`.
`--full`/`--lite` surface stale/draft/missing rules as soft blockers; finalization is the hard
stop (no READY_FOR_USER_REVIEW without rules evidence).

Optional enforcement hooks: `codedungeon hooks install --provider <claude|codex> --mode <warn|enforce>`.
Hooks are **cross-platform** — a `.sh` (bash) hook on Linux/macOS, a `.ps1` (PowerShell) hook on
Windows — and gate prompt/tool/stop (and, for Claude, task/subagent) events. `enforce` blocks
completion claims missing rules/verification (exit code 2 on Stop/SubagentStop).

## Success gate

PR-centered; requires GitHub `origin` + authenticated `gh`. No local-only completion. A
code-writing workflow reaches `READY_FOR_USER_REVIEW` only when:

1. QA produced passing verification (`codedungeon qa run`; `run finalize` runs auto-QA if the Phase 6 ledger is missing/failing).
2. Branch pushed; PR exists/open.
3. `codedungeon code-review --url <pr> --post` wrote persona/adjudication artifacts and posted the PR comment.
4. Final verdict `APPROVED`.
5. `codedungeon git verify` accepts the PR/review state (rejects arbitrary marker comments and merged PRs).
6. `codedungeon run finalize` closes phases, renders the report from DB evidence, records Phase 7, leaves the PR open.

Any failure → `BLOCKED` or `MAX_CYCLES_REACHED`, never `READY_FOR_USER_REVIEW`. Agents must not
hand-write review/final reports — gates consume DB evidence from `code-review --post`, `qa run`,
`git verify`, `run finalize`. Inspect evidence via `artifacts list|verify --latest-run` (and
`backfill --run <id>` for older runs). QA evidence is session-scoped under
`.codedungeon/qa/sessions/<id>/`.

Review cycles: 1–3 full adversarial; 4–9 reduced (fast model, focus on fixes/new diff).

Telemetry (`codedungeon trace agent-start|agent-end`, view with `observe agents|report`) is
informational — a warning, never a readiness gate.

## Implementation Executor

`codedungeon execute task --task <task.json>` is the deterministic per-task runner under the
workflows. It takes one task contract + project context, opens a durable session, and runs a
Codex-first worker loop with per-attempt git snapshots, declared verification commands, and JSON
evidence under `.codedungeon/execute/sessions/<id>/`.

- Resume is explicit (`--resume <id>`), never implicit. Sessions expire after 24h;
  `--reset-session` reopens. `execute status --session <id>` shows attempts/transitions.
  `execute rollback --session <id> --to before|attempt-N --confirm` prints the target (no silent reset).
- `.ralphrc` overrides defaults (`session_ttl_hours`, `max_iterations`, `timeout_seconds`,
  `runner`, `auto_commit`, `auto_push`, `auto_tag`, `allowed_tools`); `CODEDUNGEON_EXEC_*` env
  vars override config. Auto-commit only after verification passes; auto-push/tag are opt-in.
- Verification is mandatory, not replaced by `APPROVED`: Rust includes `cargo check`+`cargo test`;
  a `Dockerfile`/`Containerfile` change requires `podman build` or returns `BLOCKED`.

## Mode specifics

**One Shot** (`--oneshot`): smallest PR-producing flow — validate env, write
`.codedungeon/plans/one-shot/PLAN.md`, create `feat/<slug>`, run `git guard` (after branching —
guard rejects `main`), implement directly (no task files), verify, push/PR, review (≤9 cycles).

**Side Quest** (`--lite`): resolve a plan from `.codedungeon/plans/*.md`, write task state under
`.codedungeon/tasks/side-quest/`, branch, execute through the normal loop, PR + review. Use One
Shot when task splitting is overhead. `--lite`/`--oneshot` mark the pre-report ledger skipped,
then enforce readiness via QA/review/PR/report.

**Main Quest** (`--full`): full ordered phase lifecycle `0 → 1 → 2' → 3.5 → 4 → 5 → 5.5 → 5.6 →
6 → 7`, with state/plans/handoffs/tasks/reviews/reports in `.codedungeon/` for resume. Phase 5
needs approved review evidence + pushed PR; Phase 6 a passing QA ledger; Phase 7 is closed by
`run finalize`. `qa detect-framework --path .` handles single projects and monorepos. Playwright
is an external dep: if E2E needs it and it's absent, QA returns `BLOCKED` with an install hint
(not a code failure).

Final report (rendered by `run finalize`, never by hand) summarizes Status, Workflow, PR, Branch,
Review verdict, Cycles, work done, verification, telemetry, and the next action.
