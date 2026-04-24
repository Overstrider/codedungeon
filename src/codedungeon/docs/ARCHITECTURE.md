# codedungeon — Architecture

Deterministic Go CLI that orchestrates AI-coding-agent dev pipelines (plan → execute → review → PR). Single binary, embedded prompts, SQLite (FTS5) state, project-scoped.

## What it does

codedungeon is the **deterministic backbone** beneath an AI coding agent (Claude Code, Codex, OpenCode, etc.). The agent calls `codedungeon` subcommands to:

1. **Bootstrap** a project (`setup` / `bootstrap`) — installs binary + DB + 58 embedded artifacts (agents, skills, commands, phases) into the project's config dir.
2. **Track pipeline state** (`phase`) — start/done/skip/fail per phase, atomic transitions, handoff schema.
3. **Discover repos** (`repo`) — single/multi/bootstrap detection from manifests (Go/Rust/Next.js/Kotlin/Python/Elixir/C++).
4. **Decompose plans** (`plan`) — parse PLAN.md, extract task lists, append fix tasks.
5. **Run adversarial review** (`review`) — dedupe + filter + classify + render + verdict on persona findings JSON.
6. **Wrap git/gh** (`git`) — branch guard, PR check, diff, verify-review-posted.
7. **Render reports** (`report`) — final pipeline report from DB state.
8. **Manage embedded prompts** (`prompts`, `install`, `migrate`, `status`) — version drift detection, user-modification tracking.
9. **Sweep stale artifacts** (`cleanup`) — remove ephemeral state without touching code.

The AI agent never has to remember paths, model IDs, or marker strings — it asks codedungeon.

## Top-level layout

```
codedungeon/
├── main.go                       # cobra root, version wiring
├── cmd/                          # one file per subcommand domain
│   ├── common.go                 # guards, DB open, JSON emit, preflight
│   ├── bootstrap.go              # first-run: copy binary, init DB, install artifacts
│   ├── setup.go                  # interactive wrapper around bootstrap
│   ├── config.go                 # model_reasoning / model_fast meta
│   ├── repo.go                   # discover, resolve, check-test-auth, CLAUDE.md upserts
│   ├── phase.go                  # phase lifecycle + state rendering
│   ├── plan.go                   # PLAN.md parsing + fix-task append
│   ├── review.go                 # adversarial review pipeline driver
│   ├── git.go                    # git/gh wrappers (branch guard, PR query, verify)
│   ├── report.go                 # phase-7 report rendering
│   ├── install.go                # install/migrate/status for embedded artifacts
│   ├── cleanup.go                # remove stale ephemeral dirs
│   ├── spawn.go                  # phase spawn-prompt builder
│   ├── prompts.go                # embedded prompt get/list/set/diff
│   ├── mapcmd.go                 # cartographer (codebase mapper)
│   ├── tui.go                    # interactive TUI helpers (banner, prompts)
│   └── *_test.go
├── internal/
│   ├── db/                       # SQLite schema + queries (FTS5 search)
│   ├── manifest/                 # language/framework/stack detection
│   ├── osadapter/                # OS abstraction (linux/windows/darwin via build tags)
│   ├── provider/                 # AI-agent abstraction (Claude/Codex/OpenCode)  ← THIS
│   ├── prompts/                  # embedded FS for agents/skills/commands/phases
│   ├── reviewpipe/               # dedupe/filter/classify/render/verdict logic
│   ├── render/                   # template helpers
│   └── ghcli/                    # GitHub helpers
└── docs/
    ├── ARCHITECTURE.md           # this file
    └── PROVIDERS.md              # provider abstraction + how to add a new one
```

## Two abstraction layers (dependency inversion)

codedungeon has **two interface-driven abstractions** that decouple code from concrete environments:

