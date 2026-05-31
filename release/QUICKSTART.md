# CodeDungeon Quickstart

Provider binaries: `codedungeon-claude` (Claude Code), `codedungeon-codex` (Codex CLI).
Surfaces: `/codedungeon` (Claude), `$codedungeon` (Codex). Runtime state lives in `.codedungeon/`.

## Install + setup

```bash
./install.sh --provider claude        # Windows: .\install.ps1 -Provider claude
cd /path/to/git/project
codedungeon-claude setup --yes        # project-local; no home/global changes
```

Setup installs the provider pack (`.claude/*` or `.codex/*`) + shared `.codedungeon/*`,
and returns `agent_config_instruction` to insert in `CLAUDE.md` / `AGENTS.md`.

## First run

```text
/codedungeon --rules               # discover & approve Project Rules (do this first)
/codedungeon --full "build X"      # or --lite / --oneshot / no-flag (router picks)
```

`--rules` drafts `.codedungeon/project-rules.md`, asks for approval, then compacts to
`.codedungeon/project-rules.compact.md`. Optional: `/task-maker` clarifies a rough request
and prints a ready `--full` command without starting it.

Modes: `--oneshot` (small, no split), `--lite` (planned, single-repo, needs a plan in
`.codedungeon/plans/`), `--full` (complex/multi-repo), `--auto` (router). Aliases:
`/one-shot`, `/side-quest`, `/main-quest` (Codex: `$…`).

## Requirements

Git repo, `origin` remote, `gh` authenticated for PR workflows.

## Completion

PR-centered and verification-gated — no local-only completion. `READY_FOR_USER_REVIEW`
needs QA evidence, a pushed branch, an open PR, posted `code-review`, passing `git verify`,
and `run finalize`. CodeDungeon never merges; the user reviews and merges.

## Common commands

```bash
codedungeon-claude setup --yes
codedungeon-claude migrate                 # after upgrading the binary
codedungeon-claude status | diagnose | rules status
codedungeon-claude run --full --prompt "<prompt>"
codedungeon-claude qa run --auto --fresh
codedungeon-claude code-review --url <pr-url> --post
codedungeon-claude run finalize --dry-run
```

See [`../README.md`](../README.md) for the full picture, [`../docs/WORKFLOWS.md`](../docs/WORKFLOWS.md)
for mode/gate detail, and [`../docs/MIGRATING.md`](../docs/MIGRATING.md) for upgrades.
The authoritative workflow contract is `codedungeon kernel`.
