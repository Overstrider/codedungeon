---

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```
name: code-review
description: Run a standalone codedungeon-style adversarial review in Codex CLI.
---

# code-review

Use for reviewing the current branch or an implementation diff.

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

Output findings first, ordered by severity. Include file and line references. If there are no actionable findings, say so directly.
