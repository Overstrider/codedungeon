# Providers

This document explains the current provider model and how to add another AI coding-agent provider.

## Existing Providers

| Provider | Binary | Project config | Instruction file | Pack |
|---|---|---|---|---|
| `codex` | `codedungeon-codex` | `.codex/` + `.agents/skills/` + `.codedungeon/` | `AGENTS.md` | `codedungeon-codex` |
| `claude` | `codedungeon-claude` | `.claude/` + `.codedungeon/` | `CLAUDE.md` | `codedungeon-claude` |

`claude` is canonical. `claude-code` and `claude-ce` are accepted aliases for compatibility and normalize to `claude`.

## Provider Interface

Providers implement `internal/provider.Provider`:

```go
type Provider interface {
    Name() string
    ConfigDir() string
    AgentConfigFile() string

    BinDir() string
    DBPath() string
    CommandsDir() string
    AgentsDir() string
    SkillsDir() string
    PhasesDir() string
    TasksDir() string
    PlanDir() string
    StateDir() string
    PlansDir() string
    ReviewsDir() string

    HomeGuardPaths() []string
    DefaultModels() ModelConfig
    ModelAlternatives() []ModelConfig
    ReviewCommentMarker() string
    SupportsThinking() bool
}
```

Keep provider implementations small and declarative. They should describe project-local paths, names, models, and runtime capabilities.

## Prompt Packs

Each provider should have a native prompt pack when its command, skill, or agent format differs.

Current packs:

- Claude: `internal/prompts/files/`
- Codex: `internal/prompts/codex-files/`

The pack controls install paths through `prompts.ArtifactsFor(providerName)`. Codex skills intentionally install under `.agents/skills`, while Codex agents install under `.codex/agents`. Codex config enables `multi_agent_v2` project-locally through `.codex/config.toml`; runtime Codex launches that need the agent system pass `--enable multi_agent_v2` on the command line instead of mutating user-global feature flags. Do not set `agents.max_threads` or `agents.max_depth` in the project config while this flag is enabled; current Codex builds reject that combination and the Codex defaults are already 6 and 1. Editable command playbooks and phase prompts install under `.codedungeon/commands` and `.codedungeon/phases` for all providers. Claude also gets thin `.claude/commands` wrappers so slash-command discovery keeps working without storing editable playbook content in the provider directory.

## Adding a Provider

1. Create `internal/provider/<name>.go`.
2. Register aliases in `provider.byName()`.
3. Add a provider prompt pack under `internal/prompts/<name>-files/` if the existing packs are not native for the target.
4. Extend `prompts.packFor()` with provider, pack id, version, root, and install path rules.
5. Add build targets or installer handling if the provider gets a dedicated binary.
6. Add tests for provider detection, artifact install paths, prompt listing, and build targets.
7. Update `README.md`, `docs/ARCHITECTURE.md`, release docs, and installers.

## Naming Rules

- Use a short canonical provider name: `codex`, `claude`, `opencode`, `aider`.
- Keep old product-specific names as aliases only.
- Persist canonical names in `installed_artifacts.provider`.
- Use pack ids in the form `codedungeon-<provider>`.

## Smoke Tests

```bash
go test ./...
CODEDUNGEON_PROVIDER=codex go test ./...
CODEDUNGEON_PROVIDER=claude go test ./...
```

Build smoke:

```bash
go build -ldflags "-X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=codex" .
go build -ldflags "-X github.com/loldinis/codedungeon/internal/provider.DefaultProvider=claude" .
```

Project smoke:

```bash
mkdir /tmp/cd-smoke && cd /tmp/cd-smoke
git init
codedungeon-codex setup --yes
test -f .codedungeon/codedungeon.db
test -f AGENTS.md
test -d .agents/skills
```
