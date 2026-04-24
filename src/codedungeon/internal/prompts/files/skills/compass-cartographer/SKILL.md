---
name: compass-cartographer
description: Scan a codebase into a file-tree with estimated token counts. Built into the `codedungeon` binary — no Python, no tiktoken, no uv. Use before phase 1 (architect) or whenever an agent needs a compact codebase overview.
---

# cartographer

```bash
codedungeon map [<path>] [--format json|tree|compact] [--max-tokens N]
```

Walks `<path>` (default CWD), respects `.gitignore` + default ignore list (node_modules/.git/target/etc.), skips binaries + files > 1 MB + files over `--max-tokens` (default 50000).

## Output formats

- `json` (default) — full scan result: `{root, files[], directories[], total_tokens, total_files, skipped[]}`.
- `tree` — human-readable file tree with per-file token counts.
- `compact` — one line per file, sorted by tokens desc: `<tokens> <rel/path>`.

## Token estimation

Heuristic: `len(bytes)/4`. Good enough for sizing decisions (which files to include in a prompt, which are too big). Not a replacement for the real Claude tokenizer.

## Usage in pipeline

Phase 0 runs `codedungeon map <repo>` per repo and writes `docs/CODEBASE_MAP.md`. Subsequent phase agents read the map instead of re-scanning — cheap cross-phase context reuse.

## Example

```bash
codedungeon map ./backend --format compact > /tmp/scan.txt
head -20 /tmp/scan.txt
```
