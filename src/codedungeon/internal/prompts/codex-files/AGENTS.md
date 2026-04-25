# codedungeon for Codex CLI

Use codedungeon as the deterministic workflow kernel. Preserve the phase flow, DB state, handoff schema, review JSON, and task contracts.

Project artifacts:
- Workflow skills: `.agents/skills/codedungeon-dev-cycle/`, `.agents/skills/minidungeon/`, `.agents/skills/code-review/`
- Command playbooks for reference: `.codex/commands/`
- Phase instructions: `.codex/phases/`
- Codex subagents: `.codex/agents/`
- Codex skills: `.agents/skills/`
- Local binary and DB: `.codex/bin/codedungeon`, `.codex/codedungeon.db`

Default workflow:
- Invoke workflows as skills: `$codedungeon-dev-cycle`, `$minidungeon`, `$code-review`, `$codedungeon-test-loop`, `$cleanup-tasks`.
- Use `codedungeon phase info` before changing phase state.
- Use `codedungeon spawn-prompt <phase>` to compose runtime phase context.
- Close completed phases with `codedungeon phase done`.
- Treat `.codex/commands/` as reference playbooks, not Codex CLI slash commands.
- Keep provider-specific instructions in Codex files; do not copy Claude-only syntax into Codex prompts.
