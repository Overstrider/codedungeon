# Main Quest

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. Missing, draft, or stale rules are soft blockers while the agent is shaping work, but finalization must not claim READY_FOR_USER_REVIEW without the required Project Rules envelope.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

Thin state-machine orchestrator. Dispatches isolated phase agents; reads only `pipeline-state.md` + handoffs. All deterministic mechanics (state, handoffs, repo discover, review pipeline, plan parsing, QA, report) live in `codedungeon`.

Deterministic completion gates:
- Use only `./.claude/bin/codedungeon` for CodeDungeon commands.
- Do not write review reports manually.
- Do not write final reports manually.
- Run standalone review per repo/PR with `./.claude/bin/codedungeon code-review --out .codedungeon/code-review/<repo> --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context .codedungeon/tasks/<feature>/<repo>/PLAN.md --post`.
- Run verification with `./.claude/bin/codedungeon qa run --phase 6 --fresh`; in multi-repo workflows run sequential per-repo QA with `./.claude/bin/codedungeon qa run --cwd <repo> --phase 6 --fresh`.
- Run `./.claude/bin/codedungeon run finalize`; READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`.

This workflow is agent-first. Start or resume durable state with:

```bash
./.claude/bin/codedungeon run --full --prompt "<prompt>"
```

**FULLY AUTONOMOUS** once invoked. No approval gates.

## CAVEMAN:ULTRA mode (forced)

```bash
codedungeon prompts get caveman-ultra
```
Propagate verbatim into every sub-agent spawn. Applies to all narration/logs/state-notes. Exempt: code blocks, file contents (arcplan/plans/tasks/reviews/CLAUDE.md updates), commit/PR text, security warnings, error quotes.

---

## Absolute guarantees

1. **NEVER on main/develop/master** — Phase 0 creates feat branch.
2. **PR mandatory** — Phase 5 creates it.
3. **Push before exit mandatory** — verified by `codedungeon git verify`.
4. Phases 0, 1, 4, 5, 7 CANNOT be skipped. Phases 2', 3.5, 5.5, 5.6, 6 can be skipped by the phase agent based on context.

### Valid stop reasons (only)
- Design decision needed — STOP, log.
- Local git repo unavailable — STOP.
- Missing GitHub `origin` or `gh` auth — continue as a finalization blocker surfaced by `codedungeon run status` / `codedungeon run finalize --dry-run`.
- Protected branch violation mid-run — STOP.

**Not valid stop reasons**: `MAX_CYCLES_REACHED` (escalate), "good enough", soft errors, test/build failures (fix tasks, re-enter).

If in doubt: **continue**.

---

## Parameters

- `$ARGUMENTS` — feature prompt/description.

## Prerequisites

- `codedungeon` bootstrapped in project (run `codedungeon bootstrap --reasoning <id> --fast <id>` once).
- `.claude/` contains agents + skills, and `.codedungeon/` contains editable commands + phases (installed by setup/bootstrap).
- GitHub `origin` remote exists.
- `gh` CLI installed + authenticated.
- `/codedungeon-loop` and `/codedungeon-test-loop` available.

---

## Pipeline state

**Single source of truth**: `.codedungeon/codedungeon.db` (SQLite). Human view: `.codedungeon/plan/pipeline-state.md` regenerated via `codedungeon phase render-state`.

---

## Execution: state-machine loop

### Step 0: Validate input

If `$ARGUMENTS` empty:
> Usage: `/main-quest <feature description>`
> Tip: `@sphinx-prompt-enhancer` first to refine.

STOP.

Store as `FEATURE_PROMPT`.

### Step 1: Bootstrap + resume detection

```bash
# Ensure codedungeon alive in project.
if [ ! -x .claude/bin/codedungeon ] || [ ! -f .codedungeon/codedungeon.db ]; then
  echo "Status BLOCKED: run project-local codedungeon setup before /main-quest"
  exit 2
fi
CD=./.claude/bin/codedungeon

git rev-parse --is-inside-work-tree >/dev/null || { echo "Status BLOCKED: CodeDungeon requires a local git repo"; exit 2; }
$CD run --full --prompt "$FEATURE_PROMPT"

