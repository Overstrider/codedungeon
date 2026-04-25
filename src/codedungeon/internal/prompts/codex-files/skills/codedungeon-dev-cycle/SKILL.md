---
name: codedungeon-dev-cycle
description: Run the full codedungeon phase workflow for complex or multi-repo Codex CLI implementation work.
---

# codedungeon-dev-cycle

Use for complex features, multi-repo changes, or work that needs the full phase lifecycle.

Steps:
- Ensure a codedungeon run exists with `codedungeon phase init` when needed.
- Execute phases in order: `0`, `1`, `2'`, `3.5`, `4`, `5`, `5.5`, `5.6`, `6`, `7`.
- For each phase, inspect state with `codedungeon phase info <phase>`.
- Use `codedungeon spawn-prompt <phase>` to compose phase context.
- Use Codex subagents from `.codex/agents` only when delegation is explicitly useful.
- Close each phase with `codedungeon phase done`.

Do not treat `.codex/commands` as an executable command system. Those files are reference playbooks.
