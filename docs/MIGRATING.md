# Migrating Existing Projects

Use this when a project already has CodeDungeon installed and you want to move it to a newer binary or prompt pack.

CodeDungeon migration is conservative. It updates CodeDungeon-owned state and bootstrap files, but it does not clean or reset the user's repository.

## Recommended Upgrade Flow

1. Update the CodeDungeon binary or release directory.
2. In each project that already uses CodeDungeon, run the provider-specific migration:

```bash
codedungeon-codex migrate
# or
codedungeon-claude migrate
```

3. Check installed artifact drift:

```bash
codedungeon-codex status
# or
codedungeon-claude status
```

4. If the project intentionally customized installed files, review any `user-modified` or `skipped_paths` output before forcing an install.

## What `migrate` Does

`migrate` opens the project database, runs SQLite migrations, compares the stored `cd_version` with the current binary version, and reinstalls embedded artifacts when the binary changed.

It updates CodeDungeon-owned artifacts such as:

- `.codedungeon/codedungeon.db` schema
- `.codedungeon/commands/*`
- `.codedungeon/phases/*`
- `.codex/agents/*`
- `.codex/config.toml`
- `.agents/skills/*`
- `.claude/agents/*`
- `.claude/skills/*`
- `.claude/commands/*` wrappers
- `AGENTS.md` or `CLAUDE.md` CodeDungeon sections

It also records artifact metadata in the database so later `status`, `install`, and `migrate` commands can detect drift.

## What `migrate` Preserves

Migration does not reset the repository and does not delete arbitrary files.

It preserves:

- project source code
- git history and branches
- user files outside CodeDungeon-owned paths
- `.codedungeon/tasks/*`
- `.codedungeon/plan/*`
- `.codedungeon/state/*`
- `.codedungeon/reviews/*`
- `.codedungeon/memory/*`
- installed artifacts that were modified by the user, unless `install --force` is used

Mutable runtime state is intentionally stored in `.codedungeon/` so interrupted work can be resumed and previous PR work can be inspected.

## Setup vs Install vs Migrate

Use `setup` for first install in a project.

```bash
codedungeon-codex setup
codedungeon-claude setup
```

Use `migrate` after replacing the binary with a newer version.

```bash
codedungeon-codex migrate
codedungeon-claude migrate
```

Use `install --dry-run` to preview what embedded artifacts would be written.

```bash
codedungeon-codex install --dry-run
```

Use `install --force` only after reviewing `status` and accepting that user-modified installed artifacts will be overwritten.

```bash
codedungeon-codex install --force
```

## Legacy Runtime State

Older CodeDungeon versions wrote mutable state under provider-native folders such as `.claude/` or `.codex/`. Current versions use `.codedungeon/` for shared mutable state.

During setup or migration, legacy runtime state is moved or archived so that:

- `.codedungeon/` becomes the shared source of truth
- provider folders keep only provider-native bootstrap files
- user-editable legacy content is preserved rather than silently deleted

## Old Agents and Skills

New versions reinstall current CodeDungeon-owned agents and skills. They do not broadly delete unknown files from `.codex`, `.claude`, or `.agents`.

If an old agent or skill remains and `status` shows it as stale or user-modified:

1. Inspect the file.
2. Keep it if it is a project-specific customization.
3. Remove it manually if it is an obsolete CodeDungeon artifact that is no longer referenced.
4. Run `status` again to confirm the expected state.

This avoids deleting user-owned provider files that happen to live next to CodeDungeon files.

## Safety Rules

Migration must not run destructive git cleanup.

It should not:

- run `git reset`
- run `git clean`
- delete source files
- delete unknown provider files
- erase `.codedungeon` runtime history
- overwrite user-modified artifacts unless explicitly forced

When in doubt, run `status` first and inspect the paths before forcing an install.
