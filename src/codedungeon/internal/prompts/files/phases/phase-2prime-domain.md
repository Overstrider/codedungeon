# Phase 2': Consolidated Domain + Specialist Planning

**You are a phase agent.** Read these instructions, execute them, then update pipeline-state.md.

## Tools
tools: Task, TodoWrite, Read
(orchestrator — no Write/Edit)

## Inputs
- `.claude/plan/pipeline-state.md` (config, repo map, env vars)
- `.claude/plan/arcplan.md` (affected repos, sections)
- `.claude/state/phase-1-output.md` (canonical handoff from Phase 1)
- Per-repo CODEBASE_MAP: `{repo_path}/docs/CODEBASE_MAP.md` (existing project only)
- Per-repo CLAUDE.md

## Outputs
- Domain plan per repo: `.claude/plan/{repo_name}plan.md` (single-pass — no separate `#### {lang}` enrichment phase; the consolidated agent writes inline `#### {lang}` subsections in the same file)
- `.claude/state/phase-2prime-output.md` (canonical handoff, ≤ 500 tokens)
- `.claude/plan/pipeline-state.md` row: `| 2' | DONE | {artifact list} | {notes} |`

---

### PHASE 2': Consolidated domain + specialist (parallel isolated skills)

**Goal**: Spawn the single consolidated skill per affected repo, all in PARALLEL. Replaces Phase 2 + Phase 3. One pass produces the fully-enriched plan.

#### Repo → skill mapping

| repo lang/framework | skill |
|---|---|
| rust backend (axum/sqlx) | `rust-backend-agent` |
| nextjs frontend | `nextjs-frontend-agent` |
| kotlin multiplatform app | `kotlin-app-agent` |

For a repo whose stack has no consolidated skill yet (e.g., go / python / elixir / cpp backend): fall back to the legacy `{lang}-specialist` review-only reviewer + the existing `planner.md` generic domain planner chained in the old way. Phase 2' consolidated path is only for the 3 stacks above.

#### Step 2'.1: Read arcplan.md

Read `.claude/plan/arcplan.md`. Identify affected repos from `## meta → repos:`.

#### Step 2'.2: Spawn consolidated skills in PARALLEL

For each affected repo with a consolidated skill, spawn via Task in parallel.

Spawn prompt template (fill `{repo_name}`, `{repo_path}`, `{skill_name}`, `{model}`, `{project_mode}`):

```
{CAVEMAN_ULTRA_BLOCK}

You are the {skill_name} operating in MODE=plan.

Load the skill: Skill(name="{skill_name}")

If the skill cannot be loaded, STOP and report:
SKILL_DEFINITION_MISSING: skills/{skill_name}/SKILL.md
Do NOT improvise.

MODE=plan
PROJECT_MODE={project_mode}   # BOOTSTRAP | SINGLE | MULTI

Read:
- .claude/plan/arcplan.md → section `## repo:{repo_name}` + `## cross-repo`
- .claude/state/phase-1-output.md (canonical handoff)
- {repo_path}/docs/CODEBASE_MAP.md (if exists)
- {repo_path}/CLAUDE.md (if exists)

YOUR JOB (plan mode, single-pass):
1. Follow the plan workflow in SKILL.md.
2. Read companion files on demand per the SKILL.md guidance — do NOT front-load them all.
3. Write the fully-enriched plan to .claude/plan/{repo_name}plan.md (domain structure + inline `#### {lang}` subsections per change).
4. Final line of the file MUST be exactly: PLAN_COMPLETE: {repo_name}plan.md

max_thinking_tokens: 8000
model: {model}
```

For the 3 consolidated skills, spawn with `model: opus`.

**Impecable Frontend Injection**: If `IMPECABLE_SKILL_PATH` is set AND skill is `nextjs-frontend-agent`, append the existing impecable block to the prompt (keep same injection rules as legacy Phase 3).

#### Step 2'.3: Write handoff

When ALL skills return, write `.claude/state/phase-2prime-output.md`:

```
# phase-2prime-output

Phase: 2'
Status: DONE
Summary: domain + specialist enrichment single-pass. Produced {N} plans.

Artifacts Produced:
- .claude/plan/{repo1}plan.md
- .claude/plan/{repo2}plan.md
- ...

Key Decisions:
- [per-repo 1-line decision, e.g. "backend: axum + sqlx, newtype IDs, cursor pagination"]

Traps:
- [any missing reference code / deps scan gap]

Open Questions:
- [any arcplan ambiguity surfaced by skills]

Next Phase Input: .claude/plan/{repo_name}plan.md files + this handoff.

PHASE2PRIME_COMPLETE
```

#### Step 2'.4: Continue

Log to user:
> Phase 2' complete. Plans single-pass for: {list repos}. Continuing to Phase 3.5 (QA).

---

## Output mode + completion

```bash
codedungeon prompts get caveman-ultra   # inject CAVEMAN block into any sub-agent spawn
```

When this phase is DONE, close it atomically:

```bash
codedungeon phase done 2' \
  --summary "<1-line caveman>" \
  --decisions "<d1>" "<d2>" \
  --artifacts "<path1>" "<path2>" \
  --next "<path the next phase must read first>" \
  --promise "PHASE_2PRIME_COMPLETE"
```

Writes DB row + `.claude/state/phase-2prime-output.md` + updates `pipeline-state.md`.

Use `codedungeon phase skip 2' --reason "..."` or `... fail 2' --reason "..."` for non-DONE terminal states.

## Tool discipline

Phase-agent = orchestrator. Allowed: `Task` (spawn workers), `Read` (state + handoff files), `Bash` (for `codedungeon` + `git` + tool calls). Forbidden: `Write`/`Edit` on artifact files (arcplan.md, plans, task files, review files) — workers own those.

Thinking budget inherited from `PHASE_THINKING[2']` in the orchestrator (`codedungeon-dev-cycle.md`). Model tier via `codedungeon config model <reasoning|fast>` (Sprint 7).
