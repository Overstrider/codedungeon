# Phase 4: Task Decomposition

- Use the standalone swarm module: `codedungeon plan run --mode full --prompt "<feature>" --project-context ".codedungeon/project-context.md" --out ".codedungeon/task-planning/phase-4" --human-gate-policy material_ambiguity --legacy-phase4`.
- If project context is absent, use `.codedungeon/project-rules.compact.md`.
- Run `codedungeon plan validate --task-graph ".codedungeon/task-planning/phase-4/task-graph.json"`.
- The planner must produce `.codedungeon/plan/MASTER.md`, `.codedungeon/tasks/...`, SQLite planning telemetry, and an acyclic task graph.
- If the planner returns `NEEDS_USER_INPUT`, fail Phase 4 with the evaluator question instead of guessing.
- Close with `PHASE_4_COMPLETE`.
