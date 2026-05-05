# Phase 6: Execution (Tests)

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Inputs
- Pipeline state: `.codedungeon/plan/pipeline-state.md` (read this FIRST for config, repo map, env vars)
- `.codedungeon/plan/MASTER.md` (execution order)
- `.codedungeon/tasks/{feature-name}/{repo-name}/PLAN.md` per repo (check for `## Test Tasks` section)
- `PLAYWRIGHT_SKILL_PATH` from pipeline state

## Outputs
- Test results per repo (integration, API, E2E)
- Code fixes committed (if test failures triggered dev loop re-entry)
- Update `.codedungeon/plan/pipeline-state.md`: set Phase 6 status to DONE + list artifacts + results

---

### PHASE 6: Execution (Tests)

**Goal**: Execute test tasks for each repo via codedungeon-test-loop. Brings the project up, runs tests, fixes bugs automatically.

Multi-repo QA must run sequentially per repo. When a repo has concrete verification commands, record them through the QA module with `codedungeon qa run --cwd <repo> --phase 6 --fresh --cmd "<first cmd>"`, then continue that repo's remaining commands before moving to the next repo.

#### Step 6.1: Check for Test Tasks

For each repo in execution order:
1. Read the repo's PLAN.md
2. Check if `## Test Tasks` section exists
3. If no test tasks: Log "No test tasks for {repo} — skipping test phase." → continue to next repo

#### Step 6.2: Execute Test Loop per Repo

For each repo that has `## Test Tasks` in PLAN.md:

1. **Resolve the absolute path to codedungeon-test-loop.md**:
   - Use: `{project_root}/.codedungeon/commands/codedungeon-test-loop.md`
   - If it does not exist, STOP with error: "codedungeon-test-loop.md not found. Run `codedungeon install`."

2. Spawn a `general-purpose` agent with **model: `opus`**:

   ```
   You are executing the codedungeon-test-loop for a single repo.

   Read the full codedungeon-test-loop instructions from: {resolved absolute path}
   (This is an ABSOLUTE path — read it with the Read tool before doing anything else.)

   Execute with these parameters:
   TASK_DIR = {TASK_DIR}
   PLAYWRIGHT_SKILL_PATH = {PLAYWRIGHT_SKILL_PATH}
   (Pass this to wraith-tester-frontend agents for E2E test writing guidance)

   Follow the codedungeon-test-loop protocol exactly:
   - Step 0: Validate input
   - Step 1: Start the project (phoenix-project-startup agent)
   - Step 2: Integration tests (optional — skip if no test infra)
   - Step 3: API curl tests (backend repos only)
   - Step 4: E2E tests (frontend repos only)
   - Step 5: All tests passed → commit test files, push, report

   If startup-fix or code-fix tasks are generated:
   - The test loop handles re-entering the dev loop automatically
   - Max 9 total cycles

   ## NEVER SKIP (verified after you complete)
   - NEVER skip Step 1 (project startup) — you must attempt to start the project
   - NEVER skip test execution if test tasks exist — run them even if you expect failures
   - NEVER report "no tests to run" if PLAN.md has a ## Test Tasks section with pending items

   ## Required Report Format
   Your FINAL message must include:
   PROJECT_STARTED: {true | false — reason}
   INTEGRATION_TESTS: {N passed, N failed | SKIPPED — reason}
   API_TESTS: {N passed, N failed | SKIPPED — reason}
   E2E_TESTS: {N passed, N failed | SKIPPED — reason}
   CODE_FIXES_APPLIED: {N}
   UNRESOLVED_FAILURES: {N}
   ```

3. **Verify Phase 6 output** before accepting:

   a. **Parse the agent's report** for required fields:
      - `PROJECT_STARTED`, at least one test tier result
      - If `PROJECT_STARTED: false` and no reason given: log WARNING "Project startup may have been skipped"
      - If all test tiers say SKIPPED but PLAN.md has test tasks: log WARNING "Tests may have been skipped for {repo}"

   b. If the test loop reports code-fix or startup-fix tasks that could not be resolved:
      - Log: "Tests found unresolved issues in {repo}. {N} remaining failures."
      - Continue to next repo (do not block the pipeline)

4. Log: "{repo} tests complete: {results summary}"

#### Step 6.3: Continue

When all repos are processed, log to user:

> **Phase 6 complete.** Test execution finished for all repos. Continuing to Phase 7...

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 6 \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_6_COMPLETE"
```

Writes DB row + `.codedungeon/state/phase-6-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 6 --reason "..."` or `... fail 6 --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[6]` in the orchestrator (`main-quest.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
