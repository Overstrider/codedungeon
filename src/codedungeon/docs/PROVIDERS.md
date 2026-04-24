# Provider Abstraction — Adding Support for a New AI Coding Agent

This doc tells an LLM agent **exactly** how to add a new provider (Codex, OpenCode, Cursor, Aider, Continue.dev, etc.) to codedungeon.

## TL;DR — What you need to do

1. Create one new file: `internal/provider/<name>.go` implementing the `Provider` interface (~80 lines, mostly returning string constants).
2. Register it in `internal/provider/provider.go`'s `byName()` switch (1 line).
3. (Optional but recommended) Create a parallel embedded prompt set under `internal/prompts/files/` if your provider's tool names / spawn protocol differ from Claude Code's.
4. Build, test, smoke-test with `CODEDUNGEON_PROVIDER=<name> codedungeon setup --yes`.

That's it. Zero core pipeline code changes.

---

## The `Provider` interface

Defined in `internal/provider/provider.go`. Reproduced here so you don't need to read the source first:

```go
type Provider interface {
    // ---- Identity ----
    Name() string                // canonical name, e.g. "claude-code", "codex", "opencode"

    // ---- Directory conventions ----
    ConfigDir() string           // e.g. ".claude" | ".codex" | ".opencode"
    AgentConfigFile() string     // e.g. "CLAUDE.md" | "AGENTS.md" | "CONFIG.md"

    // ---- Project-level install paths ----
    // All return paths relative to project root (NOT absolute).
    // Implementations should compose from ConfigDir() for consistency.
    BinDir() string              // e.g. ".claude/bin"
    DBPath() string              // e.g. ".claude/codedungeon.db"
    CommandsDir() string         // e.g. ".claude/commands"
    AgentsDir() string           // e.g. ".claude/agents"
    SkillsDir() string           // e.g. ".claude/skills"
    PhasesDir() string           // e.g. ".claude/phases"
    TasksDir() string            // e.g. ".claude/tasks"
    PlanDir() string             // e.g. ".claude/plan"
    StateDir() string            // e.g. ".claude/state"
    PlansDir() string            // e.g. ".claude/plans" — provider's NATIVE plan-mode dir

    // ---- Global plugin install (optional, agent-specific) ----
    PluginDir() string           // absolute path: e.g. "<HOME>/.claude/plugins/local/codedungeon"
    PluginManifest(version string) []byte   // bytes for the plugin manifest file (e.g. plugin.json)
    HasPluginSystem() bool       // false → installGlobalPlugin returns "skipped"

    // ---- Home guard (refuse to run inside provider's home config dir) ----
    HomeGuardPaths() []string    // e.g. ["~/.claude", "/root/.claude"] — all checked

    // ---- Model defaults ----
    DefaultModels() ModelConfig            // baseline tier {Reasoning, Fast}
    ModelAlternatives() []ModelConfig      // 3-4 pairs offered in interactive setup picker

    // ---- Review marker ----
    ReviewCommentMarker() string // string codedungeon greps for in PR comments to verify review posted

    // ---- Capabilities ----
    SupportsThinking() bool      // true if extended-thinking budgets apply (Claude only, currently)
}

type ModelConfig struct {
    Reasoning string `json:"reasoning"` // deep-thinking model ID
    Fast      string `json:"fast"`      // fast/cheap model ID
}
```

## Where each method gets called

Use this table to predict the impact of each method:

