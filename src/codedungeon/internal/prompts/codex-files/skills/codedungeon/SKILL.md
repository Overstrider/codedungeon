---
name: codedungeon
description: Route Codex CLI requests to the official CodeDungeon workflow using --full, --lite, --oneshot, --rules, or automatic selection.
---

# codedungeon

Official Codex workflow router.

Usage:

```text
$codedungeon [--full|--lite|--oneshot|--one-shot|--auto|--rules] <prompt>
```

Compatibility aliases remain available:

- `$main-quest` is the same workflow as `$codedungeon --full`.
- `$side-quest` is the same workflow as `$codedungeon --lite`.
- `$one-shot` is the same workflow as `$codedungeon --oneshot`.

Use the project-local CLI for deterministic checks: `./.codex/bin/codedungeon`.

## Router Contract

Parse the invocation as mode flags plus the remaining user prompt.

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
   Usage: $codedungeon [--full|--lite|--oneshot|--auto|--rules] <prompt>
   ```

2. If the prompt is empty after removing the mode flag and mode is not `--rules`, stop with:

   ```text
   prompt required
   Examples:
     $codedungeon --full implement OAuth across the API and web app
     $codedungeon --lite execute .codedungeon/plans/payment-fix.md
     $codedungeon --oneshot fix the typo in README
   ```

3. In `--lite` mode, require a prior plan in `.codedungeon/plans/*.md` or an explicit plan path in the prompt. If no plan exists, stop and ask for a plan first.

4. In `--rules` mode, run Project Rules Discovery:

   - Deep-read README, `AGENTS.md`, `CLAUDE.md`, docs, manifests, test configs, CI configs, env examples, Dockerfile/Containerfile files, and existing `.codedungeon/project-rules.md` if present.
   - Write `.codedungeon/project-rules.md` with status `DRAFT`.
   - Present the draft to the user for review. Do not mark rules approved without explicit user confirmation.
   - After confirmation, run `./.codex/bin/codedungeon rules approve` and `./.codex/bin/codedungeon rules compact`.
   - Ensure `.codedungeon/project-rules.compact.md` contains `PROJECT_RULES_STATUS: APPROVED`.

5. Before following the selected workflow, print:

   ```text
   CODEDUNGEON_MODE_SELECTED: <mode> - <reason>
   ```

6. For `full`, `lite`, and `oneshot`, verify the required GitHub PR environment before dispatch:

   ```bash
   git remote get-url origin
   gh auth status
   ```

   If either command fails, stop before editing and report that CodeDungeon requires GitHub PR workflow support. Full dispatch initializes runs with `phase init --feature <prompt> --branch feat/<slug> --mode FRESH --project-mode SINGLE`.

## Auto Selection

When no mode flag is provided, behave as `--auto`.

Select `full` when the request is complex, multi-repo, architectural, or explicitly needs QA, tests, phase lifecycle, or a final report.

Select `lite` when a plan already exists under `.codedungeon/plans/*.md` and the prompt asks to execute, split, or continue simple planned work.

Select `oneshot` for small direct changes where task splitting would be overhead.

## Dispatch

After selecting the mode, follow the target workflow exactly:

- `full`: run the `main-quest` workflow with the prompt.
- `lite`: run the `side-quest` workflow with the prompt or selected plan.
- `oneshot`: run the `one-shot` workflow with the prompt.
- `rules`: run Project Rules Discovery inline from this router contract.

Code-writing dispatches must end through a GitHub PR. There is no local-only completion path.

Do not remove or rewrite the compatibility aliases. `$codedungeon` is the promoted surface, while `$main-quest`, `$side-quest`, and `$one-shot` stay supported.

## Project Rules Gate

Before dispatching `full`, `lite`, or `oneshot`, run `./.codex/bin/codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `$codedungeon --rules`; do not silently invent project rules.

Every dispatched workflow must preserve this Project Rules envelope in plans, task files, review reports, phase handoffs, and final reports:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```
