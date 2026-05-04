# Phase 0: Validation + Auto-Discovery + Codebase Mapping + Continuation Detection

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

**You are a phase agent.** Most work is deterministic — use `codedungeon` and cartographer. LLM judgment only for stack selection in BOOTSTRAP mode.

## Output mode

```bash
codedungeon prompts get caveman-ultra
```

Propagate the block verbatim into any sub-agent prompt.

EXEMPT (write normal): code blocks, FILE CONTENTS (arcplan, plans, tasks, CLAUDE.md updates), commit messages, PR titles/bodies, security warnings, error messages quoted exact.

---

## Inputs
- `$ARGUMENTS` — user's feature prompt
- root `CLAUDE.md` (optional; discover may return insertion guidance, but does not edit it)
- per-repo `CLAUDE.md` files
- per-repo `docs/CODEBASE_MAP.md` (cartographer)

## Outputs
- REPO_MAP (persisted to `runs.repo_map`)
- PROJECT_MODE (BOOTSTRAP / SINGLE / MULTI)
- MODE (FRESH / APPEND)
- `TEST_AUTH_MISSING_REPOS` (if any)
- `PLAYWRIGHT_SKILL_PATH` var
- per-repo `docs/CODEBASE_MAP.md` created/refreshed

---

## Step 0.0: Bootstrap codedungeon (first-run in project)

`codedungeon` is project-scoped. Its binary must live at `<project>/.claude/bin/codedungeon` and its DB at `<project>/.codedungeon/codedungeon.db`.

```bash
# If the project already has codedungeon bootstrapped, skip.
if [ -x .claude/bin/codedungeon ] && [ -f .codedungeon/codedungeon.db ]; then
  CD=./.claude/bin/codedungeon
else
  # Need git at project root. If missing, STOP and ask user.
  if [ ! -d .git ] && [ ! -f .git ]; then
    echo '{"error":"no git repo","action":"user-must-init-git","hint":"codedungeon only runs in a git project; run: git init && git commit --allow-empty -m init"}'
    exit 2
  fi
  echo "Status BLOCKED: run project-local codedungeon setup before Phase 0"
  exit 2
fi
```

If `bootstrap` returns a provider-home-config refusal, the agent is in the wrong directory; `cd` to the real project root first.

## Step 0.1: Validate Input

1. If `$ARGUMENTS` empty:
   > Usage: `/main-quest <feature description>`
   >
   > Tip: `@sphinx-prompt-enhancer` first to refine.

   STOP.

2. Store as `FEATURE_PROMPT`.

## Step 0.1.2: Detect Playwright skill

```bash
PLAYWRIGHT_SKILL_PATH=""
for p in \
  ".claude/skills/crystal-ball-e2e/SKILL.md"; do
  [ -f "$p" ] && PLAYWRIGHT_SKILL_PATH="$p" && break
done
```

Missing skill is non-blocking. Log + continue.

## Step 0.2: Discover repos (auto-detect single/multi/bootstrap)

```bash
# A run already exists under autonomous custody. Do not call phase init here.
codedungeon repo discover --persist > /tmp/discover.json
cat /tmp/discover.json
```

The command:
- Auto-classifies: `BOOTSTRAP` (empty root), `SINGLE` (manifest at root, no sub-manifests), `MULTI` (sub-manifests found).
- Persists REPO_MAP into active run.
- Does not mutate root CLAUDE.md/AGENTS.md. If provider instruction updates are needed, use the returned `agent_config_instruction` content for the installer agent to insert.

Parse `project_mode` from the JSON. If `BOOTSTRAP`, prompt the user for stack:

> Empty project detected. What stack should this project use?
> Examples: "Rust + Actix", "Next.js", "Go + Chi", "Python + FastAPI", "Elixir + Phoenix", "Kotlin + Compose Multiplatform", "C++ + CMake"

From user's answer, build a 1-entry REPO_MAP JSON file, write it to a temp file, and re-init:

```bash
cat > /tmp/repo-map.json <<'EOF'
[{"name":".","path":".","lang":"rust","framework":"actix","stack":"Rust + Actix","specialist":"rust-specialist","domain_planner":"planner"}]
EOF
# Keep /tmp/repo-map.json as bootstrap planning input. Do not re-run phase init.

# Initialize git if empty:
cd "$(pwd)" && test -d .git || (git init && git commit --allow-empty -m "chore: initial commit")
```

Skip Steps 0.2.5 and 0.4 in BOOTSTRAP mode (no codebase to map, no Test Auth to check yet).

## Step 0.2.5: Ensure codebase maps exist

For each repo in REPO_MAP (skip in BOOTSTRAP):

1. Check `{repo.path}/docs/CODEBASE_MAP.md` exists.
2. Check freshness (read `last_mapped` frontmatter vs `git log --oneline --since="{last_mapped}" | head -5`).
3. If missing OR stale, run cartographer:
   ```bash
   codedungeon map "{repo.path}" --format json > /tmp/scan.json
   ```
   Then spawn Explore (sonnet) subagents in parallel over file groups; synthesize into `{repo.path}/docs/CODEBASE_MAP.md`.

## Step 0.3: Detect continuation vs fresh

```bash
PREV_FEATURE=$(codedungeon phase config feature 2>/dev/null || echo "")
```

- Se `PREV_FEATURE == FEATURE_PROMPT` semanticamente → **MODE=APPEND** (já inicializado; não reset).
- Se diferente E `.codedungeon/tasks/` tem conteúdo → **MODE=FRESH**: delete `.codedungeon/tasks/*` e `.codedungeon/plan/*`; manter `.codedungeon/codedungeon.db` (histórico preservado; FTS5 search continua funcional).

## Step 0.4: Test-auth prerequisite check

Skip in BOOTSTRAP.

```bash
codedungeon repo check-test-auth > /tmp/auth.json
cat /tmp/auth.json
# {"ok":true, "missing":["backend"], "present":["portal"], "spec":"run `codedungeon prompts get test-auth-spec`"}
```

If `missing` não vazio:
```bash
codedungeon prompts get test-auth-spec > /tmp/test-auth-spec.md
```

Salvar `TEST_AUTH_MISSING_REPOS` (será lido em Phase 4 para injetar TASK-001).

---

## Completion

```bash
codedungeon phase done 0 \
  --summary "repos=$(jq -r '.repo_map | length' /tmp/discover.json); mode=$MODE; project_mode=$PROJECT_MODE" \
  --decisions "playwright=$PLAYWRIGHT_SKILL_PATH" \
  --artifacts ".codedungeon/codedungeon.db" \
  --next ".codedungeon/plan/arcplan.md (Phase 1)" \
  --promise "PHASE_0_COMPLETE: $(jq -r '.project_mode' /tmp/discover.json), $(jq -r '.repo_map | length' /tmp/discover.json) repos"
```

`codedungeon phase done 0` atomically:
- Marks phase 0 = DONE in DB
- Writes `.codedungeon/state/phase-0-output.md` (handoff for Phase 1)
- Upserts into `runs`, `phases`, `handoffs` tables

## Tool discipline

Allowed: `Bash` (for `codedungeon`, `git`), `Task` (for Explore subagents — codebase mapping ONLY), `Read` (state/handoff files).
Forbidden: `Write`/`Edit` on `pipeline-state.md` or `phase-0-output.md` — `codedungeon` handles those.

## Failure

If discover fails or required tools missing:
```bash
codedungeon phase fail 0 --reason "<specific reason>"
```
Orchestrator halts on FAIL.
