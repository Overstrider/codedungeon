---
name: codedungeon-cli
description: Use codedungeon CLI commands safely inside Codex CLI workflows.
---

# codedungeon CLI

Use when running or composing codedungeon commands.

Rules:
- Resolve the project root before DB-touching commands.
- Prefer `codedungeon phase info` before changing phase state.
- Use provider paths from `codedungeon status` and installed artifact metadata.
- Do not assume Codex command playbooks are slash commands.
