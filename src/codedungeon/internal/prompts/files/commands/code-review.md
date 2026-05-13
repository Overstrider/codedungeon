# CodeDungeon Code Review

## Project Rules Gate

Before reviewing or reporting completion, run `./.claude/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules.

Every review report and final handoff must include:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Use only `./.claude/bin/codedungeon` for this command.

## Standalone Module

Do not write review reports manually. The standalone module is the final adjudicator:

```bash
./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post
```

legacy `review run` output are invalid as final approval evidence. Never publish per-persona approvals as the PR verdict; post only the concise final adjudication from `codedungeon code-review`.

## Review Power

- Cycles 1-3: full mode. Use the configured reasoning model for persona recall and full PR diff review.
- Cycles 4-9: reduced mode. Keep all personas, use the configured fast model/effort, and focus on fixes or new diff since the previous review cycle.
- Never skip validators, classifier, stack specialist, PR posting, or verdict generation in reduced mode.

## Verification Evidence

Treat missing verification as BLOCKING. If a workflow claims completion but the PR report, task notes, or recent comments do not show concrete build/check/test evidence, emit an actionable finding for `missing verification`.

Required standard:

- Build/check/test evidence must name the command and result.
- For Rust changes, expect `cargo check` and `cargo test`.
- For changed `Dockerfile` or `Containerfile`, expect `podman build` or a documented environment blocker.
- APPROVED does not replace verification.

## Output Contract

Findings first, ordered by severity. Include file and line references. If no issue exists, include a concise no-finding summary and final adjudicator rationale. The PR comment marker is `CodeDungeon Code Review`.
