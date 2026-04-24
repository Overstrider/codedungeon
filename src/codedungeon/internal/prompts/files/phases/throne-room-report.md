# Phase 7: Push Verification + Final Report

**You are a phase agent.** This phase is almost fully deterministic — use `codedungeon` for everything. No plan/architect work here.

## Output mode

Caveman:ultra block is embedded — fetch it:

```bash
codedungeon prompts get caveman-ultra
```

Propagate the block into any sub-agent prompt you spawn.

---

## Step 1: Per-repo verification

For each repo in the run's REPO_MAP:

```bash
codedungeon git verify --repo "$REPO_DIR" --branch "$BRANCH_NAME"
```

Expected `ok: true`. If any `ok: false`, record the failure and do **not**
render the report until fixed (absolute guarantee: all commits pushed, PR
exists, adversarial review posted).

## Step 2: Render final report

```bash
# BOOTSTRAP mode:
codedungeon report render --bootstrap > /tmp/throne-room-report.txt

# SINGLE or MULTI mode:
codedungeon report render > /tmp/throne-room-report.txt
```

Emit the contents to the user.

## Step 3: Mark phase complete

```bash
codedungeon phase done 7 \
  --verdict APPROVED \
  --summary "push verified, final report emitted" \
  --artifacts ".claude/plan/pipeline-state.md" \
  --promise "PHASE_7_COMPLETE: pipeline done"
```

This atomically updates the DB, writes `.claude/state/phase-7-output.md`,
and sets phase 7 = DONE in `pipeline-state.md`.

---

## Tool discipline

Allowed: `Bash` (for `codedungeon` + `git` + `gh` only), `Read` (state/handoff files).
Forbidden: `Write`/`Edit` on any artifact — `codedungeon phase done` handles the
handoff + state file atomically.

## Failure

If `codedungeon git verify` returns `ok: false` for any repo, do NOT mark Phase 7
DONE. Report the blocker (missing PR / missing review / unpushed commits) and
FAIL:

```bash
codedungeon phase fail 7 --reason "repo X: missing adversarial review comment"
```

The orchestrator halts on FAIL.
