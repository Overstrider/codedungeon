# Phase 5: Execution (Dev)

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Inputs
- Pipeline state: `.claude/plan/pipeline-state.md` (read this FIRST for config, repo map, env vars)
- `.claude/plan/MASTER.md` (execution order)
- `.claude/tasks/{feature-name}/{repo-name}/PLAN.md` per repo
- `.claude/tasks/{feature-name}/{repo-name}/task-*.md` (dev task files)
- `IMPECABLE_SKILL_PATH`, `IMPECABLE_REFS_PATH`, `IMPECABLE_AUDIT_PATH`, `IMPECABLE_POLISH_PATH` from pipeline state

## Outputs
- Feature branches with committed code per repo
- Pull requests per repo
- Adversarial review comments on PRs (via `/code-review`)
- Update `.claude/plan/pipeline-state.md`: set Phase 5 status to DONE + list artifacts + results
- Populate `## Results (per repo)` section with PR numbers, verdicts, task counts

---

### PHASE 5: Execution (Dev)

**Goal**: Execute dev tasks for each repo via codedungeon-loop, in execution order.

#### Step 5.1: Read Execution Order

Read the MASTER.md to determine repo execution order.

#### Step 5.2: Execute Each Repo (Sequential, Isolated)

For each repo in execution order:

1. Determine paths:
   ```
   # Multi-repo mode:
   TASK_DIR = .claude/tasks/{feature-name}/{repo-name}/

   # Single-repo mode (repo-name = "."):
   TASK_DIR = .claude/tasks/{feature-name}/root/

   PLAN_FILE = {TASK_DIR}/PLAN.md
   ```

2. **Resolve the absolute path to codedungeon-loop.md** before spawning the agent:
   - First check: `{project_root}/.claude/commands/codedungeon-loop.md`
   - Fallback: `$HOME/.claude/commands/codedungeon-loop.md`
   - Set `LOLDINIS_LOOP_PATH` to the first path that exists (use the Read tool to verify)
   - If neither exists, STOP with error: "codedungeon-loop.md not found. Cannot execute Phase 5."

3. Announce to user:
   > Spawning execution agent for **{repo}** ({lang}) — {N} tasks

4. Spawn a `general-purpose` agent with **model: `opus`**:

   ```
   You are executing the codedungeon-loop for a single repo.

   Read the full codedungeon-loop instructions from: {LOLDINIS_LOOP_PATH}
   (This is an ABSOLUTE path — read it with the Read tool before doing anything else.)
   If you cannot read the file above, STOP with error — do NOT improvise.

   Execute with these parameters:
   TASK_DIR = {TASK_DIR}
   IMPECABLE_SKILL_PATH = {IMPECABLE_SKILL_PATH}
   IMPECABLE_REFS_PATH = {IMPECABLE_REFS_PATH}
   IMPECABLE_AUDIT_PATH = {IMPECABLE_AUDIT_PATH}
   IMPECABLE_POLISH_PATH = {IMPECABLE_POLISH_PATH}

   Follow the codedungeon-loop protocol exactly:
   - Validate input (Step 0)
   - Branch setup (Main Loop Step 1)
   - Orchestrator loop: dispatch each task
     - {lang}-specialist (Plan mode) → general-purpose agent (exec) → {lang}-specialist (Review mode)
   - Commit per task
   - Push, create PR (with context from MASTER.md)
   - Run /code-review for PR review (Main Loop Step 5) — see REVIEW PROTOCOL below
   - Fix issues if needed (loop continues until APPROVED — no cycle cap stops the loop)

   ## REVIEW PROTOCOL — /code-review (adversarial, Opus 4.7 fanout)
   For Main Loop Step 5 (PR review), you MUST read and follow the full /code-review
   protocol from: $HOME/.claude/commands/code-review.md

   /code-review runs a multi-persona adversarial fanout (Saboteur + New Hire + Security Auditor
   + Spec Enforcer on Opus 4.7) followed by per-finding Sonnet Validators and a stack-specific
   {LANG}-specialist pass. It always produces a verdict (APPROVED or CHANGES_REQUESTED) — there
   is NO "skip" case.

   CRITICAL: This is the ONLY review path. Codex, OpenRouter, Qwen, and any external reviewers
   have been removed from the pipeline. Do NOT invent a fallback chain; /code-review is always
   available because it is pure Claude Code.

   ## NEVER SKIP (verified after you complete)
   - NEVER skip Phase C (specialist review) for any task — the orchestrator will check that review.md contains an APPROVED verdict for every [x] task
   - NEVER skip Main Loop Step 5 (/code-review) — the orchestrator will verify a review comment exists on the PR via `gh`
   - NEVER mark a task [x] without Phase C approval
   - NEVER report completion without providing ALL of these fields in your final report

   ## Required Report Format
   Your FINAL message must include these fields (the orchestrator parses them):
   TASKS_COMPLETED: {N}/{total}
   PR_NUMBER: {number}
   PR_URL: {url}
   REVIEW_VERDICT: {APPROVED | CHANGES_REQUESTED | MAX_CYCLES_REACHED}
   REVIEW_CYCLES: {N}
   BLOCKED_TASKS: {list or "none"}
   ```

5. **Verify Phase 5 output** before accepting:

   a. **Parse the agent's report** for required fields:
      - `PR_NUMBER`, `PR_URL`, `REVIEW_VERDICT`
      - If any field is missing: log WARNING "Agent did not report {field}. Verifying manually..."

   b. **Verify PR exists**:
      ```bash
      cd {REPO_DIR} && gh pr list --head {BRANCH_NAME} --json number -q '.[0].number'
      ```
      - If no PR: log ERROR "No PR found for {repo}. Phase 5 incomplete."
      - Do NOT proceed to Phase 6 for this repo.

   c. **Verify adversarial review was posted** (exact title match — /code-review always posts this):
      ```bash
      cd {REPO_DIR} && gh pr view {PR_NUMBER} --comments --json comments -q '[.comments[] | select(.body | test("Claude Adversarial Code Review"))] | length'
      ```
      - If count = 0: log ERROR "No adversarial review comment found on PR #{PR_NUMBER}. /code-review was not invoked — Phase 5 incomplete."
      - /code-review has no skip path, so zero comments indicates a pipeline break.

   d. Report verification result:
      > **{repo}** verified: PR #{PR_NUMBER} — Review: {REVIEW_VERDICT}

6. Move to next repo.

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 5 \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_5_COMPLETE"
```

Writes DB row + `.claude/state/phase-5-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 5 --reason "..."` or `... fail 5 --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[5]` in the orchestrator (`codedungeon-dev-cycle.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
