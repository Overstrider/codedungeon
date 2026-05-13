# CodeDungeon v2.0.0 Quickstart

Use the provider-specific binary:

- `codedungeon-codex` for Codex CLI.
- `codedungeon-claude` for Claude Code.

CodeDungeon gives the agent a provider-native workflow router plus standalone
modules for rules, planning, execution, QA, review, PR gates, artifacts, and
reports. Runtime state and evidence live in `.codedungeon/`.

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

Claude:

```bash
codedungeon-claude setup --yes
```

Setup is project-local. It does not install user-home plugins, global feature
flags, or shell PATH changes.

Codex setup installs `.codex/bin/codedungeon`, `.codex/agents/*`,
`.codex/config.toml`, `.agents/skills/*`, and shared `.codedungeon/*` state.
It returns `agent_config_instruction` content for the installer agent to insert
in `AGENTS.md`.

Claude setup installs `.claude/bin/codedungeon`, `.claude/agents/*`,
`.claude/skills/*` workflow surfaces, `.claude/commands/*` wrappers, and shared
`.codedungeon/*` state. It returns `agent_config_instruction` content for
`CLAUDE.md`.

## First Run

Run Project Rules discovery before the first real task:

```text
$codedungeon --rules
# or
/codedungeon --rules
```

Project Rules discovery deep-reads the repo, drafts
`.codedungeon/project-rules.md`, asks for user confirmation, then generates
`.codedungeon/project-rules.compact.md`. Full and lite workflows read approved
compact rules before planning, executing, reviewing, or reporting completion.

Optional pre-run helper:

```text
$task-maker
# or
/task-maker
```

Task Maker clarifies a rough request, writes a minimal design and run prompt
under `.codedungeon/task-maker/sessions/<session>/`, and prints a reviewed
`$codedungeon --full "<prompt>"` or `/codedungeon --full "<prompt>"` command
without starting the workflow automatically.

## Requirements

- A git repository.
- A GitHub `origin` remote.
- `gh` authenticated for PR workflows.

## Workflows

Codex:

```text
$codedungeon --oneshot "Small scoped change"
$codedungeon --lite "Execute .codedungeon/plans/example.md"
$codedungeon --full "Complex feature with tests"
$codedungeon "Let the router choose"
```

Claude:

```text
/codedungeon --oneshot "Small scoped change"
/codedungeon --lite "Execute .codedungeon/plans/example.md"
/codedungeon --full "Complex feature with tests"
/codedungeon "Let the router choose"
```

Modes:

- `--oneshot`: small scoped change; creates or switches to a feature branch,
  runs QA, opens or reuses a PR, runs standalone code-review, and finalizes.
- `--lite`: compact single-repo task workflow. Requires a prior plan in
  `.codedungeon/plans/*.md` or an explicit plan path.
- `--full`: full phase workflow for complex or multi-repo work.
- `--auto` or no flag: router-selected mode with
  `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>`.
- `--rules`: Project Rules discovery.

Compatibility aliases remain available:

- Codex: `$one-shot`, `$side-quest`, `$main-quest`
- Claude: `/one-shot`, `/side-quest`, `/main-quest`

## Completion Gates

CodeDungeon completion is PR-centered and verification-gated. There is no
local-only completion path. `READY_FOR_USER_REVIEW` requires:

- QA evidence from `codedungeon qa run`.
- A pushed branch.
- An open PR.
- Standalone `codedungeon code-review --post` evidence.
- Passing `codedungeon git verify`.
- Final report rendering through `codedungeon run finalize`.

Review approval does not replace verification. The user performs final review
and merge.

## Useful Commands

```bash
codedungeon-codex setup --yes
codedungeon-codex migrate
codedungeon-codex status
codedungeon-codex rules status
codedungeon-codex task-maker render --surface codex --input <request.json> --out <dir> --print
codedungeon-codex plan run --prompt "<prompt>" --project-context .codedungeon/project-rules.compact.md
codedungeon-codex execute task --task <task.json> --project-context .codedungeon/project-rules.compact.md
codedungeon-codex qa run --auto --fresh
codedungeon-codex qa status --latest
codedungeon-codex code-review --url <pr-url> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/plan/PLAN.md --post
codedungeon-codex artifacts verify --latest-run
codedungeon-codex observe report
codedungeon-codex run finalize --dry-run
```

The same CLI surface exists through `codedungeon-claude`.

Existing projects should run `codedungeon-<provider> migrate` after upgrading
the binary. See [`../docs/MIGRATING.md`](../docs/MIGRATING.md) for the safe
migration flow.
