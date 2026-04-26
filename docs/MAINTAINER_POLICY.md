# Maintainer Policy

This repository is maintained directly on `main`.

## Non-Negotiable Git Rules

- All work happens on `main`.
- Branches are not used for repository maintenance.
- Git worktrees are prohibited.
- If an agent is not on `main`, it must stop before editing.

## Required Completion Flow

Every completed change must finish with:

1. Documentation updated:
   - `README.md`
   - `AGENTS.md`
   - `CLAUDE.md`
   - relevant docs under `docs/`
   - release docs when user-facing release behavior changes
2. Installers updated when install behavior changes:
   - `install.sh`
   - `install.ps1`
   - `release/install.sh`
   - `release/install.ps1`
3. Verification run:
   - `go test ./...` from `src/codedungeon` for Go or embedded prompt changes
   - focused smoke tests for setup, install, or provider behavior changes
   - `git diff --check`
4. Release artifacts rebuilt with `make release` or the equivalent cross-platform build.
5. Commit created on `main`.
6. `main` pushed to `origin`.

Completion is not valid until build, docs, commit, and push are done.

## Hook Policy

Do not add automatic hooks that silently commit or push. The required flow is explicit and auditable through `AGENTS.md`, `CLAUDE.md`, and this document. Hooks may be added only for local validation if they never mutate history, never switch branches, and never push.

## Final Agent Response

The final response should include:

- commit hash
- pushed remote and branch
- tests and validation run
- release build status
- skipped items and reasons, if any
