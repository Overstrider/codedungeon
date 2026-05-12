# code-review

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. Missing, draft, or stale rules are soft blockers while the agent is shaping work, but finalization must not claim READY_FOR_USER_REVIEW without the required Project Rules envelope.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Use for standalone adversarial review of a URL/PR/diff with explicit project and task context.

Deterministic evidence:
- Do not write review reports manually.
- Run the standalone module: `./.codex/bin/codedungeon code-review --url <PR URL> --project-context <path-or-text> --task-context <path-or-text> --out .codedungeon/code-review --post`.
- The module owns persona execution, final adjudication, rendering, posting, and integrity evidence.
- A review with no findings is valid only when every persona provides a substantive approval and the final adjudicator explicitly declares `APPROVED`.
- Empty template reviews, `_None._`-only reviews, missing adjudicator decisions, or legacy `review run` output are invalid.

Telemetry:
- The standalone module records review evidence and integrity; telemetry never replaces the review gate.
- If you spawn any supplemental reviewer, validator, classifier, or specialist outside the module, record it with `./.codex/bin/codedungeon trace agent-start` before spawn and `./.codex/bin/codedungeon trace agent-end` after it returns.

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
- If no issue exists, include a concise no-finding summary and final adjudicator rationale. Never publish per-persona approvals in the PR comment.
