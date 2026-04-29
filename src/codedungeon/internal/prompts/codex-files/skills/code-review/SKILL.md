---
name: code-review
description: Run a standalone codedungeon-style adversarial review in Codex CLI.
---

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

# code-review

Use for standalone adversarial review of a URL/PR/diff with explicit project and task context.

Deterministic evidence:
- Do not write review reports manually.
- Run the standalone module: `./.codex/bin/codedungeon code-review --url <PR URL> --project-context <path-or-text> --task-context <path-or-text> --out .codedungeon/code-review --post`.
- The module owns persona execution, final adjudication, concise rendering, posting, and integrity evidence.
- PR comments are concise final adjudication only; persona outputs are evidence artifacts, not report sections.
- A review with no findings is valid only when every persona provides a substantive approval and the final adjudicator explicitly declares `APPROVED`.
- Empty template reviews, `_None._`-only reviews, missing adjudicator decisions, or legacy `review run` output are invalid.

Telemetry:
- The standalone module records review evidence and integrity; telemetry never replaces the review gate.
- If you spawn any supplemental reviewer, validator, classifier, or specialist outside the module, record it with `./.codex/bin/codedungeon trace agent-start` before spawn and `./.codex/bin/codedungeon trace agent-end` after it returns.

Review power:
- Cycles 1-3: full adversarial mode.
- Cycles 4-9: reduced mode. Keep personas, use fast model/effort, and focus on fixes/new diff.

Review order:
- Correctness regressions.
- Security and data handling.
- Missing verification: treat absent build/check/test evidence as BLOCKING.
- Missing or weak tests.
- Maintainability only when it creates concrete risk.

If a workflow claims completion without concrete build/check/test evidence, report `missing verification` as BLOCKING. The report must name the absent command class. For Rust changes, expect `cargo check` and `cargo test`. For changed `Dockerfile` or `Containerfile`, expect `podman build` or a documented environment blocker. `APPROVED does not replace verification`.

Output findings first, ordered by severity. Include file and line references. If no issue exists, include a concise no-finding summary and final adjudicator rationale. Never publish per-persona approvals in the PR comment.
