# CodeDungeon

> **If you are a human reading this:** you do not need to run setup or read the
> installation instructions. Send this repository URL to your Codex or Claude
> Code agent; it will set itself up.

## Agent-First M2M CLI

CodeDungeon is a machine-to-machine (M2M) CLI workflow kernel for Codex and
Claude Code. It is built for agents to call commands, not for humans to manually
follow setup docs. Humans can use it directly, but the default path is: send an
agent this repository URL, then let it run the CLI.

It is more than a prompt pack. The binary installs provider-native surfaces,
routes work through repeatable workflows, stores durable SQLite state under
`.codedungeon/`, and gates completion on QA, code review, GitHub PR state,
artifact integrity, and final-report evidence.

Why agents use it:

- Project-local setup for Codex or Claude without global provider changes.
- Deterministic modules for rules, task shaping, planning, execution, QA,
  review, artifacts, telemetry, recovery, and finalization.
- Durable state for interrupted work: runs, phases, handoffs, tasks, findings,
  QA sessions, review evidence, and registered artifacts.
- PR-centered workflow gates that stop agents from claiming work is complete
  until branch, PR, verification, review, and report evidence are present.
- Provider-native command surfaces with a shared workflow kernel: `$codedungeon`
  for Codex and `/codedungeon` for Claude Code.

Agents can inspect the kernel contract directly:

```bash
codedungeon kernel
```

The command prints a machine-readable manifest of provider surfaces, workflow
modes, modules, gates, durable state paths, local project-scope guarantees, and
license. It is safe to call before choosing whether to use Project Rules, Task
Maker, the full runner, standalone QA, code review, artifact checks, or
finalization.

## Mental Model

- Provider packs install native surfaces for Codex CLI and Claude Code.
- Agent-facing commands are the normal entry points: `$codedungeon`,
  `/codedungeon`, `$task-maker`, `/task-maker`, `$code-review`, and
  `/code-review`.
- `codedungeon run` is the autonomous orchestrator behind the router. It selects
  or receives a workflow mode, starts a child custody session, and combines
  modules such as rules, planning, execution, QA, review, git, artifacts, and
  reporting.
- Standalone modules remain directly callable. You can run only Project Rules,
  Task Maker, planning, one task execution, QA, code review, artifact checks, or
  migration without starting a full workflow.
- `.codedungeon/` is the shared source of truth for mutable state, evidence, and
  workflow artifacts. Provider folders stay provider-native.

Provider binaries:

- `codedungeon-codex` for Codex CLI.
- `codedungeon-claude` for Claude Code.
- `claude` is the canonical Claude provider name. `claude-code` and `claude-ce`
  are legacy aliases.

## Install

From a checked-out CodeDungeon release or repository, run the provider installer
inside the target git project or pass a target explicitly.

```bash
./install.sh --provider codex
./install.sh --provider claude
./release/install.sh --provider codex --target /path/to/project
```

On Windows PowerShell:

```powershell
.\install.ps1 -Provider codex
.\install.ps1 -Provider claude
.\release\install.ps1 -Provider codex -Target C:\path\to\project
```

The root installers delegate to `release/install.sh` and
`release/install.ps1`. Provider selection is required so a project does not
accidentally install the wrong provider pack.

