# main-quest

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Use for complex features or multi-repo work.

## GitHub PR Prerequisites

CodeDungeon code-writing workflows require GitHub and the GitHub CLI. Before initializing or editing, verify:

```bash
git remote get-url origin
gh auth status
```

If either command fails, stop before editing and report `Status BLOCKED`. There is no local-only completion path; Phase 5 and Phase 7 require a pushed branch, a GitHub PR, and adversarial review evidence.

## Evidence Gates

- Do not write review reports manually. Persona outputs must be real files such as `findings-saboteur.json`, declared in `review-manifest.json`, then aggregated with `./.codex/bin/codedungeon review run`.
- Do not write final reports manually. COMPLETE can only come from `codedungeon report render` after phase, review, git, and QA gates pass.
- Record every concrete build/check/test command with `./.codex/bin/codedungeon qa record --phase 6 --cmd "<cmd>" --status PASS|FAIL --log <path>`.
- Review is mandatory for code-writing workflows; do not treat `Review: APPROVED` as a substitute for `Verification: PASS`.

Steps:
- Run `./.codex/bin/codedungeon phase init --feature "$FEATURE_PROMPT" --branch "feat/<slug>" --mode FRESH --project-mode SINGLE` if no active run exists.
- Execute phases in order: `0`, `1`, `2'`, `3.5`, `4`, `5`, `5.5`, `5.6`, `6`, `7`.
- For each phase, use `./.codex/bin/codedungeon spawn-prompt <phase>` and the matching Codex subagent when useful.
- If Codex rejects a custom `agent_type`, run `codex features enable multi_agent_v2` or restart Codex with `--enable multi_agent_v2`.
- Preserve the `agent_type`, `model`, and `reasoning_effort` emitted by `spawn-prompt <phase>` when spawning subagents.
- Keep all state changes in codedungeon commands.
- Do not skip review or test phases unless the DB records the skip reason.

Provider behavior:
- Codex commands are playbooks, not assumed slash commands.
- Use Codex agents from `.codex/agents`.
- Use skills from `.agents/skills`.
- Model and effort selection lives in codedungeon DB config, not in Codex agent TOML.
