# codedungeon-dev-cycle

Use for complex features or multi-repo work.

Steps:
- Run `codedungeon phase init` if no active run exists.
- Execute phases in order: `0`, `1`, `2'`, `3.5`, `4`, `5`, `5.5`, `5.6`, `6`, `7`.
- For each phase, use `codedungeon spawn-prompt <phase>` and the matching Codex subagent when useful.
- Keep all state changes in codedungeon commands.
- Do not skip review or test phases unless the DB records the skip reason.

Provider behavior:
- Codex commands are playbooks, not assumed slash commands.
- Use Codex agents from `.codex/agents`.
- Use skills from `.agents/skills`.
