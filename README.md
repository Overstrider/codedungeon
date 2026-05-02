# codedungeon

Deterministic Go CLI for AI coding-agent project pipelines. It moves repeatable workflow mechanics into one binary: state, phase handoffs, prompt/agent/skill installation, repo discovery, review dedupe, QA assertions, task tracking, migrations, and final reports. The LLM keeps the judgment work.

Provider support is first-class:

- `codedungeon-codex` for Codex CLI.
- `codedungeon-claude` for Claude Code.
- `claude` is the canonical provider name. `claude-code` and `claude-ce` are accepted only as legacy aliases.

The binary is project-scoped, requires a git repo, and stores mutable state in `.codedungeon/`:

- Shared runtime: `.codedungeon/codedungeon.db`, `.codedungeon/commands`, `.codedungeon/phases`, `.codedungeon/tasks`, `.codedungeon/plan`, `.codedungeon/state`, `.codedungeon/reviews`, `.codedungeon/qa/sessions`, `.codedungeon/memory`, `.codedungeon/project-rules.md`, `.codedungeon/project-rules.compact.md`, `.codedungeon/project-rules.json`.
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

```powershell
Push-Location src/codedungeon
go test ./...
Pop-Location
.\scripts\build-release.ps1
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

That maintainer policy applies to this repository. The installed CodeDungeon workflows remain PR-centered for user projects: they require a GitHub remote plus authenticated `gh`, create or reuse feature branches, push them, open or reuse GitHub PRs, run adversarial review, and require concrete verification before reporting `COMPLETE`. There is no local-only completion path for code-writing workflows.

See [`docs/MAINTAINER_POLICY.md`](docs/MAINTAINER_POLICY.md) for the full completion checklist.

## Architecture

Source of truth lives in `src/codedungeon/`:

- `cmd/`: Cobra commands for setup, bootstrap, phase state, repo discovery, review, plans, task execution, QA, reports, install/migrate/status, project rules, and hook adapters.
- `internal/provider/`: provider abstraction. Current providers are `claude` and `codex`.
- `internal/prompts/`: embedded provider prompt packs. Claude uses `files/`; Codex uses `codex-files/`.
- `internal/db/`: SQLite schema, migrations, FTS5, and installed artifact tracking.
- `internal/qa/`: standalone and workflow QA engine, framework detection, dependency preflight, check execution, findings, and evidence artifacts.
- `release/`: shippable installers, docs, skills, and binaries.
- `scripts/build-release.ps1`: cross-platform Go release build used by maintainers.

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
codedungeon-codex execute task --task .codedungeon/task-planning/<session>/task.json --project-context .codedungeon/project-context.md
codedungeon-codex execute status --session <execution-session-id>
codedungeon-codex qa preflight --mode e2e --root <project>
codedungeon-codex qa run --auto
codedungeon-codex qa run --mode e2e --root <project> --fresh
codedungeon-codex qa run --cwd backend --cmd "cargo test" --fresh
codedungeon-codex qa status --latest
codedungeon-codex qa report --latest
codedungeon-codex trace agent-start --phase 5 --role dev-worker
codedungeon-codex observe report
codedungeon-codex hooks install --provider codex --mode warn
```

The same command surface exists for Claude via `codedungeon-claude`.

