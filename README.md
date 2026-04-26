# codedungeon

Deterministic Go CLI for AI coding-agent project pipelines. It moves repeatable workflow mechanics into one binary: state, phase handoffs, prompt/agent/skill installation, repo discovery, review dedupe, QA assertions, task tracking, migrations, and final reports. The LLM keeps the judgment work.

Provider support is first-class:

- `codedungeon-codex` for Codex CLI.
- `codedungeon-claude` for Claude Code.
- `claude` is the canonical provider name. `claude-code` and `claude-ce` are accepted only as legacy aliases.

The binary is project-scoped, requires a git repo, and stores mutable state in `.codedungeon/`:

- Shared runtime: `.codedungeon/codedungeon.db`, `.codedungeon/commands`, `.codedungeon/phases`, `.codedungeon/tasks`, `.codedungeon/plan`, `.codedungeon/state`, `.codedungeon/reviews`, `.codedungeon/memory`.
- Codex-native bootstrap: `.codex/agents`, `.codex/config.toml` with `multi_agent_v2`, global Codex `multi_agent_v2` enablement when setup is not run with `--skip-global`, `AGENTS.md`, `.agents/skills/*`.
- Claude-native bootstrap: `.claude/agents`, `.claude/commands` wrappers, `.claude/skills`, `.claude/bin`, `CLAUDE.md`, optional global Claude plugin.

`.codedungeon/` is intended to be readable, editable, and versionable so interrupted runs can be resumed and PR history can be investigated later.

## Install

From a checked-out repo:

```bash
./install.sh --provider codex
./install.sh --provider claude
```

On Windows PowerShell:

```powershell
.\install.ps1 -Provider codex
.\install.ps1 -Provider claude
```

The root installers delegate to `release/install.sh` and `release/install.ps1`. Provider selection is required so users do not accidentally install the Claude workflow when they wanted Codex.

For direct download from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon codex
./codedungeon-codex setup
```

## First Run

Inside a git project:

```bash
codedungeon-codex setup
# or
codedungeon-claude setup
```

`setup` creates `.codedungeon`, copies the local project binary, installs provider-native agents/skills/config, installs editable command playbooks and phase prompts under `.codedungeon`, and writes the project instruction file.

Codex setup installs `.codex/*`, `.agents/skills/*`, `.codedungeon/*`, and `AGENTS.md`. Claude setup installs `.claude/*`, `.codedungeon/*`, `CLAUDE.md`, and the Claude plugin when available.

Existing projects should use `codedungeon-<provider> migrate` after upgrading the binary. See [`docs/MIGRATING.md`](docs/MIGRATING.md) for the safe upgrade flow and what migration preserves.

## Build

```bash
make test
make build
make release
```

Release builds produce provider-specific binaries:

- `release/bin/codedungeon-codex`
- `release/bin/codedungeon-claude`
- `release/bin/codedungeon-codex.exe`
- `release/bin/codedungeon-claude.exe`
- `release/bin/codedungeon-*-darwin-*`

Each provider binary embeds its default provider via Go ldflags. Normal users should not rely on `CODEDUNGEON_PROVIDER`.

## Architecture

Source of truth lives in `src/codedungeon/`:

- `cmd/`: Cobra commands for setup, bootstrap, phase state, repo discovery, review, plans, QA, reports, install/migrate/status.
- `internal/provider/`: provider abstraction. Current providers are `claude` and `codex`.
- `internal/prompts/`: embedded provider prompt packs. Claude uses `files/`; Codex uses `codex-files/`.
- `internal/db/`: SQLite schema, migrations, FTS5, and installed artifact tracking.
- `release/`: shippable installers, docs, skills, and binaries.

The shared lifecycle is provider-agnostic. Provider-specific differences live at the edges: paths, project instruction file, command/skill surfaces, agent format, models, and plugin installation.

## Provider Packs

Artifacts are versioned and tracked in the project DB with:

- `provider`
- `pack_id`
- `pack_version`
- `install_path`
- `kind`
- `logical_name`
- `sha256`
- `user_modified`

This lets `codedungeon status`, `install`, and `migrate` reason about Codex and Claude packs independently.

## Useful Commands

```bash
codedungeon-codex setup
codedungeon-codex status
codedungeon-codex phase info
codedungeon-codex spawn-prompt 5
codedungeon-codex prompts list
codedungeon-codex migrate
```

The same command surface exists for Claude via `codedungeon-claude`.

After setup, agent-facing workflows are `/one-shot`, `/side-quest`, and `/main-quest` in Claude Code, or `$one-shot`, `$side-quest`, and `$main-quest` in Codex. See [`docs/WORKFLOWS.md`](docs/WORKFLOWS.md) for when to use each workflow and how branch/PR handling works.

CodeDungeon completion is PR-centered: code-writing workflows are only `COMPLETE` after the branch is pushed, a PR exists, adversarial review is posted to the PR, and the final verdict is `APPROVED`. Terminal output uses the standard CodeDungeon PR Report.

## Docs

- `src/codedungeon/docs/ARCHITECTURE.md`: current architecture.
- `src/codedungeon/docs/PROVIDERS.md`: provider model and how to add another provider.
- `docs/WORKFLOWS.md`: workflow names, provider invocation, and One Shot / Side Quest / Main Quest behavior.
- `docs/MIGRATING.md`: safe upgrade and migration guide for existing projects.
- `release/README.md`: release/install guide.
- `release/QUICKSTART.md`: end-user quickstart.

## License

Apache-2.0.
