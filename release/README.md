# CodeDungeon Release

Precompiled provider binaries for Claude Code and Codex CLI. The provider is built into
the binary — choose by name (`codedungeon-claude` / `codedungeon-codex`) rather than relying
on `CODEDUNGEON_PROVIDER`. See [`../README.md`](../README.md) for what CodeDungeon is and how
the workflow runs; this file covers install + provider layout only.

## Contents

```text
release/
  get-codedungeon.sh   install.sh   install.ps1   QUICKSTART.md
  bin/   codedungeon-{claude,codex}[.exe|-darwin-amd64|-darwin-arm64]
  skills/grimoire-cli/SKILL.md
```

## Install

```bash
./install.sh --provider claude              # or codex; --target /path optional
# Windows: .\install.ps1 -Provider claude
```

`claude-code` / `claude-ce` normalize to `claude`. The installer copies the chosen binary to
`.codedungeon/bin/codedungeon-<provider>` and runs project-local setup — no PATH, home-plugin,
or global-flag changes.

Single-binary download:

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon claude
./codedungeon-claude setup
```

## What setup installs

| Provider | Pack | Router |
|----------|------|--------|
| claude | `.claude/bin`, `.claude/{agents,skills,commands}` | `/codedungeon` |
| codex | `.codex/bin`, `.codex/agents`, `.codex/config.toml` (multi_agent_v2), `.agents/skills` | `$codedungeon` |

Both share mutable state in `.codedungeon/` (SQLite DB, editable commands/phases, tasks,
plans, reviews, QA/execution sessions, reports, Project Rules, artifact registry). Setup
returns an `agent_config_instruction` block to insert in `CLAUDE.md` / `AGENTS.md`.

Aliases installed: `/one-shot`, `/side-quest`, `/main-quest`, `/task-maker`, `/code-review`
(Codex: `$…`).

## Gates

PR-centered, verification-gated, no local-only completion. `READY_FOR_USER_REVIEW` requires
QA evidence, a pushed branch, an open PR, posted `code-review`, passing `git verify`, and
`run finalize`. The user reviews and merges. See [`../README.md`](../README.md#completion-gates).

## Upgrade

Replace `release/`, rerun the installer, then `codedungeon-<provider> migrate` in existing
projects. Safe-upgrade detail: [`../docs/MIGRATING.md`](../docs/MIGRATING.md).
