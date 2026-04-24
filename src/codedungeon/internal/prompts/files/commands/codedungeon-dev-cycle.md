# Loldinis Dev Cycle

Thin state-machine orchestrator. Dispatches isolated phase agents; reads only `pipeline-state.md` + handoffs. All deterministic mechanics (state, handoffs, repo discover, review pipeline, plan parsing, QA, report) live in `codedungeon`.

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
- External dep unavailable (`gh` CLI missing, git not init) — STOP.
- Protected branch violation mid-run — STOP.

**Not valid stop reasons**: `MAX_CYCLES_REACHED` (escalate), "good enough", soft errors, test/build failures (fix tasks, re-enter).

If in doubt: **continue**.

---

## Parameters

- `$ARGUMENTS` — feature prompt/description.

## Prerequisites

- `codedungeon` bootstrapped in project (run `codedungeon bootstrap --reasoning <id> --fast <id>` once).
- `.claude/` contains agents + skills + phases (installed by bootstrap).
- `gh` CLI installed + authenticated.
- `/codedungeon-loop` and `/codedungeon-test-loop` available.

---

## Pipeline state

**Single source of truth**: `.claude/codedungeon.db` (SQLite). Human view: `.claude/plan/pipeline-state.md` regenerated via `codedungeon phase render-state`.

---

## Execution: state-machine loop

### Step 0: Validate input

If `$ARGUMENTS` empty:
> Usage: `/codedungeon-dev-cycle <feature description>`
> Tip: `@sphinx-prompt-enhancer` first to refine.

STOP.

Store as `FEATURE_PROMPT`.

### Step 1: Bootstrap + resume detection

```bash
# Ensure codedungeon alive in project.
if [ ! -x .claude/bin/codedungeon ] || [ ! -f .claude/codedungeon.db ]; then
  "$HOME/.claude/plugins/local/codedungeon/bin/codedungeon" bootstrap \
    --reasoning claude-opus-4-7 --fast claude-sonnet-4-6
fi
CD=./.claude/bin/codedungeon

# Check for existing run with same feature (resume).
EXISTING_FEATURE=$($CD phase config feature 2>/dev/null || echo "")
if [ "$EXISTING_FEATURE" = "$FEATURE_PROMPT" ]; then
  # RESUME — skip phase init, pick up at first non-DONE.
  NEXT=$($CD phase next | jq -r .next_phase)
else
  # FRESH — new run (cleanup old if different feature).
  $CD cleanup --tasks --plans --reviews 2>/dev/null || true
  $CD phase init --feature "$FEATURE_PROMPT" --branch "feat/$(slug "$FEATURE_PROMPT")" --mode FRESH --project-mode SINGLE
  NEXT="0"
fi
```

### Step 2: Dispatch loop

```
PHASES = [0, 1, "2'", 3.5, 4, 5, 5.5, 5.6, 6, 7]
PHASE_FILE_MAP = {
  0:   ".claude/phases/entrance-hall-validation.md",
  1:   ".claude/phases/war-room-architect.md",
  "2'": ".claude/phases/guild-quarter-domain.md",
  3.5: ".claude/phases/trap-workshop-qa.md",
  4:   ".claude/phases/armory-decomposition.md",
  5:   ".claude/phases/forge-execution.md",
  5.5: ".claude/phases/crucible-qa-refine.md",
  5.6: ".claude/phases/laboratory-test-decomp.md",
  6:   ".claude/phases/arena-tests.md",
  7:   ".claude/phases/throne-room-report.md"
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
      - `.claude/phases/{file}` (installed by bootstrap) — **primary**.
      - `$HOME/.claude/plugins/local/codedungeon/commands/phases/{file}` — fallback if project not yet migrated.

   b. Resolve model for this phase:
      ```bash
      MODEL=$($CD config model "${PHASE_TIER[$N]}")
      ```

   c. Spawn `general-purpose` agent with `model: $MODEL` and `max_thinking_tokens: PHASE_THINKING[N]`:

      ```
      You are executing Phase {N} of the codedungeon-dev-cycle pipeline.

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

4. After all phases processed → render final report.

### Step 3: Final report

```bash
$CD phase render-state        # refresh pipeline-state.md view
$CD report render             # write phase-7 formatted report to stdout
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

**DB is the context bridge.** Every phase calls `codedungeon phase done` → atomically updates DB + writes `.claude/state/phase-{N}-output.md` + refreshes `pipeline-state.md`.

---

## Interruption + resume

Re-running `/codedungeon-dev-cycle` with the same prompt auto-resumes from the first non-DONE phase. Start over = `codedungeon cleanup --all` then re-invoke.

## What this command does NOT do

- Refine prompt (use `@sphinx-prompt-enhancer`).
- Merge PRs / deploy (human).
- Read plan files / git state directly (phase agents do via `codedungeon`).
- Stop for approval (fully autonomous).
