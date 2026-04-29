---
name: main-quest
description: Run the full codedungeon phase workflow for complex or multi-repo Codex CLI implementation work.
---

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

# main-quest

Use for complex features, multi-repo changes, or work that needs the full phase lifecycle.

This workflow may execute steps only inside an autonomous CodeDungeon child session. If `CODEDUNGEON_SESSION_TOKEN` is not set, stop and run:

```bash
./.codex/bin/codedungeon run --full --prompt "<prompt>"
```

## GitHub PR Prerequisites

CodeDungeon code-writing workflows require GitHub and the GitHub CLI. Before initializing or editing, verify:

```bash
git remote get-url origin
gh auth status
```

If either command fails, stop before editing and report `Status BLOCKED`. There is no local-only completion path; Phase 5 and Phase 7 require a pushed branch, a GitHub PR, and adversarial review evidence.

## Evidence Gates

- Do not write review reports manually. Code review is a standalone module: run `./.codex/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post`. Legacy `review run` cannot approve empty findings and is not final approval evidence.
- Do not write final reports manually. READY_FOR_USER_REVIEW can only come from `codedungeon run finalize`, which closes eligible final phases, cleans stale telemetry, and renders the report after phase, review, git, and QA gates pass.
- Start final verification with `./.codex/bin/codedungeon qa run --phase 6 --fresh --cmd "<first cmd>"`; execute subsequent concrete build/check/test commands with `./.codex/bin/codedungeon qa run --phase 6 --cmd "<cmd>"`.
- Review is mandatory for code-writing workflows; do not treat `Review: APPROVED` as a substitute for `Verification: PASS`.

## Agent Telemetry

- Before every phase agent, worker, reviewer, validator, classifier, or other subagent spawn, run `./.codex/bin/codedungeon trace agent-start --phase "<phase>" --role "<role>" --agent-type "<agent_type>" --agent-name "<name>" --model "<model>" --reasoning-effort "<effort>" --task "<artifact-or-task>" --input-summary "<summary>"`.
- Save the returned `agent_run_id`; after the subagent returns, run `./.codex/bin/codedungeon trace agent-end --id "<agent_run_id>" --status COMPLETED|FAILED|ABORTED --summary "<result>" --artifact "<primary artifact>" --error "<failure if any>"`.
- Telemetry is informational and must not replace phase, QA, review, PR, or report evidence gates.

Steps:
- Use the existing run created by `codedungeon run`; do not call `phase init` or create a second run.
- Execute pre-final phases in order: `0`, `1`, `2'`, `3.5`, `4`, `5`, `5.5`, `5.6`, `6`. Do not execute Phase `7` manually; `codedungeon run finalize` closes Phase 7 after gates pass.
- For each phase, inspect state with `./.codex/bin/codedungeon phase info <phase>`.
- Use `./.codex/bin/codedungeon spawn-prompt <phase>` to compose phase context.
- If Codex rejects a custom `agent_type`, run `codex features enable multi_agent_v2` or restart Codex with `--enable multi_agent_v2`.
- When spawning a Codex subagent, pass the emitted `agent_type`; record `model` and `reasoning_effort` in telemetry/prompt context, but do not force model overrides when Codex rejects that combination.
- Use Codex subagents from `.codex/agents` only when delegation is explicitly useful; do not rely on agent TOML files to choose model or effort.
- Close phases through `6` with `./.codex/bin/codedungeon phase done`.
- Review posting is handled by `codedungeon code-review --post`; arbitrary marker comments do not satisfy `git verify`.
- Do not merge the PR. The runner final status is `READY_FOR_USER_REVIEW`; the user performs final review and merge.

Do not treat `.codedungeon/commands` as an executable command system. Those files are editable reference playbooks.
