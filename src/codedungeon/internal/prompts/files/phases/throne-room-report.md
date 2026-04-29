# Phase 7: Finalization

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

**You are a phase agent.** Phase 7 is runner-owned and deterministic. Use `codedungeon run finalize`; do not write final reports manually, do not call `report render` directly, and do not call `phase done 7` yourself.

## Output mode

Caveman:ultra block is embedded - fetch it:

```bash
codedungeon prompts get caveman-ultra
```

Propagate the block into any sub-agent prompt you spawn.

---

## Step 1: Finalize

```bash
codedungeon run finalize > /tmp/throne-room-report.txt
```

Emit the contents to the user. The emitted final report must include, per repo:

```text
+------------------------------------------------+
| CodeDungeon PR Report                          |
+------------------------------------------------+
| Status        READY_FOR_USER_REVIEW|BLOCKED|MAX_CYCLES_REACHED
| Workflow      main-quest
| PR            #<number> <url>
| Branch        <branch>
| Review        APPROVED|CHANGES_REQUESTED|MAX_CYCLES_REACHED|NOT_RUN
| Cycles        <n>/9 | last mode: full|reduced|not_run
+------------------------------------------------+

Summary
<1-line task/result summary>

Review
- Adversarial comments: <n>
- Last review marker: Claude Adversarial Code Review|none
- Remaining findings: <none or short list/count>

Work Done
- Tasks: <n>/<total or n/a>
- Changed files: <short summary or none>
- Verification: <commands/results or blocker>

PR
<url or "not created">

Next
<none or exact next human/agent action>
```

---

## Tool Discipline

Allowed: `Bash` for `codedungeon`, `git`, and `gh`; `Read` for state and handoff files.
Forbidden: `Write`/`Edit` on final report artifacts. `codedungeon run finalize` handles DB state, report evidence, and Phase 7 handoff.

## Failure

If `codedungeon run finalize` fails, report the exact blocker and do not mark phases, write report evidence, or synthesize READY_FOR_USER_REVIEW. Open non-runner telemetry may be marked ABORTED so stale delegated agents are visible to the next run.
