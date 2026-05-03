# Phase 0: Validation

- Confirm repo root, git state, provider, and DB path.
- Read project docs and provider pack metadata.
- Ensure each repo has `docs/CODEBASE_MAP.md`.
- If a map is missing or stale, run:

```bash
codedungeon map <repo_path> --format tree > <repo_path>/docs/CODEBASE_MAP.md
```

- Use the generated map as the deterministic codebase overview for later planning phases.
- Identify blockers before planning.
- Close with `PHASE_0_COMPLETE`.
