# CodeDungeon

An **agent-first M2M workflow kernel** for shipping code. A single Go binary that
drives a deterministic pipeline — plan → execute → QA → review → PR — with durable
SQLite state, hard gates, and a PR-centered handoff. It is a state machine, not just
prompts: the agent calls it, it returns the next action, the agent executes, and the
binary records evidence.

The authoritative, machine-readable workflow contract is `codedungeon kernel` (JSON).
Agents should read that over any prose here.

## Install

Run the provider installer inside a git project (or pass `--target`):

```bash
./install.sh --provider claude     # or: --provider codex
# Windows: .\install.ps1 -Provider claude
```

Direct download:

```bash
curl -fsSL https://raw.githubusercontent.com/Overstrider/codedungeon/main/release/get-codedungeon.sh | bash -s -- https://github.com/Overstrider/codedungeon claude
```

Setup is project-local: it writes the provider pack (`.claude/*` or `.codex/*`) and
shared mutable state (`.codedungeon/`), and never touches your home dir. The Claude
surface is `/codedungeon`; the Codex surface is `$codedungeon`.

## Quickstart

```bash
codedungeon setup --yes        # init DB + install provider pack
/codedungeon --rules           # discover & approve Project Rules (do this first)
/codedungeon --full "build X"  # run a full workflow → opens a PR for review
```

## Modes

| Mode | Alias | Use when | Prerequisite |
|------|-------|----------|--------------|
| `--rules` | — | Discover/approve Project Rules. Run before the first workflow. | git repo |
| `--oneshot` | `/one-shot` | Small change, no task splitting. | rules (warn) |
| `--lite` | `/side-quest` | Simple planned work, single repo. | a plan in `.codedungeon/plans/` |
| `--full` | `/main-quest` | Complex/multi-repo; full lifecycle + report. | approved rules |
| `--auto` | — | Router picks full/lite/oneshot from the request. | — |

## How it works

**Agent-first.** The agent runs `codedungeon run --full|--lite|--oneshot --prompt "…"`.
The binary returns JSON with `current_step`, `blockers`, `timeline`, and `next_action`.
The agent performs `next_action` with its own tools, then records progress:

```bash
codedungeon run advance --step <step> --status completed --summary "…" --artifact <path>
codedungeon run finalize        # closes hard gates, renders the report
```

**Stages** (full workflow). All recoverable except finalization:

```
project_rules → task_maker → planning → execution → qa → code_review → finalization
```

Each non-finalization stage produces evidence under `.codedungeon/`. For the exact
stages, modules, and gates, run `codedungeon kernel`.

**Project Rules.** `--rules` deep-reads the repo, drafts `.codedungeon/project-rules.md`,
and on approval compacts it to `.codedungeon/project-rules.compact.md`. Required for
`--full`/`--lite`, warning-only for `--oneshot`. Every artifact carries the envelope
`PROJECT_RULES_STATUS` / `PROJECT_RULES_DIGEST` / `PROJECT_RULES_READ`.

## Completion gates

`run finalize` emits `READY_FOR_USER_REVIEW` only after all gates pass:

- Git repo with `origin`; `gh` authenticated.
- Project Rules approved (full/lite).
- QA evidence recorded (phase 6).
- `code-review --post` wrote review artifacts and posted to the PR.
- Branch pushed, PR open (CodeDungeon never merges).
- Artifact registry verified.

Terminal states: `READY_FOR_USER_REVIEW`, `BLOCKED`, `MAX_CYCLES_REACHED`.

## Commands

| Area | Commands |
|------|----------|
| Setup | `setup`, `bootstrap`, `install`, `migrate`, `status`, `diagnose`, `version` |
| Workflow | `run` (`--full`/`--lite`/`--oneshot`/`--auto`), `run advance`, `run finalize`, `run unlock` |
| Rules | `rules status`, `rules lint`, `rules approve`, `rules compact`, `rules gate` |
| Planning | `plan run`, `plan status`, `plan resume`, `plan validate`, `plan promote` |
| Execution | `execute` |
| QA / Review | `qa`, `code-review`, `review`, `git` (guard/pr/verify) |
| Report | `report`, `artifacts`, `phase`, `observe`, `trace` |
| Config | `config models`, `config set-models`, `config effort` |
| Infra | `hooks`, `cleanup`, `db`, `prompts`, `repo`, `map`, `kernel`, `task-maker` |

Run `codedungeon <cmd> --help` for flags. `kernel` is the source of truth for the workflow.

## Notes

- **Cross-platform hooks.** `hooks install` generates a `.sh` hook (bash) on Linux/macOS
  and a `.ps1` hook (PowerShell) on Windows.
- **Model config.** `config set-models --reasoning <id> --fast <id>` (add `--strict-models`
  to reject unknown model IDs instead of warning).
- **State.** Durable SQLite at `.codedungeon/codedungeon.db` (schema v16, FTS5). External
  spawns (git/gh/curl/provider) run under timeouts so a hung CLI can't pend the run.
- **Providers.** `claude` and `codex` share `.codedungeon/`; each gets its own pack and
  router surface. See `src/codedungeon/docs/PROVIDERS.md`.

## More

- [docs/WORKFLOWS.md](docs/WORKFLOWS.md) — modes, Task Maker, gates, telemetry.
- [docs/MIGRATING.md](docs/MIGRATING.md) — safe upgrades.
- [src/codedungeon/docs/ARCHITECTURE.md](src/codedungeon/docs/ARCHITECTURE.md),
  [PROVIDERS.md](src/codedungeon/docs/PROVIDERS.md) — internals for contributors.

License: see [LICENSE](LICENSE).
