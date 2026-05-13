# codedungeon

Official Claude Code router for CodeDungeon workflows.

Usage:

```text
/codedungeon [--full|--lite|--oneshot|--one-shot|--auto|--rules] <prompt>
```

Compatibility aliases remain available:

- `/main-quest` is the same workflow as `/codedungeon --full`.
- `/side-quest` is the same workflow as `/codedungeon --lite`.
- `/one-shot` is the same workflow as `/codedungeon --oneshot`.

## Router Contract

Parse `$ARGUMENTS` as mode flags plus the remaining user prompt.

Mode flags:

- `--full`: select `main-quest`.
- `--lite`: select `side-quest`.
- `--oneshot`: select `one-shot`.
- `--one-shot`: compatibility spelling for `--oneshot`.
- `--auto`: explicit automatic selection.
- `--rules`: run Project Rules Discovery. This mode may run without a user prompt.

Validation:

1. If more than one mode flag is present, stop with:

   ```text
   multiple mode flags supplied
   Usage: /codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>
   ```

2. If the prompt is empty after removing the mode flag and mode is not `--rules`, stop with:

   ```text
   prompt required
   Examples:
     /codedungeon --full implement OAuth across the API and web app
     /codedungeon --lite execute .codedungeon/plans/payment-fix.md
     /codedungeon --oneshot fix the typo in README
   ```

3. In `--lite` mode, require a prior plan in `.codedungeon/plans/*.md` or an explicit plan path in the prompt. If no plan exists, stop and ask for a plan first.

4. In `--rules` mode, run Project Rules Discovery:

   - Deep-read README, `AGENTS.md`, `CLAUDE.md`, docs, manifests, test configs, CI configs, env examples, Dockerfile/Containerfile files, and existing `.codedungeon/project-rules.md` if present.
   - Write `.codedungeon/project-rules.md` with status `DRAFT`.
   - Present the draft to the user for review. Do not mark rules approved without explicit user confirmation.
   - After confirmation, run `codedungeon rules approve` and `codedungeon rules compact`.
   - Ensure `.codedungeon/project-rules.compact.md` contains `PROJECT_RULES_STATUS: APPROVED`.

5. Before following the selected workflow, print:

   ```text
   CODEDUNGEON_MODE_SELECTED: <mode> - <reason>
   ```

6. For `full`, `lite`, and `oneshot`, start or resume the agent-first workflow state:

   ```bash
   ./.claude/bin/codedungeon run --full --prompt "<prompt>"
   ./.claude/bin/codedungeon run --lite --prompt "<prompt>"
   ./.claude/bin/codedungeon run --oneshot --prompt "<prompt>"
   ```

   The runner creates or resumes the run/session and returns JSON with `current_step`, `blockers`, `timeline`, and `next_action`. Execute the returned next action with native Claude tools, then record progress with `codedungeon run advance --step <step> --status completed --summary "<summary>" --artifact <path>`. Mid-flow blockers are soft; `run finalize` remains the hard gate for `READY_FOR_USER_REVIEW`.

## Auto Selection

When no mode flag is provided, behave as `--auto`.

Select `full` when the request is complex, multi-repo, architectural, or explicitly needs QA, tests, phase lifecycle, or a final report.

Select `lite` when a plan already exists under `.codedungeon/plans/*.md` and the prompt asks to execute, split, or continue simple planned work.

Select `oneshot` for small direct changes where task splitting would be overhead.

## Dispatch

After selecting the mode, call the target agent-first state runner exactly:

- `full`: `./.claude/bin/codedungeon run --full --prompt "<prompt>"`.
- `lite`: `./.claude/bin/codedungeon run --lite --prompt "<prompt>"`.
- `oneshot`: `./.claude/bin/codedungeon run --oneshot --prompt "<prompt>"`.
- `rules`: run Project Rules Discovery inline from this router contract.

Code-writing dispatches must end through an open GitHub PR ready for human review. CodeDungeon must not merge PRs.

Do not remove or rewrite the compatibility aliases. `/codedungeon` is the promoted surface, while `/main-quest`, `/side-quest`, and `/one-shot` stay supported.

## Project Rules Gate

Before dispatching `full`, `lite`, or `oneshot`, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules`; do not silently invent project rules.

Every dispatched workflow must preserve this Project Rules envelope in plans, task files, review reports, phase handoffs, and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```
