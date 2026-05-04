---
name: grimoire-cli
description: codedungeon is a project-scoped deterministic Go CLI for the codedungeon pipeline. M2M-only. Requires a git project. Refuses to run inside provider home config directories. First invocation auto-bootstraps a copy into <project>/.claude/bin/. Use for phase lifecycle, handoffs, repo discovery, review pipeline, plan parsing, git/gh wrappers, QA validation, report rendering, cleanup, and prompt retrieval. Call codedungeon instead of narrating deterministic algorithms in prose.
---

# codedungeon

Single Go binary. SQLite (FTS5) backend + embedded prompts. Project-only.

**Binary location**:
- Project-local: `<project>/.claude/bin/codedungeon`

**State file**: `<project>/.codedungeon/codedungeon.db` (per-project SQLite, FTS5-indexed). Markdown (`pipeline-state.md`, `phase-{N}-output.md`) are rendered VIEWS.

## First-run bootstrap (agent responsibility)

Before any other command, make sure codedungeon is alive in this project:

```bash
if [ -x .claude/bin/codedungeon ] && [ -f .codedungeon/codedungeon.db ]; then
  CD=./.claude/bin/codedungeon
else
  echo '{"error":"codedungeon-not-setup","hint":"run project-local codedungeon setup from the intended project directory"}'
  exit 2
fi
```

Deterministic completion gates:
- Use only `./.claude/bin/codedungeon` for CodeDungeon commands.
- Do not write review reports manually.
- Do not write final reports manually.
- Run standalone review with `./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post`.
- Run verification with `./.claude/bin/codedungeon qa run --phase 6 --fresh`.
- Run `./.claude/bin/codedungeon run finalize`; READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`.

**Hard refusals** (exit 1 with JSON `{"error":...,"hint":...,"action":...}`):
- `refuse: ... provider home config ...` → CWD is under a user-global provider config directory. cd out.
- `refuse: ... no .git ...` → project has no git repo. `git init` first.

## Domains

```
bootstrap  first-run self-copy + init DB (requires --target or CWD with .git)
db         init, migrate, search, export
phase      init, start, done, skip, fail, next, info, config, render-state
prompts    get, list, set, diff
repo       discover, resolve, check-test-auth
review     run, post
plan       (Sprint 4) meta, append-fix-tasks
git        guard, pr, verify, diff
qa         validate-api, detect-framework, run
report     render
cleanup    (Sprint 5)
```

## Usage

```bash
codedungeon <domain> <verb> [flags]
codedungeon <domain> --help
codedungeon --version         # also JSON
```

Default output: JSON on stdout. Logs on stderr. Exit 0/1/2.

## Canonical pipeline flow

```bash
# 0) Bootstrap (once per project)
codedungeon bootstrap

# 1) Start an autonomous pipeline
codedungeon run --full --prompt "add X"

# 2) Observe or recover only; the autonomous child owns repo discovery,
# phases, QA evidence, review posting, and final report rendering.
codedungeon run status
codedungeon run unlock --reason "stale/crashed session"  # only for a confirmed stale session
```

## Design rule

- **structured → structured** (parse, merge, template, bookkeeping) → `codedungeon`.
- **text → judgment → text** (architect, bug-hunt, classify) → LLM.

## OS adaptation

Binary detects runtime OS (linux/windows/darwin) and uses OS-specific subprocess behavior (bash vs cmd, `.exe` resolution, well-known tool paths like `/c/tools/gh` on WSL2). Stored in `meta.os` on bootstrap. All OS-specific code lives in `internal/osadapter/` — one implementation per OS, selected by Go build tags. No `if runtime.GOOS` scattered across the codebase.

## FTS5 search (historical context)

```bash
codedungeon db search "TOCTOU" --table findings
codedungeon db search "test auth" --table tasks
codedungeon db search "caveman" --table prompts
codedungeon db search "arcplan" --table handoffs
```

## Failure modes

All errors are JSON. Agents should parse and act:
- `{"action":"change-directory"}` → cd to a real project.
- `{"action":"init-git-or-bootstrap"}` → `git init` or invoke bootstrap with target.
- `{"action":"bootstrap"}` → invoke `codedungeon bootstrap --target <path>`.

Never rewrite codedungeon logic inline if the binary is missing — install it or fail loud.
