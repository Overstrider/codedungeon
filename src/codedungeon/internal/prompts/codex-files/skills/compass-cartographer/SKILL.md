---
name: compass-cartographer
description: Scan a codebase into a file tree with estimated token counts. Built into the `codedungeon` binary. Use before planning or whenever an agent needs a compact codebase overview.
---

# cartographer

```bash
codedungeon map [<path>] [--format json|tree|compact] [--max-tokens N]
```

Walks `<path>` (default CWD), respects `.gitignore` plus default ignores (`node_modules`, `.git`, `target`, build outputs), skips binaries and files over `--max-tokens` (default 50000).

## Output formats

- `json` (default): full scan result `{root, files[], directories[], total_tokens, total_files, skipped[]}`.
- `tree`: human-readable file tree with per-file token counts.
- `compact`: one line per file, sorted by token estimate: `<tokens> <rel/path>`.

## Token estimation

Heuristic: `len(bytes)/4`. It is meant for sizing and prioritizing files, not exact provider tokenizer parity.

## Usage in pipeline

Phase 0 runs `codedungeon map <repo> --format tree` per repo and writes `docs/CODEBASE_MAP.md`. Later phase agents read the map instead of rescanning.

## Example

```bash
codedungeon map ./backend --format compact > /tmp/scan.txt
head -20 /tmp/scan.txt
```
