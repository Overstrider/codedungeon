# codedungeon Architecture

codedungeon is a deterministic workflow kernel beneath AI coding agents. It owns structured state and repeatable mechanics; the agent owns judgment, planning, implementation, and review decisions.

## Providers

Providers are selected by provider-specific binaries:

- `codedungeon-codex` embeds `DefaultProvider=codex`.
- `codedungeon-claude` embeds `DefaultProvider=claude`.

`CODEDUNGEON_PROVIDER` remains available for development and tests. `claude-code` and `claude-ce` are legacy aliases that normalize to canonical provider `claude`.

Provider differences are isolated behind `internal/provider.Provider`:

- project paths: `.codex/*` or `.claude/*`
- project instruction file: `AGENTS.md` or `CLAUDE.md`
- model defaults
- review marker
- plugin support
- thinking-token support

## Prompt Packs

Embedded prompt packs live in `internal/prompts/`:

- Claude pack: `files/`, provider `claude`, pack `codedungeon-claude`.
- Codex pack: `codex-files/`, provider `codex`, pack `codedungeon-codex`.

The shared phase lifecycle stays provider-agnostic. Provider-native mechanics live in the pack content:

- Codex installs `.codex/agents/*.toml`, `.codex/commands/*.md`, `.codex/phases/*.md`, `.codex/config.toml`, `AGENTS.md`, and `.agents/skills/*`.
- Claude installs `.claude/agents/*`, `.claude/commands/*`, `.claude/skills/*`, `.claude/phases/*`, and `CLAUDE.md`.

Commands, agents, skills, and phase prompts are tracked as installed artifacts with provider, pack id, pack version, install path, kind, logical name, sha256, and user-modified state.

## Data Model

Schema v5 uses SQLite with FTS5. Important tables:

- `meta`: schema version, OS, project root, binary version, selected models.
- `runs`, `phases`, `handoffs`: pipeline lifecycle state.
- `prompts`: embedded/user prompt content indexed for search.
- `tasks`, `findings`: task and review history.
- `installed_artifacts`: provider-native artifact drift tracking.

Migration v5 canonicalizes Claude metadata from `claude-code`/`claude-ce` to `claude` and from `codedungeon-claude-code` to `codedungeon-claude`.

## Setup Flow

`setup` is the human-friendly entry point:

1. Verify the target is a git project and not inside provider home config.
2. Install global plugin only when the provider has a plugin system.
3. Pick or accept model tiers.
4. Run bootstrap.
5. Seed prompts and install provider-native artifacts.
6. Write the project instruction section.

Codex has no global plugin step. Claude installs the plugin under `~/.claude/plugins/local/codedungeon`.

## Release Shape

Release builds produce provider-specific binaries. The provider is baked into the binary with ldflags, so end users choose by binary name rather than environment variables.

Installers:

- root `install.sh` and `install.ps1` wrap release installers.
- `release/install.sh` and `release/install.ps1` install local binaries.
- Claude installation also installs the global Claude plugin and copies the plugin binary as `bin/codedungeon`.
- Codex installation installs only the provider binary; project setup installs `.codex/*` and `.agents/skills/*`.

## Adding Work

- Add shared behavior to command/database layers only when it is provider-agnostic.
- Add provider-specific behavior through the provider interface or prompt pack metadata.
- Do not put Claude-specific syntax in Codex prompt files.
- Do not put Codex-specific TOML/skill assumptions in Claude prompt files.
