# Phase 5.6: Test Task Decomposition

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Inputs
- Pipeline state: `.claude/plan/pipeline-state.md` (read this FIRST for config, repo map)
- Refined QA plans: `.claude/plan/{repo_name}qaplan.md` (from Phase 5.5)
- Existing PLAN.md files: `.claude/tasks/{feature}/{repo}/PLAN.md`

## Outputs
- Test task files: `.claude/tasks/{feature}/{repo}/test-NN-*.md`
- Updated PLAN.md files with `## Test Tasks` section
- Update `.claude/plan/pipeline-state.md`: set Phase 5.6 status to DONE

---

## Skip Condition

If Phase 5.5 was SKIPPED in pipeline-state.md, set Phase 5.6 to SKIPPED and return immediately.

---

## Goal

Create test task files from the refined qaplan so Phase 6 (test execution) has structured tasks to execute.

## Step 5.6.1: Spawn Task Architect (MODE=test)

Spawn a `general-purpose` agent with **model: `opus`** and **max_thinking_tokens: 32000**:

```
{CAVEMAN_ULTRA_BLOCK}

You are the task-architect operating in MODE=test.

Read your full instructions from: .claude/agents/task-architect.md

MODE=test

Read refined QA plans from .claude/plan/:
- {repo_name}qaplan.md for each affected repo (refined in Phase 5.5)

Read existing dev task files + MASTER.md at .claude/tasks/{feature}/ (do NOT modify them).

Read canonical task template: .claude/tasks/TEMPLATE.md
Read previous handoff: .claude/state/phase-55-output.md

Follow the MODE=test workflow in agent SKILL.md:
- Produce TEST-{LAYER}-NNN.md files per test layer (integration/api/e2e).
- Append `## Test Tasks` section to existing MASTER.md (preserve dev `## Tasks` block — do NOT rewrite dev side).
- Append `## Test Tasks` to each repo's PLAN.md.
- Write handoff `.claude/state/phase-56-output.md`.

FULLY AUTONOMOUS. No approval gates.

Final line MUST be exactly: TASKS_COMPLETE: {feature} — test — {N} test tasks across {M} repos

max_thinking_tokens: 32000
model: opus
```

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 5.6 \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_56_COMPLETE"
```

Writes DB row + `.claude/state/phase-56-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 5.6 --reason "..."` or `... fail 5.6 --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[5.6]` in the orchestrator (`codedungeon-dev-cycle.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
