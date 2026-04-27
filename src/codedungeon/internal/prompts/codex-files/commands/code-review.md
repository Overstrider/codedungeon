# code-review

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Use for standalone adversarial review of the current branch.

Deterministic evidence:
- Do not write review reports manually.
- Write `review-manifest.json` with personas, base/head SHA, PR number, and timestamp.
- Ensure each persona writes its own output, including `findings-saboteur.json`, before aggregation.
- Run `./.codex/bin/codedungeon review run` to generate `review.md` and `review.json`.

Review power:
- Cycles 1-3: full adversarial mode.
- Cycles 4-9: reduced mode. Keep personas, use fast model/effort, and focus on fixes/new diff.

Review order:
- Correctness and regressions.
- Security and data handling.
- Missing verification: treat absent build/check/test evidence as BLOCKING.
- Tests and missing coverage.
- Maintainability only when it creates concrete risk.

If a workflow claims completion without concrete build/check/test evidence, report `missing verification` as BLOCKING. The report must name the absent command class. For Rust changes, expect `cargo check` and `cargo test`. For changed `Dockerfile` or `Containerfile`, expect `podman build` or a documented environment blocker. `APPROVED does not replace verification`.

Output:
- Findings first, ordered by severity.
- Include file and line references.
- Include no finding if no actionable issue exists.
