# codedungeon — release

Pre-compiled binaries + Claude Code skill. Drop this folder anywhere, run `./install.sh`, done.

## Contents

```
release/
├── install.sh                   # one-shot installer (auto-detect OS)
├── bin/
│   ├── codedungeon              # linux-amd64
│   ├── codedungeon.exe          # windows-amd64
│   ├── codedungeon-darwin-amd64 # macOS Intel
│   └── codedungeon-darwin-arm64 # macOS Apple Silicon
└── skills/
    └── codedungeon-cli/
        └── SKILL.md             # Claude Code skill definition
```

## Install

```bash
./install.sh
```

What it does:

1. Detects your OS/arch, picks the right binary.
2. Copies it to `~/.local/bin/codedungeon`.
3. Creates plugin at `~/.claude/plugins/local/codedungeon/` with `.claude-plugin/plugin.json`, `bin/codedungeon`, and `skills/codedungeon-cli/SKILL.md`.
4. Claude Code discovers the plugin automatically (skill + slash-commands become available).

## First use (per project)

```bash
cd /path/to/your/git/project
codedungeon bootstrap --reasoning claude-opus-4-7 --fast claude-sonnet-4-6
```

`bootstrap` is **project-scoped**:
- Refuses to run inside `~/.claude` or `/root/.claude`.
- Requires `.git/` at the target.
- Self-copies binary to `<project>/.claude/bin/codedungeon`.
- Creates `<project>/.claude/codedungeon.db` (SQLite + FTS5).
- Installs embedded agents/skills/commands/phases into `<project>/.claude/`.
- Records OS, project_root, binary version, model tiers.

## Hard refusals (structured errors)

All failures return JSON with `{"error","hint","action"}`. Agents parse `action` to recover:

| action | meaning |
|---|---|
| `change-directory` | CWD is under `~/.claude` — move out. |
| `init-git-or-bootstrap` | No `.git/` — run `git init` or bootstrap with `--target`. |
| `ask-user-models` | `--reasoning` / `--fast` missing on bootstrap. |
| `run-codedungeon-migrate` | Binary version ≠ DB `cd_version` — run `codedungeon migrate`. |

## Slash commands (after install + bootstrap)

- `/codedungeon-dev-cycle "<feature>"` — full pipeline (10 phases).
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
