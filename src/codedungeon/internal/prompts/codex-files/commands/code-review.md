# code-review

Use for standalone adversarial review of the current branch.

Review power:
- Cycles 1-3: full adversarial mode.
- Cycles 4-9: reduced mode. Keep personas, use fast model/effort, and focus on fixes/new diff.

Review order:
- Correctness and regressions.
- Security and data handling.
- Tests and missing coverage.
- Maintainability only when it creates concrete risk.

Output:
- Findings first, ordered by severity.
- Include file and line references.
- Include no finding if no actionable issue exists.
