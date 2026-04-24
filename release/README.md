# codedungeon — release

Pre-compiled binaries + Claude Code pipeline. Two install paths: **quick setup** (recommended) or legacy installer.

## Contents

```
release/
├── get-codedungeon.sh           # remote bootstrap (download binary + guide)
├── install.sh                   # legacy installer (local folder)
├── QUICKSTART.md                # agent-friendly setup + usage guide
├── bin/
│   ├── codedungeon              # linux-amd64
│   ├── codedungeon.exe          # windows-amd64
│   ├── codedungeon-darwin-amd64 # macOS Intel
│   └── codedungeon-darwin-arm64 # macOS Apple Silicon
└── skills/
    └── grimoire-cli/
        └── SKILL.md             # Claude Code skill definition
```

## Quick Setup (recommended)

### Option A: Remote — one command from GitHub

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon
```

Downloads the binary + `QUICKSTART.md` into CWD. Then:

```bash
./codedungeon setup
```

### Option B: Give to any AI agent

Pass this to any agent (Claude Code, Cursor, Copilot, Windsurf, etc.):

```
Download and run:
  bash get-codedungeon.sh https://github.com/Overstrider/codedungeon
Then read QUICKSTART.md and run: ./codedungeon setup --yes
```

The agent downloads the binary, reads the guide, and knows how to use `/minidungeon`, `/codedungeon-dev-cycle`, and `/code-review`.

### Option C: Local folder

```bash
./install.sh
```

Detects OS/arch, copies binary to `~/.local/bin/`, creates Claude Code plugin.

## First use (per project)

```bash
cd /path/to/your/git/project
codedungeon setup          # interactive — picks models, installs everything
codedungeon setup --yes    # non-interactive — uses defaults (Opus + Sonnet)
```

`setup` does everything in one step:
- Detects OS, project stack (Go/Rust/Next.js/Kotlin/Python/Elixir/C++).
- Installs global Claude Code plugin (`~/.claude/plugins/local/codedungeon/`).
- Self-copies binary to `<project>/.claude/bin/codedungeon`.
- Creates `<project>/.claude/codedungeon.db` (SQLite + FTS5).
- Installs 58 embedded artifacts (agents, skills, commands, phases) into `<project>/.claude/`.
- Writes codedungeon section to `CLAUDE.md` (slash command reference).
- Records OS, project_root, binary version, model tiers.

## Hard refusals (structured errors)

All failures return JSON with `{"error","hint","action"}`. Agents parse `action` to recover:

| action | meaning |
|---|---|
| `change-directory` | CWD is under `~/.claude` — move out. |
| `init-git-or-bootstrap` | No `.git/` — run `git init` or bootstrap with `--target`. |
| `ask-user-models` | `--reasoning` / `--fast` missing on bootstrap. |
| `run-codedungeon-migrate` | Binary version ≠ DB `cd_version` — run `codedungeon migrate`. |

## Slash commands (after setup)

- `/minidungeon` — lightweight pipeline: plan mode → split → execute → review → PR. Single-repo, no tests.
- `/codedungeon-dev-cycle "<feature>"` — full pipeline (10 phases, multi-repo, architect, QA, tests, report).
- `/codedungeon-loop <task-dir>` — dev execution loop (specialist plan + exec + review).
- `/codedungeon-test-loop <task-dir>` — QA loop (startup + integration + API + E2E).
- `/code-review [repo]` — adversarial Opus 4.7 fanout + Sonnet validators.
- `/cleanup-tasks` — remove stale `.claude/` artifacts.

## CLI domains (direct invocation)

```
codedungeon bootstrap       # first-run
codedungeon phase           # lifecycle (init/start/done/skip/fail/next/info/config)
codedungeon repo            # discover, resolve, check-test-auth
codedungeon review          # dedupe + filter + classify + render + verdict
codedungeon plan            # meta + append-fix-tasks
codedungeon git             # guard, pr, verify, diff
codedungeon qa              # validate-api, detect-framework
codedungeon report          # render
codedungeon cleanup         # remove stale artifacts
codedungeon prompts         # get/list/set/diff embedded+DB versioned
codedungeon db              # init, migrate, search, export
codedungeon config          # models/model/set-models
codedungeon install         # install embedded tree
codedungeon migrate         # version-delta install
codedungeon status          # installed_artifacts drift check
codedungeon version         # binary + schema + runtime info
```

## Uninstall

```bash
rm -f ~/.local/bin/codedungeon
rm -rf ~/.claude/plugins/local/codedungeon
# Per-project: the bootstrapped .claude/ stays untouched (user owns it).
```

## Upgrading

Drop a newer `release/` over the old one, re-run `./install.sh`. Bootstrapped projects auto-prompt for `codedungeon migrate` on the next invocation (version mismatch guard).

## License

Apache-2.0.
