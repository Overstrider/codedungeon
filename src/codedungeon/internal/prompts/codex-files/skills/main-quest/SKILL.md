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

- Do not write review reports manually. Persona outputs must be real files such as `findings-saboteur.json`, declared in `review-manifest.json`, then aggregated with `./.codex/bin/codedungeon review run`.
- Do not write final reports manually. READY_FOR_USER_REVIEW can only come from `codedungeon report render` after phase, review, git, and QA gates pass.
- Execute every concrete build/check/test command with `./.codex/bin/codedungeon qa run --phase 6 --cmd "<cmd>"`.
- Review is mandatory for code-writing workflows; do not treat `Review: APPROVED` as a substitute for `Verification: PASS`.

Steps:
- Use the existing run created by `codedungeon run`; do not call `phase init` or create a second run.
- Execute phases in order: `0`, `1`, `2'`, `3.5`, `4`, `5`, `5.5`, `5.6`, `6`, `7`.
- For each phase, inspect state with `./.codex/bin/codedungeon phase info <phase>`.
- Use `./.codex/bin/codedungeon spawn-prompt <phase>` to compose phase context.
- If Codex rejects a custom `agent_type`, run `codex features enable multi_agent_v2` or restart Codex with `--enable multi_agent_v2`.
- When spawning a Codex subagent, pass the `agent_type`, `model`, and `reasoning_effort` emitted by `spawn-prompt <phase>`.
- Use Codex subagents from `.codex/agents` only when delegation is explicitly useful; do not rely on agent TOML files to choose model or effort.
- Close each phase with `./.codex/bin/codedungeon phase done`.
- Post review evidence with `./.codex/bin/codedungeon review post`; arbitrary marker comments do not satisfy `git verify`.
- Do not merge the PR. The runner final status is `READY_FOR_USER_REVIEW`; the user performs final review and merge.

Do not treat `.codedungeon/commands` as an executable command system. Those files are editable reference playbooks.
