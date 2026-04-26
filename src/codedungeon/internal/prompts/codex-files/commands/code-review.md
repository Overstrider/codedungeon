# code-review

Use for standalone adversarial review of the current branch.

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
