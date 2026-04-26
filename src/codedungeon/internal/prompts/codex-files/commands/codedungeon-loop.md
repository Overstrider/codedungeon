# codedungeon-loop

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Use for iterative implementation.

Loop:
- Inspect current phase and tasks.
- Select the next unblocked task.
- Implement with focused tests.
- Run the Verification Gate before marking work complete.
- Record progress in codedungeon state.
- Stop when phase done criteria are met.

## Verification Gate

`APPROVED does not replace verification`. Reviews are judgment gates; they do not prove the code compiles, tests pass, or container images build.

Before marking a task complete, before commit/push, and before returning `Status COMPLETE`:

1. Run `./.codex/bin/codedungeon qa detect-framework --path "$REPO_DIR"`.
2. Run the detected build/check/test command.
3. For Rust, run `cargo check` and `cargo test`; `cargo check` is mandatory.
4. If `Dockerfile` or `Containerfile` changed, run `podman build -t codedungeon-verify "$REPO_DIR"`.
5. If no verification command is identifiable, or if required `podman build` cannot run, return `Status BLOCKED`.

For `Status COMPLETE`, the CodeDungeon PR Report must include `Verification: PASS - <commands and result summary>`. If verification is missing, skipped, failed, or blocked, return `Status BLOCKED` with the blocker. `APPROVED does not replace verification`.
