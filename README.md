# codedungeon

Deterministic Go CLI for AI coding-agent project pipelines. It moves repeatable workflow mechanics into one binary: state, phase handoffs, prompt/agent/skill installation, repo discovery, review dedupe, QA assertions, task tracking, migrations, and final reports. The LLM keeps the judgment work.

Provider support is first-class:

- `codedungeon-codex` for Codex CLI.
- `codedungeon-claude` for Claude Code.
- `claude` is the canonical provider name. `claude-code` and `claude-ce` are accepted only as legacy aliases.

The binary is project-scoped, requires a git repo, and stores mutable state in `.codedungeon/`:

- Shared runtime: `.codedungeon/codedungeon.db`, `.codedungeon/commands`, `.codedungeon/phases`, `.codedungeon/tasks`, `.codedungeon/plan`, `.codedungeon/state`, `.codedungeon/reviews`, `.codedungeon/memory`, `.codedungeon/project-rules.md`, `.codedungeon/project-rules.compact.md`, `.codedungeon/project-rules.json`.
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

Recommended next step after setup is Project Rules discovery:

```text
$codedungeon --rules
# or
/codedungeon --rules
```

The agent deep-reads the repo, drafts `.codedungeon/project-rules.md`, waits for user confirmation, then uses `codedungeon rules approve` and `codedungeon rules compact` to generate `.codedungeon/project-rules.compact.md`. Workflows read the compact rules and include `PROJECT_RULES_STATUS`, `PROJECT_RULES_DIGEST`, and `PROJECT_RULES_READ` in handoffs.

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

## Maintainer Rules

Repository maintenance is main-only. Agents must not create branches or git worktrees. Every finished change must update relevant docs (`README.md`, `AGENTS.md`, `CLAUDE.md`, and `docs/*`), update installers only when install behavior changes, run validation, build release artifacts, commit on `main`, and push `main` to `origin`.

That maintainer policy applies to this repository. The installed CodeDungeon workflows remain PR-centered for user projects: they create or reuse feature branches, push them, open or reuse GitHub PRs, run adversarial review, and require concrete verification before reporting `COMPLETE`.

See [`docs/MAINTAINER_POLICY.md`](docs/MAINTAINER_POLICY.md) for the full completion checklist.

## Architecture

Source of truth lives in `src/codedungeon/`:

- `cmd/`: Cobra commands for setup, bootstrap, phase state, repo discovery, review, plans, QA, reports, install/migrate/status, project rules, and hook adapters.
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
codedungeon-codex rules status
codedungeon-codex rules lint
codedungeon-codex hooks install --provider codex --mode warn
```

The same command surface exists for Claude via `codedungeon-claude`.

After setup, the promoted agent-facing workflow is `/codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>` in Claude Code, or `$codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>` in Codex. Without a mode flag, the router selects automatically and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>` before dispatch. Compatibility aliases remain installed: `/one-shot`, `/side-quest`, `/main-quest` for Claude Code, and `$one-shot`, `$side-quest`, `$main-quest` for Codex. See [`docs/WORKFLOWS.md`](docs/WORKFLOWS.md) for mode selection, Project Rules, hooks, and branch/PR handling.

CodeDungeon completion is PR-centered and verification-gated: code-writing workflows are only `COMPLETE` after build/check/test verification passes, the branch is pushed, a PR exists, adversarial review is posted to the PR, and the final verdict is `APPROVED`. Review approval does not replace verification. Terminal output uses the standard CodeDungeon PR Report.

## Docs

- `src/codedungeon/docs/ARCHITECTURE.md`: current architecture.
- `src/codedungeon/docs/PROVIDERS.md`: provider model and how to add another provider.
- `docs/WORKFLOWS.md`: workflow names, provider invocation, and One Shot / Side Quest / Main Quest behavior.
- `docs/MIGRATING.md`: safe upgrade and migration guide for existing projects.
- `docs/MAINTAINER_POLICY.md`: main-only maintenance, required docs/build/commit/push flow, and hook policy.
- `release/README.md`: release/install guide.
- `release/QUICKSTART.md`: end-user quickstart.

## License

Apache-2.0.
