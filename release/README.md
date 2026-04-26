# codedungeon release

Precompiled provider-specific binaries for Codex CLI and Claude Code.

## Contents

```text
release/
  get-codedungeon.sh
  install.sh
  install.ps1
  QUICKSTART.md
  bin/
    codedungeon-codex
    codedungeon-claude
    codedungeon-codex.exe
    codedungeon-claude.exe
    codedungeon-*-darwin-*
  skills/
    grimoire-cli/SKILL.md
```

## Install

Bash:

```bash
./install.sh --provider codex
./install.sh --provider claude
```

PowerShell:

```powershell
.\install.ps1 -Provider codex
.\install.ps1 -Provider claude
```

Provider selection is required. `claude-code` and `claude-ce` are accepted as legacy aliases and normalize to `claude`.

## Download One Binary

Codex:

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon codex
./codedungeon-codex setup
```

Claude:

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon claude
./codedungeon-claude setup
```

## Provider Behavior

The provider is built into the binary. Normal use should not depend on `CODEDUNGEON_PROVIDER`.

- `codedungeon-codex` installs `.codex/*`, `.agents/skills/*`, `.codedungeon/*`, and `AGENTS.md`.
- `codedungeon-claude` installs `.claude/*`, `.codedungeon/*`, the Claude plugin, and `CLAUDE.md`.

Mutable runtime state lives in `.codedungeon/`: SQLite DB, editable commands, phases, tasks, plans, state handoffs, reviews, reports, and PR memory. Provider directories keep only provider-native bootstrap files and Claude slash-command wrappers.

Codex workflows are skills such as `$one-shot`, `$side-quest`, `$main-quest`, and `$code-review`.
Claude workflows are plugin slash commands such as `/one-shot`, `/side-quest`, `/main-quest`, and `/code-review`.

Workflow guide:

- `one-shot`: smallest PR-producing workflow; creates or switches to `feat/<slug>`, then runs guard, commits, pushes, creates or reuses a PR, and runs review.
- `side-quest`: compact task-splitting workflow for simple single-repo work.
- `main-quest`: full phase lifecycle for complex or multi-repo work.

`one-shot` runs `codedungeon git guard` only after switching to a feature branch because guard rejects protected branches such as `main`.

## Upgrade

Replace the `release/` directory, rerun the provider installer, then run `codedungeon-<provider> migrate` inside existing projects when prompted.

For the full safe upgrade flow, including what is preserved and when to use `install --dry-run` or `install --force`, see [`../docs/MIGRATING.md`](../docs/MIGRATING.md).
