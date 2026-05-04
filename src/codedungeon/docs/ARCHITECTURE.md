# codedungeon Architecture

codedungeon is a deterministic workflow kernel beneath AI coding agents. It owns structured state and repeatable mechanics; the agent owns judgment, planning, implementation, and review decisions.

## Providers

Providers are selected by provider-specific binaries:

- `codedungeon-codex` embeds `DefaultProvider=codex`.
- `codedungeon-claude` embeds `DefaultProvider=claude`.

`CODEDUNGEON_PROVIDER` remains available for development and tests. `claude-code` and `claude-ce` are legacy aliases that normalize to canonical provider `claude`.

Provider differences are isolated behind `internal/provider.Provider`:

- provider-native paths: `.codex/*`, `.claude/*`, `.agents/skills/*`
- shared mutable runtime path: `.codedungeon/*`
- project instruction file: `AGENTS.md` or `CLAUDE.md`
- model defaults
- review marker
- thinking-token support

## Prompt Packs

Embedded prompt packs live in `internal/prompts/`:

- Claude pack: `files/`, provider `claude`, pack `codedungeon-claude`.
- Codex pack: `codex-files/`, provider `codex`, pack `codedungeon-codex`.

The shared phase lifecycle stays provider-agnostic. Provider-native mechanics live in the pack content:

- Codex installs `.codex/agents/*.toml`, `.codex/config.toml` with `multi_agent_v2`, `.agents/skills/*`, and editable `.codedungeon/commands/*.md` plus `.codedungeon/phases/*.md`. Runtime Codex launches that need custom agents pass `--enable multi_agent_v2` directly; setup never persists user-global feature flags. The project config leaves `agents.max_threads` and `agents.max_depth` unset so current Codex builds can use their defaults while the feature flag is active. Setup emits `agent_config_instruction` guidance for `AGENTS.md` instead of editing it.
- Claude installs `.claude/agents/*`, `.claude/commands/*` wrappers, `.claude/skills/*`, and editable `.codedungeon/commands/*.md` plus `.codedungeon/phases/*.md`. Setup emits `agent_config_instruction` guidance for `CLAUDE.md` instead of editing it.

Commands, command wrappers, agents, skills, and phase prompts are tracked as installed artifacts with provider, pack id, pack version, install path, kind, logical name, sha256, and user-modified state.

## Data Model

Schema v16 uses SQLite with FTS5 in `.codedungeon/codedungeon.db`. It uses DELETE journaling so the versionable `.codedungeon` directory does not depend on WAL sidecar files. Important tables:

- `meta`: schema version, OS, project root, binary version, selected models.
- `runs`, `phases`, `handoffs`: pipeline lifecycle state.
- `prompts`: embedded/user prompt content indexed for search.
- `tasks`, `findings`: task and review history.
- `installed_artifacts`: provider-native and `.codedungeon` artifact drift tracking.
- `artifacts`: runtime evidence registry for QA, review, planning, execution, report, phase/handoff, and trace outputs.

Migration v5 canonicalizes Claude metadata from `claude-code`/`claude-ce` to `claude` and from `codedungeon-claude-code` to `codedungeon-claude`. Migration v7 switches SQLite away from WAL. Migration v16 adds the runtime artifact registry.

`installed_artifacts` and `artifacts` intentionally solve different problems. `installed_artifacts` tracks provider pack files written by setup/install/migrate. `artifacts` tracks per-run runtime evidence with module, owner type, owner id, phase, role, path, artifact type, media type, size, SHA-256, and metadata JSON. The registry is an index and evidence integrity layer; it does not replace the existing module-specific tables that own domain state.

## Setup Flow

`setup` is the human-friendly entry point:

1. Verify the target is a git project and not inside provider home config.
2. Pick or accept model tiers.
3. Run bootstrap.
4. Migrate legacy runtime state from `.claude`/`.codex` into `.codedungeon`.
5. Seed prompts and install provider-native bootstrap files plus editable commands/phases.
6. Write the project instruction section.

Setup is strictly project-local for every provider. Compatibility flags such as `--skip-global` remain accepted but do not enable global install behavior.

## Release Shape

Release builds produce provider-specific binaries. The provider is baked into the binary with ldflags, so end users choose by binary name rather than environment variables.

Installers:

- root `install.sh` and `install.ps1` wrap release installers.
- `release/install.sh` and `release/install.ps1` use `--target` or the current directory literally, validate that it is inside a git worktree, copy the selected provider binary to `<project>/.codedungeon/bin/codedungeon-<provider>`, then run project-local setup from that binary.
- Claude project setup installs `.claude/*` and `.codedungeon/*`, then emits guidance for `CLAUDE.md`.
- Codex project setup installs `.codex/*`, `.agents/skills/*`, and `.codedungeon/*`, then emits guidance for `AGENTS.md`.

## Adding Work

- Add shared behavior to command/database layers only when it is provider-agnostic.
- Add provider-specific behavior through the provider interface or prompt pack metadata.
- Do not put Claude-specific syntax in Codex prompt files.
- Do not put Codex-specific TOML/skill assumptions in Claude prompt files.