### 1. `internal/osadapter/` — OS abstraction
- Interface: `Adapter` (HomeDir, ExecutableExt, FindTool, RunShell, RunExec)
- Impls: `linux.go` / `windows.go` / `darwin.go` (build tags)
- Factory: `osadapter.Detect()`
- Why: Windows path quirks (`gh.exe`, `\` separators, `cmd /c` shell) vs unix.

### 2. `internal/provider/` — AI-coding-agent abstraction
- Interface: `Provider` (ConfigDir, AgentConfigFile, BinDir, DBPath, ModelDefaults, ReviewCommentMarker, …)
- Impls: `claude.go` (only one currently)
- Factory: `provider.Detect()` (env var `CODEDUNGEON_PROVIDER` or default Claude)
- Why: Claude Code uses `.claude/` + `CLAUDE.md` + `claude-opus-4-7`. Codex uses `.codex/` + `AGENTS.md` + `o3`. OpenCode/Cursor are different again.

**Both follow the same pattern**: interface + factory in pkg root, one file per concrete impl, callers do `pkg.Detect().Method()`.

## Data flow

### Setup flow (`codedungeon setup --yes`)

```
runSetup
 ├─ IsHomeConfig(target)?              ← provider.HomeGuardPaths()
 ├─ HasGit(target)?
 ├─ installGlobalPlugin()              ← provider.PluginDir() + PluginManifest()
 │   └─ skipped if !HasPluginSystem()
 ├─ pick models (interactive or defaults)
 │                                     ← provider.ModelAlternatives()
 ├─ RunBootstrap(target, reasoning, fast, force)
 │   ├─ mkdir <target>/<provider.BinDir()>
 │   ├─ copy self → <target>/<provider.BinDir()>/codedungeon
 │   ├─ Open(<target>/<provider.DBPath()>)
 │   ├─ seedEmbeddedPrompts(s)
 │   ├─ installEmbeddedArtifacts(s)    ← writes to <target>/<provider.ConfigDir()>/...
 │   ├─ s.SetMeta(os, project_root, cd_version, model_reasoning, model_fast)
 │   └─ upsertCodedungeonSection(<target>/<provider.AgentConfigFile()>)
 └─ EmitJSON(result)
```

### Pipeline flow (`/codedungeon-dev-cycle "feature"`)

The Claude Code slash command (an `.md` file) drives the agent through 10 phases. Each phase is a separate Task spawn. codedungeon CLI provides the deterministic glue:

```
Phase 0: validation         → codedungeon phase start 0; ...; codedungeon phase done 0
Phase 1: architect          → reads phase file from <provider.PhasesDir()>/war-room-architect.md
Phase 2': domain planning   ← spawn-prompt builds prompt with phase file path
Phase 3.5: QA planning      ← phase done writes handoff to <provider.StateDir()>/phase-N-output.md
Phase 4: task decomposition
Phase 5: execution          ← per-task ralph loop (specialist plan + general-purpose exec + review)
Phase 5.5: QA refinement
Phase 5.6: test decomposition
Phase 6: test execution
Phase 7: report             ← codedungeon report --plan-paths
```

State is durable in `<provider.DBPath()>` (SQLite FTS5). Findings, phases, handoffs, artifacts all queryable.

### Adversarial review flow (`/code-review`)

```
agent fans out 4 personas (Saboteur, NewHire, SecurityAuditor, SpecEnforcer) on Opus
  → each writes findings-<persona>.json to <provider.PlanDir()>/adv-review/
agent runs validators on Sonnet per finding
  → writes validator-*.json
agent runs classifier on Sonnet per validated finding
  → writes classifier-*.json
codedungeon review run --dir <provider.PlanDir()>/adv-review
  ├─ DedupeAndPromote
  ├─ ApplyValidators
  ├─ ApplyClassifiers
  ├─ Render review.md + review.json
  └─ Verdict (APPROVED | CHANGES_REQUESTED)
agent posts review.md to PR with marker "<provider.ReviewCommentMarker()>"
codedungeon git verify
  → checks PR has comment matching marker
```

## Embedded prompts vs install target

`internal/prompts/` uses `go:embed` to bundle 58 `.md` files into the binary:

- `agents/*.md` — subagent definitions (architect-planner, {lang}-specialist, …)
- `skills/{name}/SKILL.md` — language skills (rust, next.js, kotlin, …)
- `commands/*.md` — slash commands (`/minidungeon`, `/codedungeon-dev-cycle`, `/code-review`, …)
- `phases/*.md` — phase instructions (entrance-hall, war-room, forge, throne-room, …)

**Mechanism (agnostic)**: `prompts.Artifacts()` returns `[]Artifact{RelPath, Content}`. Install code in `cmd/install.go` and `cmd/bootstrap.go` writes each to `<root>/<provider.ConfigDir()>/<RelPath>`.

**Content (Claude-flavored)**: every prompt mentions `subagent_type`, the `Task` tool, `claude-opus-4-7` model IDs, `.claude/` paths, `CLAUDE.md`. A future provider needs its own prompt content (separate embed FS or replacement strategy — see PROVIDERS.md).

## Hard refusals (structured errors)

All failures emit JSON `{"error","hint","action"}`. The AI agent parses `action` to recover:

| action | meaning |
|---|---|
| `change-directory` | CWD is under provider's home config dir — move out |
| `init-git-or-bootstrap` | No `.git/` — run `git init` or `--target` |
| `ask-user-models` | `--reasoning` / `--fast` missing on bootstrap |
| `run-codedungeon-migrate` | Binary version ≠ DB `cd_version` — run `codedungeon migrate` |

## Database schema

SQLite FTS5 at `<project>/<provider.DBPath()>`. Key tables:

- `meta(key, value)` — bootstrap config (os, project_root, cd_version, model_reasoning, model_fast)
- `runs(id, feature, mode, project_mode, repo_map_json, created_at)` — one row per pipeline invocation
- `phases(run_id, phase, status, summary, decisions, artifacts, …)` — phase lifecycle rows
- `handoffs(run_id, phase, rendered_md, …)` — phase output snapshots
- `findings(run_id, cycle, severity, file, line_start, line_end, title, …)` — review findings (FTS5-indexed)
- `installed_artifacts(rel_path, sha256, binary_version, user_modified, installed_at)` — drift tracking

## Versioning

`main.go` sets `cmd.SetVersion(Version)` from a build flag. `setup`/`bootstrap` writes `cd_version` to `meta`. On every DB-touching command, `OpenDB()` checks binary version vs `meta.cd_version` — mismatch → `migration-required` action.

`codedungeon migrate` re-runs `installEmbeddedArtifacts` (preserving user-modified files) and bumps `cd_version`.

## Build + test

```bash
cd src/codedungeon
/usr/local/go/bin/go build -o ./codedungeon .
/usr/local/go/bin/go test ./...
```

Pure Go (`modernc.org/sqlite`) — no CGO, cross-compiles cleanly.

## Where to look first

- New to the codebase? Read `cmd/bootstrap.go` + `cmd/common.go` + `internal/provider/provider.go`.
- Adding a feature? Find the matching `cmd/<domain>.go` and look at how it calls `provider.Detect()`.
- Adding a new provider? Read `docs/PROVIDERS.md`.
