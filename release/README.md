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

- `codedungeon-codex` installs `.codex/*`, `.agents/skills/*`, and `AGENTS.md`.
- `codedungeon-claude` installs `.claude/*`, the Claude plugin, and `CLAUDE.md`.

Codex workflows are skills such as `$minidungeon`, `$codedungeon-dev-cycle`, and `$code-review`.
Claude workflows are plugin slash commands such as `/minidungeon`, `/codedungeon-dev-cycle`, and `/code-review`.

## Upgrade

Replace the `release/` directory, rerun the provider installer, then run `codedungeon-<provider> migrate` inside existing projects when prompted.
