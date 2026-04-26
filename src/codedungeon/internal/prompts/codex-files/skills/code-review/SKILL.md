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
- Missing or weak tests.
- Maintainability only when it creates concrete risk.

Output findings first, ordered by severity. Include file and line references. If there are no actionable findings, say so directly.
