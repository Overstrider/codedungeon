# Minidungeon

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

Lightweight pipeline. Reads a Claude Code plan (`.claude/plans/*.md`), splits into tasks, runs the ralph loop (codedungeon-loop), runs adversarial code review, ends with approved PR. No architect, no QA, no tests, no report — just plan → split → execute → review → PR.

**Deterministic mechanics (branch guard, plan parsing, PR creation, fix-task generation) delegated to `codedungeon`. Only LLM work (task decomposition, specialist plan/exec/review, persona fanout) is inline.**

## Protected-branch rule (ABSOLUTE)

**NEVER commit, push, or write code on `main`, `master`, `develop`, `dev`, `staging`, `production`, `release`.**

Every `git commit` / `git push` preceded by:

```bash
codedungeon git guard --repo "$REPO_DIR"
```

Exits 1 if protected → HARD STOP.

## Parameters

- `$ARGUMENTS` — optional path to a specific plan file. If omitted, uses most recently modified `.claude/plans/*.md`.

## Prerequisites

- Claude Code plan file exists (from plan mode)
- `codedungeon` bootstrapped in project (`.claude/bin/codedungeon` + `.claude/codedungeon.db`)
- `gh` CLI authenticated
- Project is a git repo

---

## Architecture

```
MINIDUNGEON (single orchestrator)
  │
  ├─ Step 0: Resolve plan source (.claude/plans/*.md)
  ├─ Step 1: Discover repo lang/stack
  ├─ Step 2: Decompose plan → PLAN.md + TASK-NNN.md files
  │
  └─ Step 3: Spawn codedungeon-loop (ralph loop)
       ├─ Branch setup
       ├─ Per task: specialist plan → exec → specialist review
       ├─ Commit + push + PR
       └─ /code-review adversarial fanout
            ├─ APPROVED → DONE
            └─ CHANGES_REQUESTED → fix tasks → re-enter loop
```

---

## Execution

### Step 0: Resolve plan source

```bash
if [ -n "$ARGUMENTS" ] && [ -f "$ARGUMENTS" ]; then
  PLAN_FILE="$ARGUMENTS"
else
  PLAN_FILE=$(ls -t .claude/plans/*.md 2>/dev/null | head -1)
fi
[ -z "$PLAN_FILE" ] && { echo "No plan found. Use Claude Code plan mode first, then re-run /minidungeon."; exit 2; }
```

Read `PLAN_FILE` fully. Extract feature name from first `# ` heading or filename.

Generate branch-safe slug: `FEATURE_SLUG=$(echo "$FEATURE_NAME" | tr '[:upper:]' '[:lower:]' | sed 's/[^a-z0-9]/-/g' | sed 's/--*/-/g' | head -c 50)`.

### Step 1: Discover repo

```bash
CD=.claude/bin/codedungeon
DISCOVER=$($CD repo discover --root . --persist=false 2>/dev/null)
LANG=$(echo "$DISCOVER" | jq -r '.repo_map[0].lang // empty')
REPO_NAME=$(echo "$DISCOVER" | jq -r '.repo_map[0].name // empty')
```

Fallback if `codedungeon repo discover` fails — check manifest files:

| File | Lang |
|------|------|
| `go.mod` | go |
| `Cargo.toml` | rust |
| `package.json` + `next.config.*` | nextjs |
| `package.json` (no next) | typescript |
| `build.gradle.kts` | kotlin |
| `mix.exs` | elixir |
| `requirements.txt` / `pyproject.toml` | python |
| `CMakeLists.txt` / `Makefile` (with .cpp) | cpp |

`REPO_DIR` = project root (cwd or nearest `.git` ancestor).

If `REPO_NAME` empty → use basename of `REPO_DIR`.

### Step 2: Decompose plan into tasks

Create task directory:

```bash
mkdir -p .claude/tasks/mini
```

Read the Claude Code plan file content. Decompose into discrete implementation tasks.

**Decomposition rules:**
1. Each logical change unit → one task (file creation, API endpoint, refactor, config change)
2. Target 200–500 tokens per task file
3. Max 5 acceptance criteria per task — split if more
4. Merge trivially-coupled steps (e.g., create handler + register route) into one task
5. Tasks numbered `TASK-001`, `TASK-002`, ... in execution order from plan
6. `depends:` references earlier TASK-xxx when ordering matters

