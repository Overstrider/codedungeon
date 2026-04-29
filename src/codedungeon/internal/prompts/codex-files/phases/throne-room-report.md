# Phase 7: Finalization

You are a phase agent. Phase 7 is runner-owned and deterministic: do not write final reports manually, do not call `report render` directly, and do not call `phase done 7` yourself.

## Step 1: Delegate Finalization

Run the finalizer. It verifies phase, review, PR, git, QA, and report gates; closes eligible final phases; records Phase 7; and emits the final report.

```bash
./.codex/bin/codedungeon run finalize > /tmp/throne-room-report.txt
```

Emit the report contents to the user. The report must include a CodeDungeon PR Report block with PR, review, cycles, work done, and verification evidence.

## Failure

If `codedungeon run finalize` fails, report the exact blocker and do not mark phases, write report evidence, or synthesize READY_FOR_USER_REVIEW. Open non-runner telemetry may be marked ABORTED so stale delegated agents are visible to the next run.
