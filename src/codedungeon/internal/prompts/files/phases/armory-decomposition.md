# Phase 4: Task Decomposition

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Inputs
- Pipeline state: `.codedungeon/plan/pipeline-state.md` (read this FIRST for config, repo map, env vars)
- `.codedungeon/plan/arcplan.md` (architect's plan)
- `.codedungeon/plan/{repo_name}plan.md` for each affected repo (enriched domain plans)
- `.codedungeon/plan/{repo_name}qaplan.md` for each affected repo (QA test strategies, if they exist)
- `TEST_AUTH_MISSING_REPOS` and `TEST_AUTH_SPEC` from pipeline state (if populated)

## Outputs
- `.codedungeon/plan/MASTER.md` (master task list with execution order)
- `.codedungeon/tasks/{feature-name}/{repo-name}/PLAN.md` per repo
- `.codedungeon/tasks/{feature-name}/{repo-name}/task-*.md` (dev task files)
- `.codedungeon/tasks/{feature-name}/{repo-name}/test-task-*.md` (test task files, if qaplan exists)
- Update `.codedungeon/plan/pipeline-state.md`: set Phase 4 status to DONE + list artifacts + results

---

### PHASE 4: Task Decomposition (Task Architect, Autonomous)

**Goal**: Spawn `spider-architect-task` in MODE=dev to decompose enriched {repo}plan.md files into canonical task files per `.codedungeon/tasks/TEMPLATE.md` (200–500 tokens each) + MASTER.md + PLAN.md per repo.

#### Step 4.1: Spawn Task Architect (MODE=dev)

Use the Task tool to spawn a `general-purpose` agent with **model: `opus`** and **max_thinking_tokens: 32000**:

```
{CAVEMAN_ULTRA_BLOCK}

You are the spider-architect-task operating in MODE=dev.

Read your full instructions from: .claude/agents/spider-architect-task.md

MODE=dev

Read all plan files from .codedungeon/plan/:
- arcplan.md (architect's plan)
- {repo_name}plan.md for each affected repo (single-pass enriched, includes inline `#### {lang}` subsections — Phase 2' output)

Read canonical task template at: .codedungeon/tasks/TEMPLATE.md (must conform).
Read previous handoff: .codedungeon/state/phase-2prime-output.md (or phase-35-output.md if QA ran).

Follow the MODE=dev workflow in the agent SKILL.md:
- Produce MASTER.md (topo-sorted by depends-on).
- Produce `.codedungeon/tasks/{feature}/{repo}/PLAN.md` per repo.
- Produce `.codedungeon/tasks/{feature}/{repo}/TASK-NNN.md` per dev task (200–500 tokens each, template-conformant).
- Write handoff `.codedungeon/state/phase-4-output.md`.

CRITICAL: FULLY AUTONOMOUS. Execute all decomposition in one pass. No approval gate.

Final line MUST be exactly: TASKS_COMPLETE: {feature} — dev — {N} tasks across {M} repos

max_thinking_tokens: 32000
model: opus
```

**Test Auth Injection (Step 4.1 continued):** If `TEST_AUTH_MISSING_REPOS` is not empty, append this block to the spider-architect-task prompt above, right before `CRITICAL: FULLY AUTONOMOUS`:

```
TEST_AUTH_PREREQUISITE:
Repos needing test-auth endpoint as FIRST dev task (TASK-001):
Repos: {TEST_AUTH_MISSING_REPOS joined by comma}

For each repo listed, create TASK-001-test-auth-endpoint.md with:
- depends-on: [] (root task)
- All other dev TASK-NNN in this repo depend-on TASK-001.
- Task `What` steps + `Acceptance Criteria` pulled from TEST_AUTH_SPEC below.

Full TEST_AUTH_SPEC (paste into task.Context + task.What):
{paste the full TEST_AUTH_SPEC variable here — the entire spec stored in Step 0.4}

The TASK-001 must also state: update repo CLAUDE.md `## Test Auth` section documenting the endpoint (format in spec).
```

If `TEST_AUTH_MISSING_REPOS` is empty, do NOT include this block.

#### Step 4.2: Continue

When the agent returns, read the MASTER.md to get the execution order, then log to user:

> **Phase 4 complete.** Tasks generated. Continuing to Phase 5...
> - {repo1}: {N} tasks
> - {repo2}: {N} tasks
> - Execution order: {order}

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 4 \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_4_COMPLETE"
```

Writes DB row + `.codedungeon/state/phase-4-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 4 --reason "..."` or `... fail 4 --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[4]` in the orchestrator (`main-quest.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
