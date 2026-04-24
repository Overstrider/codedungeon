# codedungeon

Deterministic Go CLI for Claude Code project pipelines. Absorbs LLM-narrated mechanics (state parsing, review dedupe, repo discovery, QA assertions, report rendering) into a single binary. LLMs keep the judgment work.

> Project-scoped. Requires `.git/`. Refuses to run under `~/.claude`. State in SQLite (FTS5). Prompts + agents + skills + commands embedded via `go:embed` and auto-installed into each project.

---

## Repo layout

```
.
├── src/codedungeon/          # Go source (source of truth)
│   ├── go.mod
│   ├── main.go
│   ├── cmd/                  # cobra subcommands
│   ├── internal/
│   │   ├── db/               # SQLite + migrations + FTS5 triggers
│   │   ├── manifest/         # lang + framework detection
│   │   ├── osadapter/        # OS DI (linux/windows/darwin/fallback)
│   │   ├── prompts/          # go:embed FS + artifact walker
│   │   │   └── files/        # all shipped prompts/agents/skills/commands/phases
│   │   └── reviewpipe/       # dedupe + filter + classify + render + verdict
│   └── testdata/             # fixtures
│
├── release/                  # shippable artifacts (run ./install.sh)
│   ├── README.md             # end-user install/usage doc
│   ├── install.sh            # OS-detecting installer
│   ├── bin/                  # pre-compiled binaries (4 targets)
│   └── skills/codedungeon-cli/SKILL.md
│
├── OLD/                      # pre-refactor snapshot (gitignored)
├── Makefile                  # build / test / release / install
├── plan.md                   # historic planning doc (kept for context)
├── README.md                 # (this file — dev/source overview)
└── .gitignore
```

**Consumers** use `release/` + `./install.sh`. **Developers** work in `src/`.

---

## Build

```bash
make build           # linux-amd64 only (local dev)
make release         # all 4 platforms + re-sync skill into release/
make test            # go test ./...
make install         # build linux + run release/install.sh
make clean           # rm release/bin/*
```

Override version: `VERSION=v0.9.0 make release`.

---

## Architecture

### Binary + skill (pair)

1. **Binary** (`release/bin/codedungeon`) — one Go binary, 15+ subcommands. Self-contained (pure-Go SQLite, no CGO).
2. **Skill** (`release/skills/codedungeon-cli/SKILL.md`) — Claude Code skill instructing LLM agents how to invoke the CLI for the full pipeline.

Installer puts both into `~/.claude/plugins/local/codedungeon/`. Claude Code auto-discovers the plugin (manifest + skill + slash commands).

### First-run (per project)

```bash
cd /path/to/git/project
codedungeon bootstrap --reasoning claude-opus-4-7 --fast claude-sonnet-4-6
```

Bootstrap:
1. Guards: refuses under `~/.claude`, requires `.git/`.
2. Self-copies binary → `<project>/.claude/bin/codedungeon`.
3. Creates DB → `<project>/.claude/codedungeon.db`.
4. Installs embedded tree → `<project>/.claude/{agents,skills,commands,phases}/`.
5. Records `meta.{os, project_root, cd_version, model_reasoning, model_fast}`.
6. Seeds 9 base prompts into DB.

### Project pipeline (end-to-end)

```
/codedungeon-dev-cycle "<feature>"
  ↓
Phase 0 (validation + repo discover)
Phase 1 (architect → arcplan.md)                     [reasoning model]
Phase 2' (domain plan per repo)                      [reasoning]
Phase 3.5 (qa plan)                                  [fast]
Phase 4 (task decomposition → MASTER.md)             [reasoning]
Phase 5 (dev loop per repo → code + PR + /code-review) [fast, escalate]
Phase 5.5 (qa refine)                                [fast]
Phase 5.6 (test task decomp)                         [reasoning]
Phase 6 (test loop per repo)                         [fast]
Phase 7 (final report)                               [fast]
```

Each phase is an isolated agent. Context bridges = DB handoffs + `.claude/loldinis-state/phase-{N}-output.md`. Orchestrator reads only `codedungeon phase next/info`.

### Deterministic vs judgment split

| Work | Implementation |
|---|---|
| Parse state, render reports, dedupe findings, scan manifests, run curl+assert, generate fix tasks, version/migrate prompts | `codedungeon` Go binary |
| Architect plans, domain plans, bug hunting (review personas), semantic classification, test plan authoring, Playwright specs | LLM (Claude agents) |

### OS adapter (DI)

```
internal/osadapter/
├── adapter.go      # Adapter interface
├── linux.go        # //go:build linux
├── windows.go      # //go:build windows (hints: C:\tools\gh\gh.exe)
├── darwin.go       # //go:build darwin
└── fallback.go     # //go:build !(linux|windows|darwin)
```

Commands call `adapter.FindTool()` / `adapter.RunExec()` / `adapter.RunShell()` — zero `if runtime.GOOS` scattered.

### SQLite + FTS5 schema (v3)

- `meta` — schema_version, os, project_root, cd_version, model_reasoning, model_fast, bootstrapped_at.
- `runs` — one per pipeline invocation.
- `phases` — 10 rows per run, status lifecycle.
- `handoffs` — phase-N-output cache + structured fields.
- `prompts` — versioned (embedded + user overrides).
- `tasks` — per-run per-repo task metadata.
- `findings` — code-review findings (cross-run history).
- `installed_artifacts` — sha256 tracking of installed agents/skills/commands/phases.
- FTS5 virtual tables + triggers: handoffs, prompts, findings, tasks.

---

## Token delta (migrated files)

| File | Before | After | Δ |
|---|---:|---:|---:|
| `phase-0-validation.md` | 504 | 187 | −63% |
| `phase-7-report.md` | 244 | 71 | −71% |
| `phase-1-architect.md` | 254 | 180 | −29% |
| `phase-2prime-domain.md` | 193 | 143 | −26% |
| `phase-35-qa.md` | 199 | 125 | −37% |
| `phase-4-decomp.md` | 168 | 115 | −32% |
| `phase-5-execution.md` | 235 | 160 | −32% |
| `phase-55-qa-refine.md` | 174 | 100 | −43% |
| `phase-56-test-decomp.md` | 142 | 89 | −37% |
| `phase-6-tests.md` | 198 | 124 | −37% |
| `code-review.md` | 410 | 183 | −55% |
| `codedungeon-loop.md` | 778 | 241 | −69% |
| `codedungeon-dev-cycle.md` | 354 | 198 | −44% |
| `codedungeon-test-loop.md` | 471 | 168 | −64% |
| `cleanup-tasks.md` | 111 | 45 | −59% |
| `test-api.md` (agent) | 283 | 110 | −61% |
| **Total** | **4718** | **2239** | **−53%** |

---

## Dev workflow

```bash
# Edit source
vim src/codedungeon/cmd/review.go

# Test
make test

# Rebuild linux-only for local iteration
make build

# Full release build (all 4 targets)
make release

# Re-install into local Claude Code plugin
make install
```

**Source-of-truth layering**:
1. Go source in `src/codedungeon/` — edit here.
2. Embedded tree in `src/codedungeon/internal/prompts/files/` — shipped IN the binary.
3. `release/bin/*` + `release/skills/codedungeon-cli/` — committed release artifacts.
4. `~/.claude/plugins/local/codedungeon/` — installed copy (symlinked back to `release/` in this dev setup; end-users get a real copy via `install.sh`).

---

## License

Apache-2.0.