For direct download from GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon codex
./codedungeon-codex setup
```

Use `claude` instead of `codex` for Claude Code.

## Quickstart

Inside a git project:

```bash
codedungeon-codex setup
# or
codedungeon-claude setup
```

Non-interactive setup:

```bash
codedungeon-codex setup --yes
codedungeon-claude setup --yes
```

Recommended first workflow after setup:

```text
$codedungeon --rules
# or
/codedungeon --rules
```

Project Rules discovery deep-reads the repo, drafts
`.codedungeon/project-rules.md`, waits for explicit user confirmation, then
uses `codedungeon rules approve` and `codedungeon rules compact` to create
`.codedungeon/project-rules.compact.md`. Full and lite workflows require
approved rules. One-shot workflows may continue with a warning.

When the user request is still rough, run Task Maker first:

```text
$task-maker
# or
/task-maker
```

Task Maker asks short clarifying questions, writes a minimal design plus a clean
English run prompt under `.codedungeon/task-maker/sessions/<session>/`, and
prints the reviewed provider-native command as `$codedungeon --full "<prompt>"`
or `/codedungeon --full "<prompt>"`. It does not start the full workflow unless
the user explicitly asks after reviewing the prompt.

Existing projects should run `codedungeon-<provider> migrate` after replacing
the binary. See [docs/MIGRATING.md](docs/MIGRATING.md).

## After Setup

Setup is project-local. It does not install user-home plugins, global feature
flags, or shell PATH changes. `--skip-global` is accepted only as a compatibility
no-op.

The release installer first stages the provider binary in:

```text
.codedungeon/bin/codedungeon-codex
.codedungeon/bin/codedungeon-claude
```

Then `setup` copies the running binary to the provider-native project bin:

```text
.codex/bin/codedungeon
.claude/bin/codedungeon
```

Codex setup installs:

- `.codex/agents/*.toml`
- `.codex/config.toml` with project-local `multi_agent_v2` config
- `.agents/skills/*`, including `codedungeon`, `task-maker`, `code-review`,
  `one-shot`, `side-quest`, and `main-quest`
- `.codedungeon/commands/*` editable playbooks
- `.codedungeon/phases/*` phase prompts
- `.codedungeon/codedungeon.db` and runtime directories
- an `agent_config_instruction` block describing what to insert in `AGENTS.md`

Runtime Codex launches that need custom agents pass `--enable multi_agent_v2`
directly. Setup does not persist user-global Codex feature flags.

Claude setup installs:

- `.claude/agents/*`
- `.claude/skills/*`
- `.claude/commands/*` thin slash-command wrappers
- `.claude/bin/codedungeon`
- `.codedungeon/commands/*` editable playbooks
- `.codedungeon/phases/*` phase prompts
- `.codedungeon/codedungeon.db` and runtime directories
- an `agent_config_instruction` block describing what to insert in `CLAUDE.md`

Both providers share `.codedungeon/` for mutable state:

```text
.codedungeon/codedungeon.db
.codedungeon/commands
.codedungeon/phases
.codedungeon/tasks
.codedungeon/plans
.codedungeon/plan
.codedungeon/state
.codedungeon/reviews
.codedungeon/code-review
.codedungeon/qa/sessions
.codedungeon/execute/sessions
.codedungeon/memory
.codedungeon/reports
.codedungeon/task-maker
.codedungeon/project-rules.md
.codedungeon/project-rules.compact.md
.codedungeon/project-rules.json
```

## Choose A Workflow

Use the provider-native router unless you specifically need a standalone module.

| Mode or alias | Provider surface | Use when | Composes | Prerequisites | Expected output |
|---|---|---|---|---|---|
| `--rules` | `$codedungeon --rules` or `/codedungeon --rules` | First setup, major repo change, or stale rules. | Repo reading, rules draft, approval, compaction. | Git project with setup complete. | `.codedungeon/project-rules.md`, `.codedungeon/project-rules.compact.md`, and status/digest metadata. |
| `--oneshot` | `$codedungeon --oneshot "<prompt>"` or `/codedungeon --oneshot "<prompt>"` | Small scoped change that does not need task splitting. | Short plan, feature branch, implementation, QA, standalone code review, PR, finalization. | GitHub `origin`, authenticated `gh`; approved rules preferred but not mandatory. | Open PR and `READY_FOR_USER_REVIEW` report, or `BLOCKED` with evidence. |
| `--lite` | `$codedungeon --lite "<prompt>"` or `/codedungeon --lite "<prompt>"` | Simple single-repo work with an existing plan. | Plan resolution, task files, execution loop, QA, code review, PR, finalization. | Approved rules, prior plan in `.codedungeon/plans/*.md` or explicit plan path, GitHub `origin`, authenticated `gh`. | Open PR and final report after gates pass. |
| `--full` | `$codedungeon --full "<prompt>"` or `/codedungeon --full "<prompt>"` | Complex features, multi-repo work, architectural work, broad QA, or final reporting. | Full phase lifecycle, Project Context, planning, execution, QA, code review, git/PR gates, artifacts, final report. | Approved rules, GitHub `origin`, authenticated `gh`, model config from setup. | Durable phase state, evidence artifacts, open PR, and `READY_FOR_USER_REVIEW` report. |
| `--auto` or no mode flag | `$codedungeon "<prompt>"` or `/codedungeon "<prompt>"` | You want the router to choose. | Selects full, lite, or oneshot and prints `CODEDUNGEON_MODE_SELECTED: <mode> - <reason>`. | Same as the selected mode. | Same as the selected mode. |
| `one-shot` alias | `$one-shot "<prompt>"` or `/one-shot "<prompt>"` | Compatibility name for one-shot. | Same as `--oneshot`. | Same as `--oneshot`. | Same as `--oneshot`. |
| `side-quest` alias | `$side-quest "<prompt>"` or `/side-quest "<prompt>"` | Compatibility name for lite. | Same as `--lite`. | Same as `--lite`. | Same as `--lite`. |
| `main-quest` alias | `$main-quest "<prompt>"` or `/main-quest "<prompt>"` | Compatibility name for full. | Same as `--full`. | Same as `--full`. | Same as `--full`. |

Code-writing workflows are PR-centered. They create or reuse a feature branch,
push it, create or reuse a GitHub PR, run QA, run standalone CodeDungeon
code-review, post review evidence, and finalize only after gates pass. They do
not merge; the user performs final review and merge.

## Standalone Modules

Every row below can be used independently of `$codedungeon` or `/codedungeon`.
Use `codedungeon-codex` and `codedungeon-claude` interchangeably according to
the installed provider.

| Module | Purpose | Common commands |
|---|---|---|
| Setup, install, migrate, status | Bootstrap a project and manage installed provider pack drift. | `setup`, `bootstrap`, `install`, `install --dry-run`, `install --force`, `migrate`, `status`, `diagnose` |
| Task Maker | Turn a rough request into a reviewed full-run prompt without dispatching it. | `task-maker render --surface codex|claude --input <request.json> --out <dir> --print` |
| Project Rules | Track repo-specific operating rules and hook gates. | `rules status`, `rules lint`, `rules digest`, `rules approve`, `rules compact`, `rules gate` |
| Project Context | Maintain compact project memory and an audit ledger. | `project-context status`, `init`, `approve`, `reject`, `audit`, `envelope` |
| Planning | Produce and validate plans, task graphs, and promoted task artifacts. | `plan run`, `plan status`, `plan resume`, `plan validate`, `plan promote`, `plan meta`, `plan append-fix-tasks` |
| Execution | Run implementation task contracts one at a time. | `execute task`, `execute plan`, `execute run`, `execute status`, `execute rollback` |
| QA | Detect frameworks, run verification, record evidence, and report sessions. | `qa preflight`, `qa run`, `qa status`, `qa report`, `qa detect-framework`, `qa validate-api`, `qa secret-scan` |
| Review | Run standalone review or lower-level review pipeline commands. | `code-review --post`, `review context-paths`, `review run`, `review post` |
| Git | Guard branch safety, resolve PRs, verify PR/review gates, inspect diffs. | `git guard`, `git pr`, `git verify`, `git diff` |
| Report and run finalization | Inspect or close autonomous workflow gates. | `run status`, `run finalize`, `run finalize --dry-run`, `run unlock`, `report render` |
| Artifact registry | Inspect, verify, and backfill runtime evidence rows. | `artifacts list`, `artifacts verify`, `artifacts backfill` |
| Phase lifecycle | Create and update phase state and handoffs. | `phase init`, `phase start`, `phase done`, `phase skip`, `phase fail`, `phase next`, `phase info`, `phase render-state` |
| Prompts and DB | Manage embedded/user prompt versions and SQLite state. | `prompts list|get|set|diff`, `db init|migrate|export|search` |
| Repo and map | Discover repos, resolve repo keys, and scan codebase shape. | `repo discover`, `repo resolve`, `repo check-test-auth`, `map <path>` |
| Hooks | Install Project Rules hook adapters. | `hooks install --provider codex|claude --mode warn|enforce` |
| Trace and observe | Record and inspect agent telemetry. | `trace agent-start`, `trace agent-end`, `observe agents`, `observe report` |
| Cleanup and config | Remove stale runtime artifacts and manage model config. | `cleanup --dry-run`, `cleanup --all --confirm`, `config models`, `config set-models`, `config model`, `config effort` |

## Command Examples

Setup and migration:

```bash
codedungeon-codex setup --yes
codedungeon-claude setup --target /path/to/project --yes
codedungeon-codex install --dry-run
codedungeon-codex install --force
codedungeon-codex migrate
codedungeon-codex status
codedungeon-codex diagnose --strict
```

Workflow router:

```text
$codedungeon --rules
$codedungeon --oneshot "Fix the failing import path"
$codedungeon --lite "Execute .codedungeon/plans/refactor.md"
$codedungeon --full "Implement the billing export flow with tests"
$codedungeon "Small typo fix in README"

/codedungeon --rules
/codedungeon --oneshot "Fix the failing import path"
/codedungeon --lite "Execute .codedungeon/plans/refactor.md"
/codedungeon --full "Implement the billing export flow with tests"
/codedungeon "Small typo fix in README"
```

Task Maker:

```text
$task-maker
/task-maker
```

```bash
codedungeon-codex task-maker render \
  --surface codex \
  --input .codedungeon/task-maker/sessions/<session>/request.json \
  --out .codedungeon/task-maker/sessions/<session> \
  --print

codedungeon-claude task-maker render \
  --surface claude \
  --input .codedungeon/task-maker/sessions/<session>/request.json \
  --out .codedungeon/task-maker/sessions/<session> \
  --print
```

Project Rules and Project Context:

```bash
codedungeon-codex rules status
codedungeon-codex rules lint
codedungeon-codex rules digest
codedungeon-codex rules approve --by "reviewer"
codedungeon-codex rules compact
codedungeon-codex rules gate --event Stop --mode warn

codedungeon-codex project-context status
codedungeon-codex project-context init --mode auto --first-prompt "Add CSV export"
codedungeon-codex project-context approve --proposal <id> --by "reviewer"
codedungeon-codex project-context audit --query "billing" --limit 10
codedungeon-codex project-context envelope
```

Planning and execution:

```bash
codedungeon-codex plan run \
  --prompt "Split the auth cleanup into safe tasks" \
  --mode full \
  --project-context .codedungeon/project-rules.compact.md \
  --out .codedungeon/task-planning/<session> \
  --promote

codedungeon-codex plan status --session <planning-session-id>
codedungeon-codex plan resume --session <planning-session-id>
codedungeon-codex plan validate --task-graph .codedungeon/task-planning/<session>/task-graph.json
codedungeon-codex plan promote --from .codedungeon/task-planning/<session>
codedungeon-codex plan append-fix-tasks --from .codedungeon/code-review/review.json --to .codedungeon/plan/PLAN.md --cycle 2

codedungeon-codex execute task \
  --task .codedungeon/tasks/<task>.json \
  --project-context .codedungeon/project-rules.compact.md

codedungeon-codex execute status --session <execution-session-id>
codedungeon-codex execute rollback --session <execution-session-id> --to before --confirm
```

QA:

```bash
codedungeon-codex qa detect-framework --path .
codedungeon-codex qa preflight --mode e2e --root .
codedungeon-codex qa run --auto --fresh
codedungeon-codex qa run --mode e2e --root frontend --fresh
codedungeon-codex qa run --cwd backend --cmd "cargo test" --fresh
codedungeon-codex qa run --phase 6 --cmd "go test ./..." --fresh
codedungeon-codex qa status --latest
codedungeon-codex qa report --latest
codedungeon-codex qa secret-scan --tracked-only
```

Review and git gates:

```bash
codedungeon-codex code-review \
  --url <pr-url> \
  --project-context .codedungeon/project-rules.compact.md \
  --task-context .codedungeon/plan/PLAN.md \
  --out .codedungeon/code-review \
  --post

codedungeon-codex review context-paths
codedungeon-codex review run --dir .codedungeon/reviews/adv-review
codedungeon-codex review post --dir .codedungeon/reviews/adv-review

codedungeon-codex git guard --repo .
codedungeon-codex git pr --repo . --branch <branch> --with-context
codedungeon-codex git verify --repo . --branch <branch>
codedungeon-codex git diff --repo . --base main --mode stat
```

Use `code-review --post` as the normal final review evidence path. The lower
level `review` commands are kept for the adversarial review pipeline and legacy
evidence flows.

Run finalization, reporting, and recovery:

```bash
codedungeon-codex run --full --prompt "Implement the full workflow"
codedungeon-codex run status
codedungeon-codex run finalize --dry-run
codedungeon-codex run finalize
codedungeon-codex run unlock --reason "provider child crashed before returning"
codedungeon-codex report render
```

Artifacts, phases, prompts, repo discovery, telemetry, and cleanup:

```bash
codedungeon-codex artifacts list --latest-run
codedungeon-codex artifacts verify --latest-run
codedungeon-codex artifacts backfill --run <run-id>

codedungeon-codex phase init --feature "CSV export" --branch feat/csv-export --project-mode SINGLE
codedungeon-codex phase start 5
codedungeon-codex phase done 5 --summary "Implementation complete" --artifacts .codedungeon/code-review/review.json
codedungeon-codex phase info 5
codedungeon-codex phase next
codedungeon-codex phase render-state

codedungeon-codex prompts list
codedungeon-codex prompts get main-quest
codedungeon-codex prompts diff main-quest --from 1 --to 2
codedungeon-codex db search "verification"
codedungeon-codex db export

codedungeon-codex repo discover --root . --persist
codedungeon-codex repo resolve api
codedungeon-codex map . --format compact --max-tokens 20000

codedungeon-codex hooks install --provider codex --mode warn
codedungeon-codex hooks install --provider claude --mode warn

codedungeon-codex trace agent-start --phase 5 --role dev-worker --agent-name worker-1
codedungeon-codex trace agent-end --id <agent-run-id> --status COMPLETED --summary "task done"
codedungeon-codex observe agents
codedungeon-codex observe report

codedungeon-codex config models
codedungeon-codex config set-models --reasoning gpt-5.5 --reasoning-effort xhigh --fast gpt-5.5 --fast-effort medium
codedungeon-codex cleanup --dry-run --all
codedungeon-codex cleanup --all --confirm
```

The same module surface exists through `codedungeon-claude`. Use Codex or
Claude examples only where the agent-facing command syntax differs.

## Gates And Evidence

CodeDungeon completion is verification-gated:

- The project must be a git repo with a GitHub `origin` remote.
- `gh` must be authenticated for PR workflows.
- Full and lite workflows require approved Project Rules.
- Code-writing workflows must run QA and record evidence under
  `.codedungeon/qa/sessions/<session-id>/`.
- Standalone code review must write review artifacts and, for PR workflows, post
  the review comment with custody evidence.
- The branch must be pushed and the PR must exist and remain open.
- `codedungeon git verify` rejects protected branches, unpushed commits, missing
  PRs, missing review post evidence, arbitrary marker comments, and merged PRs.
- `codedungeon run finalize` closes final gates, auto-runs workflow QA when the
  Phase 6 ledger is missing or failing, renders the final report from DB
  evidence, records Phase 7, and leaves the PR open.

Terminal workflow states are:

```text
READY_FOR_USER_REVIEW
BLOCKED
MAX_CYCLES_REACHED
```

Review approval does not replace verification. Hand-written review reports and
hand-written final reports are not accepted as gate evidence.

## Runtime Artifact Registry

Runtime evidence is indexed in SQLite table `artifacts`. This is separate from
`installed_artifacts`, which tracks provider pack files written by setup,
install, and migrate.

The runtime registry records normalized project-relative paths, absolute paths,
module ownership, owner IDs, phase, role, kind, file/directory type, media type,
size, SHA-256, metadata JSON, and timestamps. Producers register artifacts as
they run:

- `qa`: session directories, request/preflight/check/result JSON, logs,
  summaries, Playwright results, and findings.
- `review`: review directories, manifests, Markdown/JSON review output,
  summaries, decisions, and posted review evidence.
- `planning`: planning session directories, blackboards, evaluations, task
  graphs, master plans, task contracts, and agent outputs.
- `execution`: task graphs/contracts, execution sessions, attempts, diffs,
  worker results, result JSON, and verification logs.
- `report`, `phase`, `handoff`, and `trace`: final reports, memory reports,
  phase outputs, handoff artifacts, agent task paths, and agent artifacts.

Use:

```bash
codedungeon-codex artifacts list --latest-run
codedungeon-codex artifacts verify --latest-run
codedungeon-codex artifacts backfill --run <run-id>
```

`artifacts verify` checks registered files and directories for missing or
drifted evidence. `artifacts backfill` indexes evidence produced before the
registry existed or by modules that wrote files before a run was interrupted.

## Architecture

Source of truth lives in `src/codedungeon/`:

- `main.go`: root Cobra command and provider-specific binary entry point.
- `cmd/`: setup, bootstrap, install/migrate/status, run router, phase state,
  repo discovery, project rules, project context, planning, execution, QA,
  code review, git gates, runtime artifacts, telemetry, cleanup, and reports.
- `internal/provider/`: provider abstraction. Current providers are `codex` and
  `claude`.
- `internal/prompts/`: embedded provider prompt packs. Claude uses `files/`;
  Codex uses `codex-files/`.
- `internal/db/`: SQLite schema, migrations, FTS5 search, installed artifact
  tracking, runtime evidence registry, telemetry, QA, review, planning, and
  execution state.
- `internal/artifacts/`: runtime artifact registry service, hashing, path
  normalization, verification, and backfill.
- `internal/qa/`: standalone and workflow QA engine, framework detection,
  dependency preflight, check execution, findings, and evidence artifacts.
- `internal/codereview/` and `internal/reviewpipe/`: standalone code-review
  engine and lower-level review pipeline.
- `internal/taskplanning/` and `internal/taskexec/`: planning swarm contracts and
  one-task implementation executor.
- `release/`: shippable installers, docs, skills, and binaries.
- `scripts/build-release.ps1`: release build script used by maintainers.

The shared lifecycle is provider-agnostic. Provider-specific behavior is pushed
to paths, instruction files, command/skill surfaces, agent formats, model
defaults, required provider CLI args, and prompt pack content.

## Provider Packs

Installed provider artifacts are tracked in `installed_artifacts` with:

- `provider`
- `pack_id`
- `pack_version`
- `install_path`
- `kind`
- `logical_name`
- `sha256`
- `user_modified`

This lets `status`, `install`, and `migrate` reason about Codex and Claude packs
independently and preserve user-modified artifacts unless `install --force` is
used.

Current packs:

- Codex pack `codedungeon-codex`: `.codex/agents`, `.codex/config.toml`,
  `.agents/skills`, `.codedungeon/commands`, and `.codedungeon/phases`.
- Claude pack `codedungeon-claude`: `.claude/agents`, `.claude/skills`,
  `.claude/commands` wrappers, `.codedungeon/commands`, and `.codedungeon/phases`.
  Provider instruction files are never overwritten; setup returns insertion
  guidance for the installer agent.

See [src/codedungeon/docs/PROVIDERS.md](src/codedungeon/docs/PROVIDERS.md) for
provider extension details.

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

Each provider binary embeds its default provider via Go ldflags. Normal users
should choose by binary name and not rely on `CODEDUNGEON_PROVIDER`.

## Maintainer Policy

Repository maintenance is main-only. Maintainers should follow
[docs/MAINTAINER_POLICY.md](docs/MAINTAINER_POLICY.md): update relevant docs,
run validation, rebuild release artifacts when required, commit on `main`, and
push `main` to `origin`.

That policy applies to this repository; installed CodeDungeon workflows remain PR-centered for user
projects and may create or reuse feature branches before review.

## Docs

- [docs/WORKFLOWS.md](docs/WORKFLOWS.md): workflow mode details, Project Rules,
  Task Maker, hooks, branch/PR handling, and gates.
- [docs/MIGRATING.md](docs/MIGRATING.md): safe upgrade and migration guide for
  existing projects.
- [docs/MAINTAINER_POLICY.md](docs/MAINTAINER_POLICY.md): repository
  maintenance policy and completion checklist.
- [src/codedungeon/docs/ARCHITECTURE.md](src/codedungeon/docs/ARCHITECTURE.md):
  current architecture.
- [src/codedungeon/docs/PROVIDERS.md](src/codedungeon/docs/PROVIDERS.md):
  provider model and how to add another provider.
- [release/README.md](release/README.md): release/install guide.
- [release/QUICKSTART.md](release/QUICKSTART.md): shorter end-user quickstart.

## License

AGPL-3.0-only. See [LICENSE](LICENSE).
