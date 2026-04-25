# codedungeon Quickstart

Use the provider-specific binary:

- `codedungeon-codex` for Codex CLI.
- `codedungeon-claude` for Claude Code.

## Install

Bash:

```bash
./install.sh --provider codex
```

PowerShell:

```powershell
.\install.ps1 -Provider codex
```

Use `claude` instead of `codex` for Claude Code. Provider selection is required.

## Setup In A Project

```bash
cd /path/to/git/project
codedungeon-codex setup
```

Non-interactive:

```bash
codedungeon-codex setup --yes
```

Codex setup:

1. Copies the project binary to `.codex/bin/codedungeon`.
2. Creates `.codex/codedungeon.db`.
3. Installs Codex agents, commands, phases, and config under `.codex/`.
4. Installs workflow/domain skills under `.agents/skills/`.
5. Writes a codedungeon section to `AGENTS.md`.

Claude setup:

```bash
codedungeon-claude setup
```

Claude setup installs `.claude/*`, `CLAUDE.md`, and the global Claude plugin.

## Requirements

- A git repository.
- `gh` authenticated for PR workflows.

## Codex Workflows

After Codex setup, invoke workflows as skills:

- `$minidungeon`
- `$codedungeon-dev-cycle`
- `$code-review`
- `$codedungeon-test-loop`
- `$cleanup-tasks`

`.codex/commands/` contains reference playbooks, not Codex CLI slash commands.

## Useful Commands

```bash
codedungeon-codex setup
codedungeon-codex status
codedungeon-codex phase info
codedungeon-codex spawn-prompt 5
codedungeon-codex prompts list
codedungeon-codex migrate
```

The same command surface exists through `codedungeon-claude`.
