---
name: grimoire-cli
description: codedungeon is a project-scoped deterministic Go CLI for the codedungeon pipeline. M2M-only. Requires a git project. Refuses to run under ~/.claude or /root/.claude. First invocation auto-bootstraps a copy into <project>/.claude/bin/. Use for phase lifecycle, handoffs, repo discovery, review pipeline, plan parsing, git/gh wrappers, QA validation, report rendering, cleanup, and prompt retrieval. Call codedungeon instead of narrating deterministic algorithms in prose.
---

# codedungeon

Single Go binary. SQLite (FTS5) backend + embedded prompts. Project-only.

**Two binary locations** (identical content):
- Shipped master: `$HOME/.claude/plugins/local/codedungeon/bin/codedungeon`
- Project-local (after bootstrap): `<project>/.claude/bin/codedungeon`

**State file**: `<project>/.codedungeon/codedungeon.db` (per-project SQLite, FTS5-indexed). Markdown (`pipeline-state.md`, `phase-{N}-output.md`) are rendered VIEWS.

## First-run bootstrap (agent responsibility)

Before any other command, make sure codedungeon is alive in this project:

```bash
if [ -x .claude/bin/codedungeon ] && [ -f .codedungeon/codedungeon.db ]; then
  CD=./.claude/bin/codedungeon
else
  # Requires .git at project root.
  [ -d .git ] || [ -f .git ] || { echo '{"error":"no-git","hint":"git init first"}'; exit 2; }
  "$HOME/.claude/plugins/local/codedungeon/bin/codedungeon" bootstrap
  CD=./.claude/bin/codedungeon
fi
```

**Hard refusals** (exit 1 with JSON `{"error":...,"hint":...,"action":...}`):
- `refuse: ... ~/.claude ...` → CWD is under a home .claude (user or root). cd out.
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

# 2) Discover repos (auto-populates REPO_MAP, upserts CLAUDE.md)
codedungeon repo discover --write-claude-md --persist

# 3) Per-phase agent
codedungeon phase next
codedungeon phase config feature
codedungeon prompts get caveman-ultra
# ... do work (LLM judgment) ...
codedungeon phase done 5 \
  --verdict APPROVED \
  --summary "..." --decisions "..." --artifacts "..." \
  --next "..." --promise "PHASE_5_COMPLETE: ..."

# 4) Orchestrator
codedungeon phase render-state

# 5) Final leaves the PR open for human review/merge
codedungeon report render
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
