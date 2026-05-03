# CodeDungeon v2.0.0 Release

Precompiled provider-specific binaries for Codex CLI and Claude Code.

CodeDungeon installs a provider-native workflow router, standalone modules, and
durable `.codedungeon/` state. The promoted agent-facing entry points are
`$codedungeon` for Codex and `/codedungeon` for Claude Code.

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
./install.sh --provider codex --target /path/to/project
```

PowerShell:

```powershell
.\install.ps1 -Provider codex
.\install.ps1 -Provider claude
.\install.ps1 -Provider codex -Target C:\path\to\project
```

Provider selection is required. `claude-code` and `claude-ce` are accepted as
legacy aliases and normalize to `claude`.

The release installer copies the selected provider binary to
`.codedungeon/bin/codedungeon-<provider>` and then runs project-local setup. It
does not modify shell PATH, user-home plugins, or global provider feature flags.

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

The provider is built into the binary. Normal use should choose by binary name
instead of depending on `CODEDUNGEON_PROVIDER`.

Codex setup installs:

- `.codex/bin/codedungeon`
- `.codex/agents/*`
- `.codex/config.toml` with project-local `multi_agent_v2` config
- `.agents/skills/*`
- `AGENTS.md`
- `.codedungeon/commands/*`, `.codedungeon/phases/*`, and runtime state

Claude setup installs:

- `.claude/bin/codedungeon`
- `.claude/agents/*`
- `.claude/skills/*`
- `.claude/commands/*` wrappers
- `CLAUDE.md`
- `.codedungeon/commands/*`, `.codedungeon/phases/*`, and runtime state

Mutable runtime state lives in `.codedungeon/`: SQLite DB, editable commands,
phases, tasks, plans, state handoffs, reviews, code-review evidence, QA sessions,
execution sessions, reports, memory, PR evidence, Project Rules files, and the
runtime artifact registry.

## Workflow Router

Promoted provider-native surfaces:

```text
$codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>
/codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>
```

Without a mode flag, the router selects automatically and prints:

```text
CODEDUNGEON_MODE_SELECTED: <mode> - <reason>
```

Mode summary:

- `--rules`: deep-read the repo, draft `.codedungeon/project-rules.md`, wait for
  user confirmation, then approve and compact rules.
- `--oneshot`: smallest PR-producing workflow for a scoped change.
- `--lite`: compact task workflow for simple work with a prior plan.
- `--full`: full phase lifecycle for complex or multi-repo work.
- `--auto`: explicit automatic mode selection.

Compatibility aliases remain installed:

- Codex: `$one-shot`, `$side-quest`, `$main-quest`
- Claude: `/one-shot`, `/side-quest`, `/main-quest`

Task Maker is installed as `$task-maker` or `/task-maker`. It clarifies a rough
request, persists `request.json`, `design.md`, `prompt.txt`, and `output.md`
under `.codedungeon/task-maker/sessions/<session>/`, and prints a reviewed
`$codedungeon --full "<prompt>"` or `/codedungeon --full "<prompt>"` command
without starting the workflow automatically.

Standalone review is `$code-review` or `/code-review`; the underlying CLI module
is:

```bash
codedungeon-<provider> code-review --url <pr-url> --project-context <path> --task-context <path> --out .codedungeon/code-review --post
```

## Gates

Code-writing workflows are PR-centered and verification-gated. They require a
GitHub `origin` remote and authenticated `gh`; there is no local-only completion
path. A workflow reaches `READY_FOR_USER_REVIEW` only after:

- QA records passing verification evidence.
- The branch is pushed.
- A PR exists and remains open.
- Standalone CodeDungeon code-review records and posts review evidence.
- `codedungeon git verify` accepts the PR/review state.
- `codedungeon run finalize` renders the final report from DB evidence.

The user performs final review and merge.

## Useful Module Commands

```bash
codedungeon-<provider> setup --yes
codedungeon-<provider> migrate
codedungeon-<provider> status
codedungeon-<provider> rules status
codedungeon-<provider> rules lint
codedungeon-<provider> task-maker render --surface codex --input <request.json> --out <dir> --print
codedungeon-<provider> plan run --prompt "<prompt>" --project-context .codedungeon/project-rules.compact.md
codedungeon-<provider> execute task --task <task.json> --project-context .codedungeon/project-rules.compact.md
codedungeon-<provider> qa run --auto --fresh
codedungeon-<provider> code-review --url <pr-url> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/plan/PLAN.md --post
codedungeon-<provider> artifacts list --latest-run
codedungeon-<provider> artifacts verify --latest-run
codedungeon-<provider> observe report
codedungeon-<provider> run finalize --dry-run
```

See `../README.md` for the complete module reference and
`../docs/WORKFLOWS.md` for workflow behavior.

## Upgrade

Replace the `release/` directory, rerun the provider installer, then run
`codedungeon-<provider> migrate` inside existing projects when prompted.

For the full safe upgrade flow, including what is preserved and when to use
`install --dry-run` or `install --force`, see
[`../docs/MIGRATING.md`](../docs/MIGRATING.md).
