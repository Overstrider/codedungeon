# Phase 7: Push Verification + Final Report

You are a phase agent. This phase is deterministic: use `codedungeon` for all state and report work.

## Step 1: Verify GitHub PR state

For each repo in the run's repo map, run:

```bash
./.codex/bin/codedungeon git verify --repo "$REPO_DIR" --branch "$BRANCH_NAME"
```

Expected `ok: true`. If any repo fails, do not render the final report and do not mark Phase 7 done. CodeDungeon has no local-only completion path: GitHub PR, pushed branch, and adversarial review evidence are mandatory.

## Step 2: Render final report

```bash
# BOOTSTRAP mode:
./.codex/bin/codedungeon report render --bootstrap > /tmp/throne-room-report.txt

# SINGLE or MULTI mode:
./.codex/bin/codedungeon report render > /tmp/throne-room-report.txt
```

Emit the report contents to the user. The report must include a CodeDungeon PR Report block with PR, review, cycles, work done, and verification evidence.

## Step 3: Mark phase complete

```bash
./.codex/bin/codedungeon phase done 7 \
  --verdict APPROVED \
  --summary "push verified, final report emitted" \
  --artifacts ".codedungeon/plan/pipeline-state.md" \
  --promise "PHASE_7_COMPLETE: pipeline done"
```

## Failure

If `codedungeon git verify` returns `ok: false` for any repo, fail the phase:

```bash
./.codex/bin/codedungeon phase fail 7 --reason "repo X: missing PR, pushed branch, or adversarial review comment"
```