**Write `.claude/tasks/mini/PLAN.md`:**

```markdown
# Plan: <FEATURE_NAME>
# Repo: <REPO_NAME>
# Lang: <LANG>

## Tasks
- [ ] TASK-001 <title>
- [ ] TASK-002 <title>
- [ ] TASK-003 <title>
...
```

Headers `# Plan:`, `# Repo:`, `# Lang:` are **required** — `codedungeon plan meta` parses them.

Task lines `- [ ] TASK-NNN <title>` are **required** — `codedungeon plan meta` counts them.

**Write `.claude/tasks/mini/TASK-NNN.md`** per task:

```markdown
# TASK-NNN: <title>

## Meta
lang: <LANG>
depends: <TASK-xxx or none>
priority: medium
estimated_complexity: <low|medium|high>

## Context
<2-3 lines — what exists today, what's missing, from the plan>

## Detailed Requirements
<3-7 bullet steps — concrete actions, not vague goals>

## Files
<files to create or modify, with paths>

## Done when
<1-line observable completion criteria>

## Review checklist
- [ ] Requirements implemented and tested
- [ ] No regressions introduced
```

**Verify decomposition:**

```bash
$CD plan meta .claude/tasks/mini/PLAN.md
```

Must return valid JSON with `"ok": true`, correct `total_tasks`, `pending` = total, `done` = 0.

### Step 3: Invoke codedungeon-loop (ralph loop)

Resolve loop instructions:

```bash
LOOP_PATH=".claude/commands/codedungeon-loop.md"
[ -f "$LOOP_PATH" ] || LOOP_PATH=$($CD prompts get codedungeon-loop --path 2>/dev/null || echo "")
```

If loop instructions not found → HARD STOP: "codedungeon-loop.md not found. Run `codedungeon install`."

Spawn a **single `general-purpose` agent** (model: opus) with this prompt:

```
Read the full codedungeon-loop instructions from: {LOOP_PATH}
(Use the Read tool to read the file before doing anything else.)

Execute with these parameters:
  TASK_DIR = .claude/tasks/mini/

Follow the codedungeon-loop protocol exactly — branch setup, per-task specialist
plan/exec/review cycle, commit, push, PR creation, /code-review adversarial
fanout, and fix loop on CHANGES_REQUESTED.

Feature branch: feat/{FEATURE_SLUG}

Report these fields when done:
  TASKS_COMPLETED: N/M
  PR_NUMBER: #X
  PR_URL: https://...
  REVIEW_VERDICT: APPROVED|CHANGES_REQUESTED|MAX_CYCLES_REACHED
  REVIEW_CYCLES: N
  BLOCKED_TASKS: list or "none"
```

Wait for the agent to complete. Parse its output for the required fields.

### Step 4: Report

Emit final summary:

```
MINIDUNGEON_COMPLETE
TASKS_COMPLETED: N/M
PR_NUMBER: #X
PR_URL: https://...
REVIEW_VERDICT: APPROVED
REVIEW_CYCLES: N
```

---

## Failure modes

| Error | Action |
|-------|--------|
| No plan file found | STOP — "No plan found. Use Claude Code plan mode first, then /minidungeon." |
| Protected branch detected | HARD STOP (codedungeon git guard) |
| `gh pr create` fails | HARD STOP (via codedungeon-loop) |
| 9 adversarial cycles without APPROVED | MAX_CYCLES_REACHED, exit 3, human triage |
| Task stuck 9 worker iterations | Mark `[!]` blocked, continue to next task |
| codedungeon-loop.md not found | STOP — "Run codedungeon install" |
| `codedungeon plan meta` returns invalid | STOP — verify PLAN.md has required headers |

## Resume

If `/minidungeon` is re-invoked and `.claude/tasks/mini/PLAN.md` exists with some `[x]` tasks:
- Skip Step 2 (tasks already exist)
- Re-enter Step 3 — codedungeon-loop picks up from first `[ ]` task
- To start fresh: delete `.claude/tasks/mini/` first