| Method | Called from | Effect |
|---|---|---|
| `ConfigDir()` | `cmd/install.go`, `cmd/bootstrap.go`, `cmd/repo.go` (excludedDirs), `cmd/cleanup.go` | Root dir for all per-project artifacts. Discover skips this dir when scanning sub-repos. |
| `AgentConfigFile()` | `cmd/repo.go`, `cmd/bootstrap.go`, `cmd/review.go` (context-paths) | Filename codedungeon upserts the slash-command reference into. Discover scans for `## Test Auth` here. |
| `BinDir()` | `cmd/bootstrap.go`, `cmd/setup.go` (interactive labels) | Where the self-copied binary lives. |
| `DBPath()` | `cmd/common.go` (OpenDB / OpenDBNoGuard), `cmd/setup.go` (already-bootstrapped check) | SQLite path. |
| `CommandsDir/AgentsDir/SkillsDir/PhasesDir` | Currently informational — concrete `installEmbeddedArtifacts` writes by `RelPath` from embed FS, prefixed with `ConfigDir()`. Reserved for future per-category installs. |
| `TasksDir()` | `cmd/cleanup.go`, `cmd/review.go` (context-paths task glob) | Per-feature task decomposition + spec-enforcer hint. |
| `PlanDir()` | `cmd/review.go` (default `--dir`), `cmd/cleanup.go`, `cmd/phase.go` (pipeline-state.md), `cmd/report.go` | Adversarial review findings + pipeline state + per-repo plans. |
| `StateDir()` | `cmd/phase.go` (handoff render), `cmd/cleanup.go` | `phase-N-output.md` cache (DB is source of truth). |
| `PlansDir()` | `cmd/install.go`, `cmd/minidungeon.md` slash command | Provider's NATIVE plan-mode dir (Claude Code = `.claude/plans/`). minidungeon reads from here. |
| `PluginDir()` | `cmd/setup.go::installGlobalPlugin` | Where to install the global agent plugin. |
| `PluginManifest(version)` | `cmd/setup.go::installGlobalPlugin` | Bytes written to plugin manifest file. Skip via `HasPluginSystem()=false` if N/A. |
| `HasPluginSystem()` | `cmd/setup.go::installGlobalPlugin` | `false` → installGlobalPlugin returns "skipped (no plugin system)". |
| `HomeGuardPaths()` | `cmd/common.go::IsHomeConfig` | Refuse to run if CWD is under any of these paths. |
| `DefaultModels()` | `cmd/config.go` (var ModelDefaults init), `cmd/setup.go` (non-interactive fallback) | Model IDs the agent uses for `--reasoning` / `--fast`. |
| `ModelAlternatives()` | `cmd/config.go` (var ModelAlternatives init), `cmd/setup.go::buildModelTiers` | Pairs offered in interactive picker. |
| `ReviewCommentMarker()` | `cmd/git.go::gitVerifyCmd` | Substring grepped in PR comments to verify adversarial review was posted. |
| `SupportsThinking()` | Reserved — currently only Claude has extended-thinking budgets in `cmd/spawn.go::phaseThinking`. Future providers: return `false` and the spawn-prompt builder will emit `0` budgets. |

## Step-by-step: adding "Codex" as an example

### Step 1 — Create `internal/provider/codex.go`

```go
package provider

import (
    "encoding/json"
    "os"
    "path/filepath"
)

type Codex struct{}

func (Codex) Name() string             { return "codex" }
func (Codex) ConfigDir() string        { return ".codex" }
func (Codex) AgentConfigFile() string  { return "AGENTS.md" }

func (Codex) BinDir() string      { return filepath.Join(".codex", "bin") }
func (Codex) DBPath() string      { return filepath.Join(".codex", "codedungeon.db") }
func (Codex) CommandsDir() string { return filepath.Join(".codex", "commands") }
func (Codex) AgentsDir() string   { return filepath.Join(".codex", "agents") }
func (Codex) SkillsDir() string   { return filepath.Join(".codex", "skills") }
func (Codex) PhasesDir() string   { return filepath.Join(".codex", "phases") }
func (Codex) TasksDir() string    { return filepath.Join(".codex", "tasks") }
func (Codex) PlanDir() string     { return filepath.Join(".codex", "plan") }
func (Codex) StateDir() string    { return filepath.Join(".codex", "state") }
func (Codex) PlansDir() string    { return filepath.Join(".codex", "plans") }

// Codex has no plugin system as of writing — return empty + false.
func (Codex) PluginDir() string             { return "" }
func (Codex) HasPluginSystem() bool         { return false }
func (Codex) PluginManifest(_ string) []byte { return nil }

func (Codex) HomeGuardPaths() []string {
    home, _ := os.UserHomeDir()
    paths := []string{"/root/.codex"}
    if home != "" {
        paths = append([]string{filepath.Join(home, ".codex")}, paths...)
    }
    return paths
}

func (Codex) DefaultModels() ModelConfig {
    return ModelConfig{Reasoning: "o3", Fast: "o4-mini"}
}

func (Codex) ModelAlternatives() []ModelConfig {
    return []ModelConfig{
        {Reasoning: "o3", Fast: "o4-mini"},
        {Reasoning: "o3", Fast: "gpt-4.1-mini"},
        {Reasoning: "gpt-4.1", Fast: "gpt-4.1-mini"},
    }
}

func (Codex) ReviewCommentMarker() string { return "Codex Adversarial Code Review" }
func (Codex) SupportsThinking() bool      { return false }

// Optional: if Codex DOES have a plugin system later, drop these in:
//
// func (Codex) PluginDir() string {
//     home, _ := os.UserHomeDir()
//     return filepath.Join(home, ".codex", "extensions", "codedungeon")
// }
// func (Codex) HasPluginSystem() bool { return true }
// func (Codex) PluginManifest(version string) []byte {
//     m := map[string]any{
//         "name": "codedungeon", "version": version,
//         "description": "...",
//         "author": map[string]string{"name": "loldinis"},
//     }
//     b, _ := json.MarshalIndent(m, "", "  ")
//     return append(b, '\n')
// }
```

