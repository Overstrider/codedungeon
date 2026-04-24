# Cleanup Tasks

Wrapper for `codedungeon cleanup`. Removes stale artifacts under `.claude/`
(tasks, plans, reviews, state). Never touches `commands/`, `agents/`,
`skills/`, `bin/`, `settings*.json`, or `codedungeon.db`.

## Step 1: Inventory (default)

```bash
codedungeon cleanup
```

Returns JSON with file count + bytes per dir. Use this to decide what to delete.

## Step 2: Choose scope

| Flag | Effect |
|---|---|
| `--all` | delete tasks + plans + reviews + state |
| `--tasks` | only `.claude/tasks/*` |
| `--plans` | only `.claude/plan/*` |
| `--reviews` | only `.claude/codereview/*` |
| `--state` | only `.claude/state/*` |
| `--feature NAME` | only `.claude/tasks/NAME/` |
| `--dry-run` | list targets without deleting |

## Step 3: Execute

```bash
# Preview first
codedungeon cleanup --all --dry-run

# Then commit
codedungeon cleanup --all
```

## Safety

Hardcoded never-touch list (in `cmd/cleanup.go`):
- `.claude/commands/`, `.claude/agents/`, `.claude/skills/`, `.claude/bin/`
- `.claude/settings*.json`
- `.claude/codedungeon.db` (DB is source of truth; delete manually if needed)
- `.git/`

Output is JSON `{"ok":bool, "mode":"inventory|dry-run|delete", "deleted":[...], "errors":[...]}`.
