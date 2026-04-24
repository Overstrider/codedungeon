# codedungeon — Quickstart

CLI pipeline for autonomous dev workflows. One binary, zero dependencies.

## Setup (one command)

```bash
./codedungeon setup
```

This does everything:
1. Detects OS, project stack (Go/Rust/Next.js/Kotlin/Python/Elixir/C++)
2. Installs global Claude Code plugin (`~/.claude/plugins/local/codedungeon/`)
3. Asks which model tier to use (interactive) or uses defaults (`--yes`)
4. Copies binary to `.claude/bin/codedungeon`
5. Creates SQLite database `.claude/codedungeon.db`
6. Installs 58 artifacts: agents, skills, commands, phases into `.claude/`
7. Writes codedungeon section to `CLAUDE.md`

Non-interactive: `./codedungeon setup --yes`

### Requirements

- Git repository (run `git init` first if needed)
- `gh` CLI authenticated (for PR creation)

## Available Commands

After setup, three slash commands are available:

### `/minidungeon` — Simple tasks

For single-repo, straightforward features. You plan, it executes.

**Flow:**
1. Use Claude Code's plan mode to write your plan
2. Run `/minidungeon`
3. It reads your plan, splits into tasks, executes each with specialist review, runs adversarial code review, creates PR

**No architect, no QA, no test phase** — just plan → split → execute → review → PR.

### `/codedungeon-dev-cycle` — Full pipeline

For complex, multi-repo features. 10-phase pipeline:

```
Phase 0: Validation (entrance-hall)
Phase 1: Architecture (war-room)
Phase 2': Domain Planning (guild-quarter)
Phase 3.5: QA Planning (trap-workshop)
Phase 4: Task Decomposition (armory)
Phase 5: Execution (forge)
Phase 5.5: QA Refinement (crucible)
Phase 5.6: Test Decomposition (laboratory)
Phase 6: Test Execution (arena)
Phase 7: Report (throne-room)
```

Usage: `/codedungeon-dev-cycle "add user authentication"`

### `/code-review` — Adversarial review

Standalone adversarial code review on current branch:
- 4 personas (Saboteur, New Hire, Security Auditor, Spec Enforcer) on Opus 4.7
- Per-finding validators on Sonnet 4.6
- Design-decision classifier
- Posts results to GitHub PR

Usage: `/code-review`

## How it works for agents

When an AI agent needs to use codedungeon in a new project:

```bash
# 1. Download (if not already present)
#    Agent received the get-codedungeon.sh script or binary directly

# 2. Setup
./codedungeon setup --yes

# 3. For simple tasks:
#    Enter plan mode, write the plan, then:
#    /minidungeon

# 4. For complex features:
#    /codedungeon-dev-cycle "feature description"

# 5. For code review only:
#    /code-review
```

## CLI reference

```
codedungeon setup           # one-step interactive setup
codedungeon bootstrap       # M2M setup (JSON output, non-interactive)
codedungeon version         # binary + schema + runtime info
codedungeon phase           # pipeline phase lifecycle
codedungeon repo            # discover project repos + language detection
codedungeon review          # adversarial review pipeline (deterministic steps)
codedungeon plan            # parse PLAN.md + generate fix tasks
codedungeon git             # branch guard, PR check, diff
codedungeon prompts         # embedded prompt management
codedungeon install         # reinstall embedded artifacts
codedungeon cleanup         # remove stale artifacts
```

## File structure after setup

```
your-project/
  .claude/
    bin/codedungeon          # binary
    codedungeon.db           # SQLite state
    commands/                # slash commands (/minidungeon, /code-review, etc.)
    agents/                  # subagent definitions (18 agents)
    skills/                  # language skills (Rust, Next.js, Kotlin, etc.)
    phases/                  # phase instructions (10 dungeon floors)
  CLAUDE.md                  # auto-updated with codedungeon section
```

## Uninstall

```bash
# Per-project
rm -rf .claude/bin/codedungeon .claude/codedungeon.db
codedungeon cleanup --all   # or just delete .claude/ artifacts

# Global plugin
rm -rf ~/.claude/plugins/local/codedungeon
```