### Step 2 — Register in `internal/provider/provider.go`

Find the `byName()` function and add a case:

```go
func byName(name string) Provider {
    switch name {
    case "claude", "claude-code":
        return &Claude{}
    case "codex", "openai-codex":         // ← add
        return &Codex{}
    default:
        return &Claude{}
    }
}
```

### Step 3 — Build + test

```bash
cd src/codedungeon
/usr/local/go/bin/go build ./...
/usr/local/go/bin/go test ./...
```

Should pass with zero changes elsewhere. If it doesn't, you forgot a method on the interface.

### Step 4 — Smoke test in a real project

```bash
rm -rf /tmp/test-codex && mkdir /tmp/test-codex && cd /tmp/test-codex && git init
CODEDUNGEON_PROVIDER=codex /path/to/codedungeon setup --yes
ls -la .codex/                      # should exist with bin/, codedungeon.db, commands/, agents/, …
cat AGENTS.md                       # should have "## codedungeon" section
.codex/bin/codedungeon version --human
```

## Embedded prompts: the hard part

The interface refactor decoupled **paths** from the provider. But the **prompt content** under `internal/prompts/files/` is hardcoded for Claude Code:

- Spawn protocol: `subagent_type: "architect-planner"` (Claude's `Task` tool). Codex/Cursor/Aider use different mechanisms.
- Tool names: `Bash`, `Read`, `Edit`, `Write`, `Grep`, `Glob`, `WebFetch` (Claude's tools).
- Model pinning: `claude-opus-4-7`, `claude-sonnet-4-6`, `claude-haiku-4-5`.
- Path references: `.claude/...` strings inside the prompts.

**You have three options**, in order of effort:

### Option A — Reuse Claude prompts as-is (smallest delta)
If your target provider can be coaxed into running Claude Code-flavored prompts (e.g. via a compatibility shim), you skip prompt work entirely. Most providers cannot.

### Option B — Provider-specific prompt set (recommended)
Add a parallel embed tree:

```
internal/prompts/
├── files/                # Claude (existing)
│   ├── agents/*.md
│   ├── skills/*/SKILL.md
│   ├── commands/*.md
│   └── phases/*.md
└── files-codex/          # Codex (new)
    ├── agents/*.md       # rewritten for Codex spawn protocol + tool names
    ├── skills/*/SKILL.md
    ├── commands/*.md
    └── phases/*.md
```

Then modify `internal/prompts/prompts.go` to select the embed FS based on `provider.Detect().Name()`:

```go
//go:embed all:files
var claudeFS embed.FS

//go:embed all:files-codex
var codexFS embed.FS

func currentFS() fs.FS {
    switch provider.Detect().Name() {
    case "codex":
        sub, _ := fs.Sub(codexFS, "files-codex")
        return sub
    default:
        sub, _ := fs.Sub(claudeFS, "files")
        return sub
    }
}
```

Update `Artifacts()`, `Get()`, `GetRaw()`, `List()` to call `currentFS()` instead of the hardcoded var.

⚠️ **Circular import risk**: if `internal/prompts` starts importing `internal/provider`, and `provider` ever needs `prompts`, you'll loop. Keep `provider` zero-deps (it currently is — only `os`, `encoding/json`, `path/filepath`, `sync`).

### Option C — Template-based prompts (most flexible)
Make every embedded prompt a Go `text/template` with provider-aware variables:

```
{{.SpawnProtocol "architect-planner" "..."}}
{{.ToolName "Bash"}}
{{.ModelID "reasoning"}}
{{.ConfigDir}}/agents/...
```

Add corresponding methods on the `Provider` interface (`SpawnProtocol`, `ToolName`, `ModelID`). At install time, render templates per-provider. Highest upfront cost, lowest ongoing cost.

**For a first new-provider rollout, do Option B.** Option C makes sense once you have 3+ providers.

## Things that are STILL Claude-specific even after the abstraction

These don't break adding a new provider but you should know they exist:

1. **`cmd/setup.go::installGlobalPlugin`** uses the literal string `".claude-plugin"` for Claude's plugin manifest dirname. This is gated by `HasPluginSystem()`, so a Codex provider with `HasPluginSystem() = false` skips the whole function. If your provider DOES have a plugin system but uses a different manifest dirname, refactor `installGlobalPlugin` to ask the provider for it (add `PluginManifestDir() string` to the interface).

2. **`cmd/spawn.go::phaseThinking`** has hardcoded extended-thinking budgets (`32000`, `8000`, `2000`). The current builder always emits `max_thinking_tokens: N`. If `provider.SupportsThinking() == false`, the value is meaningless to the agent but harmless. To suppress it, gate the line in `buildSpawnPrompt`.

3. **Embedded prompt content** mentions `claude-opus-4-7` in narrative text (e.g. `/code-review` says "Opus 4.7 persona fanout + Sonnet validators"). Cosmetic — agent doesn't parse this. But fix it in your provider-specific prompt set (Option B above).

4. **`docs/`** uses Claude examples throughout. Update if you contribute a Codex variant.

## Sanity-check: is your new provider correctly wired?

After Step 3, run this command and inspect the output:

```bash
CODEDUNGEON_PROVIDER=codex /path/to/codedungeon repo discover --root /tmp/some-project
# Should produce JSON, no panics, no "claude" strings in paths
```

Then:

```bash
grep -rn '\.claude' src/codedungeon/cmd/ src/codedungeon/internal/ \
  --include='*.go' \
  | grep -v 'provider/claude' \
  | grep -v '_test.go' \
  | grep -v '// ' | grep -v 'Short:' | grep -v 'Long:' | grep -v 'String('
```

Should return zero functional lines (only comments / help text). If your new provider sees `.claude/` paths anywhere at runtime, something escaped the abstraction — fix it before merging.

## What to NOT touch

- `internal/db/` — schema is provider-agnostic.
- `internal/manifest/` — language detection.
- `internal/reviewpipe/` — dedupe/filter/classify/render logic.
- `internal/osadapter/` — OS abstraction is orthogonal.
- `cmd/phase.go` (lifecycle), `cmd/plan.go` (PLAN.md parsing), `cmd/git.go` (git wrappers), `cmd/report.go` (report rendering) — all already provider-aware via `provider.Detect()`. Don't add provider-specific branches here; add methods to the interface instead.

## Glossary

- **Provider** — an AI coding agent that drives codedungeon (Claude Code, Codex, OpenCode, Cursor, Aider, …).
- **Config dir** — provider's per-project artifact root (`.claude`, `.codex`, …).
- **Agent config file** — provider's per-project markdown file the agent auto-reads on session start (`CLAUDE.md`, `AGENTS.md`, …).
- **Plugin system** — provider's mechanism for loading global skills/commands. Claude Code has one (`~/.claude/plugins/local/<name>`). Codex/Cursor at time of writing do not.
- **Review marker** — substring inserted in PR review comment so codedungeon can verify a review was posted (`gh pr view --comments | grep <marker>`).

## Reference: the existing Claude impl

`internal/provider/claude.go` — read it before writing your own. It's ~80 lines and answers any "what should this method return for a working provider?" question.