After setup, the promoted agent-facing workflow is `/codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>` in Claude Code, or `$codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>` in Codex. Code-writing modes delegate to `codedungeon run --full|--lite|--oneshot --prompt "<prompt>"`, which owns the workflow as an autonomous child session. Without a mode flag, the router selects automatically and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>` before dispatch. Compatibility aliases remain installed: `/one-shot`, `/side-quest`, `/main-quest` for Claude Code, and `$one-shot`, `$side-quest`, `$main-quest` for Codex. See [`docs/WORKFLOWS.md`](docs/WORKFLOWS.md) for mode selection, Project Rules, hooks, and branch/PR handling.

CodeDungeon completion is PR-centered and verification-gated: code-writing workflows end at `READY_FOR_USER_REVIEW`, not merge. QA can run standalone with `codedungeon qa run --auto` or concrete checks such as `codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`; every QA run creates a durable session under `.codedungeon/qa/sessions/`. During full-cycle finalization, `codedungeon run finalize` invokes QA automatically when the active Phase 6 ledger is missing or failing, so the final workflow does not depend on an agent manually remembering to call QA. The branch must be pushed, a PR must exist and remain open, adversarial review must be generated by `codedungeon review run`, and the review must be posted with `codedungeon review post` from the latest validated review evidence directory. `codedungeon git verify` accepts only recorded review-comment evidence and rejects arbitrary marker comments or merged PRs. Review approval does not replace verification. Final reports must be finalized with `codedungeon run finalize`, which records Phase 7 after gates pass; hand-written review or final reports are not accepted as gate evidence. The user performs final review and merge.

## QA / Testing / Verification

The QA module is a standalone subsystem and the full-cycle verification gate. It can be called directly by users, by agents, or by `codedungeon run finalize`.

Standalone examples:

```bash
codedungeon-codex qa preflight --mode e2e --root ./my-project
codedungeon-codex qa run --auto --root ./my-project --fresh
codedungeon-codex qa run --mode e2e --root ./my-project --fresh
codedungeon-codex qa run --root ./my-project --cwd backend --cmd "cargo test" --fresh
codedungeon-codex qa status --latest
codedungeon-codex qa report --latest
```

Full-cycle behavior:

- `codedungeon run finalize` checks Phase 6 verification records.
- If Phase 6 evidence is missing or failing, it invokes workflow QA automatically.
- Workflow QA runs in `auto` mode and records Phase 6 evidence.
- Finalization is blocked unless QA passes.

Artifacts are written to `.codedungeon/qa/sessions/<session-id>/`:

- `request.json`: normalized request.
- `preflight.json`: dependency readiness, including Playwright.
- `checks/*.json`: structured check results.
- `logs/*.log`: command stdout/stderr/error evidence.
- `playwright/results.json`: Playwright JSON output when E2E runs.
- `findings.json`: blocking findings for missing dependencies or failed checks.
- `summary.md` and `result.json`: human and machine-readable final evidence.

Framework behavior:

- `qa preflight` records readiness only; it does not execute tests.
- `qa run --auto` detects project or monorepo components and runs checks sequentially.
- Backend projects use native test commands, such as `go test ./...` for Go or `cargo test` for Rust.
- Frontend web E2E uses Playwright when `@playwright/test` or `playwright.config.*` is present.
- In monorepos, Playwright is resolved inside the owning component, such as `frontend`, instead of requiring Playwright at the root.
- Missing required Playwright is reported as `BLOCKED` with an install hint. A failing test command is `FAIL` with log evidence.

The verified `examples/v6` monorepo path currently runs:

```text
backend: cargo test
frontend: npx playwright test --reporter=json
```

A single QA session runs checks sequentially. Avoid launching multiple E2E QA sessions concurrently against the same project root unless the app uses unique ports; two Playwright web servers can conflict on fixed ports such as `127.0.0.1:3100`.

Agent telemetry is informational evidence. Workflows record subagent lifecycles with `codedungeon trace agent-start` and `codedungeon trace agent-end`; `codedungeon observe agents` and `codedungeon observe report` summarize the run timeline without replacing QA, review, PR, or report gates.

Task execution is available as a standalone Implementation Executor/Ralphloop module through `codedungeon execute task --task <task.json>`. It runs one task contract at a time, persists an explicit execution session, supports `--resume <id>` and `--reset-session`, writes attempt snapshots under `.codedungeon/execute/sessions/`, blocks known destructive commands in its own shell/git paths, and auto-commits only after declared verification commands pass. Auto-push and semver tagging are opt-in through `.ralphrc` or `CODEDUNGEON_EXEC_*` environment overrides.

`--lite` and `--oneshot` are compact workflows. The runner pre-skips the full phase ledger before phase 7, but final readiness is still blocked on QA, review, PR, and report evidence.

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