# A run and custody session now exist or resumed. Do not run phase init or create another run.
NEXT=$($CD phase next | jq -r .next_phase)
```

### Step 2: Dispatch loop

```
PHASES = [0, 1, "2'", 3.5, 4, 5, 5.5, 5.6, 6, 7]
PHASE_FILE_MAP = {
  0:   ".codedungeon/phases/entrance-hall-validation.md",
  1:   ".codedungeon/phases/war-room-architect.md",
  "2'": ".codedungeon/phases/guild-quarter-domain.md",
  3.5: ".codedungeon/phases/trap-workshop-qa.md",
  4:   ".codedungeon/phases/armory-decomposition.md",
  5:   ".codedungeon/phases/forge-execution.md",
  5.5: ".codedungeon/phases/crucible-qa-refine.md",
  5.6: ".codedungeon/phases/laboratory-test-decomp.md",
  6:   ".codedungeon/phases/arena-tests.md",
  7:   ".codedungeon/phases/throne-room-report.md"
}

# Model tier per phase (deep thinking vs fast). Resolved at runtime via config.
PHASE_TIER = {
  0: "fast", 1: "reasoning", "2'": "reasoning", 3.5: "fast",
  4: "reasoning", 5: "fast", 5.5: "fast", 5.6: "reasoning",
  6: "fast", 7: "fast"
}

PHASE_THINKING = {
  0: 0, 1: 32000, "2'": 8000, 3.5: 2000, 4: 32000,
  5: 2000, 5.5: 2000, 5.6: 32000, 6: 2000, 7: 0
}

CLEAR_BETWEEN_PHASES = True
```

**For each phase in order:**

1. Query `codedungeon phase info <N>` → status.
2. Status `DONE` or `SKIPPED` → skip to next.
3. Status `PENDING`:

   a. Resolve phase file path:
      - `.codedungeon/phases/{file}` — editable project-local phase file installed by CodeDungeon.

   b. Resolve model for this phase:
      ```bash
      MODEL=$($CD config model "${PHASE_TIER[$N]}")
      ```

   c. Spawn `general-purpose` agent with `model: $MODEL` and `max_thinking_tokens: PHASE_THINKING[N]`:

      ```
      You are executing Phase {N} of the main-quest pipeline.

      Read your full phase instructions from: {ABSOLUTE_PHASE_FILE_PATH}
      Read the pipeline state: `codedungeon phase info <N>`  and  `codedungeon phase info <PREV_N>` for last phase's handoff.

      Execute the phase. When done, call `codedungeon phase done <N> ...` (or skip/fail).

      $(codedungeon prompts get caveman-ultra)

      max_thinking_tokens: {PHASE_THINKING[N]}
      model: {MODEL}
      ```

      No inline instructions. Phase file + DB state + handoff = everything needed.

   d. Agent returns → log: `"Phase {N}: $(codedungeon phase info <N> | jq -r .phase.status) — $(codedungeon phase info <N> --field summary)"`.
   e. If `CLEAR_BETWEEN_PHASES` and not last phase: emit `/clear` transition marker.
   f. Continue.

4. After Phase 6 completes, return control to `codedungeon run`; finalization is handled by `codedungeon run finalize`.

### Step 3: Final report

```bash
$CD phase render-state        # refresh pipeline-state.md view
$CD run finalize              # close Phase 7 and write final report under gates
```

---

## Architecture: isolated phases

```
ORCHESTRATOR (thin — reads only `codedungeon phase info/next`)
  │
  ├─ Phase 0 agent → validation + repo discover + bootstrap       [fast]
  ├─ Phase 1 agent → architect → arcplan.md                        [reasoning, think 32k]
  ├─ Phase 2' skills (parallel) → {repo}plan.md                    [reasoning, think 8k]
  ├─ Phase 3.5 agents → qaplan                                     [fast]
  ├─ Phase 4 spider-architect-task → MASTER.md + task files               [reasoning, think 32k]
  ├─ Phase 5 → codedungeon-loop per repo → code + PR + /code-review   [fast, escalate on stuck]
  ├─ Phase 5.5 → qa refine                                         [fast]
  ├─ Phase 5.6 → test spider-architect-task                               [reasoning, think 32k]
  ├─ Phase 6 → codedungeon-test-loop per repo                         [fast]
  └─ Phase 7 → report                                              [fast, think 0]
```

**DB is the context bridge.** Every phase calls `codedungeon phase done` → atomically updates DB + writes `.codedungeon/state/phase-{N}-output.md` + refreshes `pipeline-state.md`.

---

## Interruption + resume

Re-running `/main-quest` with the same prompt auto-resumes from the first non-DONE phase. Start over = `codedungeon cleanup --all` then re-invoke.

## What this command does NOT do

- Refine prompt (use `@sphinx-prompt-enhancer`).
- Merge PRs / deploy (human).
- Read plan files / git state directly (phase agents do via `codedungeon`).
- Stop for approval (fully autonomous).
