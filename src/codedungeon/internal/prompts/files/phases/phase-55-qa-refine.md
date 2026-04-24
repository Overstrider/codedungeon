# Phase 5.5: QA Refinement (Post-Dev)

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Inputs
- Pipeline state: `.claude/plan/pipeline-state.md` (read this FIRST for config, repo map, env vars)
- Existing QA plans: `.claude/plan/{repo_name}qaplan.md` (from Phase 3.5)
- Actual implemented code (from Phase 5)

## Outputs
- Refined `.claude/plan/{repo_name}qaplan.md` (updated with real selectors, paths, labels)
- Update `.claude/plan/pipeline-state.md`: set Phase 5.5 status to DONE

---

## Skip Condition

If Phase 3.5 was SKIPPED in pipeline-state.md (no qaplans exist), set Phase 5.5 to SKIPPED and return immediately.

---

## Goal

The qa-planner re-reads the ACTUAL implemented code and updates the qaplan with real values. This bridges the gap between theoretical planning (Phase 3.5) and accurate testing (Phase 6).

## Step 5.5.1: Spawn QA Refinement Agents in PARALLEL

Read pipeline-state.md to find which repos had qaplans. For each:

Spawn a `general-purpose` agent with **model: `opus`**:

```
You are the qa-planner in REFINEMENT MODE.

Read your full instructions from: .claude/agents/qa-planner.md

If you cannot find or read the file above, STOP immediately and report:
AGENT_DEFINITION_MISSING: .claude/agents/qa-planner.md
Do NOT improvise without your agent definition.

Read the EXISTING qaplan from: .claude/plan/{repo_name}qaplan.md
Read the repo's CLAUDE.md for conventions.
Read {repo_path}/docs/CODEBASE_MAP.md for comprehensive repo context.

MODE: REFINEMENT (post-dev — code has been implemented)

YOUR JOB:
1. Read the existing qaplan (written during Phase 3.5 based on the domain plan)
2. Read the ACTUAL implemented code:
   - Scan form components for real field labels, button text, error messages
   - Scan API routes for real endpoint paths and response shapes
   - Check if input masks were actually implemented
   - Check if empty/loading/error states were actually implemented
   - Find actual CSS classes, data-testid attributes, ARIA labels
3. UPDATE the qaplan with real values:
   - Replace theoretical selectors with actual selectors from the code
   - Replace placeholder labels with actual field labels
   - Replace placeholder error messages with actual error text
   - Update API endpoint paths if they differ from the plan
   - Add/remove test flows based on what was actually built vs planned
   - In ## frontend-ux-checks: update input masks with actual implementations
4. Write the refined qaplan back to: .claude/plan/{repo_name}qaplan.md

RULES:
- Keep the qaplan structure intact (same sections, same format)
- Only UPDATE values — do not remove test flows unless the feature wasn't implemented
- Add new test flows if the implementation added features not in the original plan
- Mark any planned test that CAN'T be tested (feature not implemented) as SKIP with reason
```

Launch ALL repos in PARALLEL.

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 5.5 \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_55_COMPLETE"
```

Writes DB row + `.claude/state/phase-55-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 5.5 --reason "..."` or `... fail 5.5 --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[5.5]` in the orchestrator (`codedungeon-dev-cycle.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
