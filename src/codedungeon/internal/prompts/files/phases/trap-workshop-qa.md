# Phase 3.5: QA Planning

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Inputs
- Pipeline state: `.claude/plan/pipeline-state.md` (read this FIRST for config, repo map, env vars)
- Enriched domain plans: `.claude/plan/{repo_name}plan.md` (one per affected repo, produced by Phase 3)
- Per-repo CODEBASE_MAP: `{repo_path}/docs/CODEBASE_MAP.md`
- Per-repo CLAUDE.md (for test conventions and startup instructions)
- Playwright skill at `PLAYWRIGHT_SKILL_PATH` (if set)

## Outputs
- QA plan per repo: `.claude/plan/{repo_name}qaplan.md`
- Update `.claude/plan/pipeline-state.md`: set Phase 3.5 status to DONE + list artifacts

---

### PHASE 3.5: QA Planning (Parallel Isolated Agents)

**Goal**: Spawn the `basilisk-planner-qa` agent for each affected repo to produce QA test strategy documents.

#### Step 3.5.1: Spawn QA Planners in PARALLEL

For each affected repo (from arcplan.md meta → repos):

1. Look up `REPO_MAP[repo]` to get `lang`
2. **If `lang` == `kotlin`**: Skip this repo. Log: "No test infrastructure for {repo} (Kotlin) — skipping QA planning."
3. Spawn a `general-purpose` agent with **model: `opus`**:

   ```
   You are the basilisk-planner-qa for this project.

   Read your full instructions from: .claude/agents/basilisk-planner-qa.md

   If you cannot find or read the file above, STOP immediately and report:
   AGENT_DEFINITION_MISSING: .claude/agents/basilisk-planner-qa.md
   Do NOT improvise without your agent definition.

   Read the enriched domain plan from: .claude/plan/{repo_name}plan.md
   Read the repo's CLAUDE.md for test conventions and startup instructions.
   Read {repo_path}/docs/CODEBASE_MAP.md for comprehensive repo context.

   REPO INFO:
   - repo: {repo_name}
   - lang: {lang}
   - stack: {stack}
   - path: {repo_path}

   PLAYWRIGHT SKILL (E2E expert patterns):
   - Path: {PLAYWRIGHT_SKILL_PATH}
   - If the path is set, read this file BEFORE planning E2E tests.
     It contains selector priority, POM patterns, fixtures, configuration
     best practices, and anti-patterns. Use it to produce higher-quality
     E2E test plans.
   - If the path is empty, plan E2E tests with basic patterns (data-testid + flows).

   YOUR JOB:
   1. Detect the test framework from the repo's manifest/config files
   2. Search for existing test files and patterns
   3. Determine which test tiers apply:
      - Integration tests: only if the project has test infrastructure
      - API tests: only if this is a backend repo with HTTP endpoints
      - E2E tests: only if this is a frontend repo with ## Test Auth configured
        in CLAUDE.md OR where TEST_AUTH_BEING_CREATED = true
   4. Write the QA plan to: .claude/plan/{repo_name}qaplan.md
   5. Include EXACT curl commands for API tests (the mimic-tester-api agent executes them literally)
   6. Include a precise Definition of Done for each feature

   BE PRECISE. Plan only the tests that matter. Do not over-test.
   This plan will be used by test agents to create and run tests.
   ```

   **Test Auth Injection (Step 3.5.1 continued):** For each repo that is in `TEST_AUTH_MISSING_REPOS`, you MUST append the following block to that repo's basilisk-planner-qa prompt above, right before `YOUR JOB:`:

   ```
   TEST_AUTH_BEING_CREATED = true
   This repo does NOT have ## Test Auth in its CLAUDE.md yet, but it IS being
   created as a prerequisite dev task (task-01). Plan tests AS IF the endpoint
   exists. Use the standard spec:
   - Endpoint: POST /api/auth/test-login
   - Body: { "email": "$TEST_USER_EMAIL" }
   - Response: { "token": "...", "user": { "id": "...", "email": "..." } }
   - storageState: tests/e2e/.auth/user.json (frontend repos)
   ```

   If the repo is NOT in `TEST_AUTH_MISSING_REPOS`, do NOT include this block.

4. Launch ALL repos in PARALLEL (simultaneous tool calls)

#### Step 3.5.2: Continue

When ALL QA planners return, log to user:

> **Phase 3.5 complete.** QA plans written for: {list repos}. Continuing to Phase 4...

**Immediately proceed to Phase 4. Do NOT wait for approval.**

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 3.5 \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_35_COMPLETE"
```

Writes DB row + `.claude/state/phase-35-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 3.5 --reason "..."` or `... fail 3.5 --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[3.5]` in the orchestrator (`codedungeon-dev-cycle.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
