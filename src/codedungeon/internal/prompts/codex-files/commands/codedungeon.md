# codedungeon

Official Codex router playbook for CodeDungeon workflows.

Usage:

```text
$codedungeon [--full|--lite|--oneshot|--one-shot|--auto] <prompt>
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

Validation:

1. If more than one mode flag is present, stop with:

   ```text
   multiple mode flags supplied
   Usage: $codedungeon [--full|--lite|--oneshot|--auto] <prompt>
   ```

2. If the prompt is empty after removing the mode flag, stop with:

   ```text
   prompt required
   Examples:
     $codedungeon --full implement OAuth across the API and web app
     $codedungeon --lite execute .codedungeon/plans/payment-fix.md
     $codedungeon --oneshot fix the typo in README
   ```

3. In `--lite` mode, require a prior plan in `.codedungeon/plans/*.md` or an explicit plan path in the prompt. If no plan exists, stop and ask for a plan first.

4. Before following the selected workflow, print:

   ```text
   CODEDUNGEON_MODE_SELECTED: <mode> - <reason>
   ```

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

Editable reference playbooks live in `.codedungeon/commands/`:

- `.codedungeon/commands/main-quest.md`
- `.codedungeon/commands/side-quest.md`
- `.codedungeon/commands/one-shot.md`

Do not remove or rewrite the compatibility aliases. `$codedungeon` is the promoted surface, while `$main-quest`, `$side-quest`, and `$one-shot` stay supported.
