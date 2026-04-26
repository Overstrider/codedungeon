---
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
