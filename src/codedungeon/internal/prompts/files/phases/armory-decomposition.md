# Phase 4: Task Decomposition

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This phase should not spawn provider-specific task architects directly.

Run task decomposition through the standalone `codedungeon plan` swarm module.

Required command shape:

```bash
codedungeon plan run \
  --mode full \
  --prompt "<same user prompt / run feature>" \
  --project-context ".codedungeon/project-context.md" \
  --out ".codedungeon/task-planning/phase-4" \
  --human-gate-policy material_ambiguity \
  --legacy-phase4
```

If `.codedungeon/project-context.md` is absent, use `.codedungeon/project-rules.compact.md`.

Then run:

```bash
codedungeon plan validate --task-graph ".codedungeon/task-planning/phase-4/task-graph.json"
codedungeon plan status --session "phase-4"
```

Close with `codedungeon phase done 4` and `PHASE_4_COMPLETE` only after `.codedungeon/plan/MASTER.md` and `.codedungeon/tasks/...` compatibility artifacts exist.

Do not spawn a provider-specific task architect directly. Planning agents may not edit source code.
