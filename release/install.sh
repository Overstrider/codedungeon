#!/usr/bin/env bash
# codedungeon installer — detect OS, install binary, register Claude Code plugin.
#
# Usage:
#   ./install.sh                # interactive
#   ./install.sh --yes          # non-interactive
#
# Steps:
#   1. Detect OS/arch → pick matching binary from ./bin/.
#   2. Copy binary to ~/.local/bin/codedungeon.
#   3. Create plugin dir ~/.claude/plugins/local/codedungeon/{bin,skills}.
#   4. Install binary + skill into plugin (Claude Code discovers natively).
#   5. Write plugin manifest.

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# --- 1. Pick binary ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS:$ARCH" in
    linux:x86_64)   BIN="codedungeon" ;;
    linux:aarch64)  echo "[WARN] linux-arm64 not built — using amd64 via emulation"; BIN="codedungeon" ;;
    darwin:arm64)   BIN="codedungeon-darwin-arm64" ;;
    darwin:x86_64)  BIN="codedungeon-darwin-amd64" ;;
    *mingw*|*cygwin*|*msys*) BIN="codedungeon.exe" ;;
    *) echo "[ERROR] unsupported OS/arch: $OS/$ARCH"; exit 1 ;;
esac
SRC="$HERE/bin/$BIN"
[[ -x "$SRC" ]] || { echo "[ERROR] binary missing: $SRC"; exit 1; }
echo "[OK] detected $OS/$ARCH → $BIN"

# --- 2. Install binary into user PATH ---
DEST_DIR="$HOME/.local/bin"
mkdir -p "$DEST_DIR"
DEST_BIN="$DEST_DIR/codedungeon"
[[ "$OS" == *mingw* || "$OS" == *msys* ]] && DEST_BIN="$DEST_DIR/codedungeon.exe"
cp "$SRC" "$DEST_BIN"
chmod +x "$DEST_BIN"
echo "[OK] copied binary → $DEST_BIN"

# --- 3. Plugin dir ---
PLUG="$HOME/.claude/plugins/local/codedungeon"
mkdir -p "$PLUG/bin" "$PLUG/skills"

cp "$SRC" "$PLUG/bin/$(basename "$DEST_BIN")"
chmod +x "$PLUG/bin/$(basename "$DEST_BIN")"
echo "[OK] plugin binary → $PLUG/bin/$(basename "$DEST_BIN")"

# --- 4. Skill ---
rm -rf "$PLUG/skills/grimoire-cli"
cp -r "$HERE/skills/grimoire-cli" "$PLUG/skills/"
echo "[OK] skill → $PLUG/skills/grimoire-cli/"

# --- 5. Plugin manifest ---
mkdir -p "$PLUG/.claude-plugin"
cat > "$PLUG/.claude-plugin/plugin.json" <<'EOF'
{
  "name": "codedungeon",
  "version": "0.8.0",
  "description": "Deterministic Go CLI for project pipelines: phase state, review, repo discovery, QA, code-review, task decomposition. SQLite FTS5 backend, embedded prompts.",
  "author": { "name": "loldinis" }
}
EOF
echo "[OK] manifest → $PLUG/.claude-plugin/plugin.json"

# --- 6. PATH advisory ---
if ! command -v codedungeon >/dev/null 2>&1; then
    echo ""
    echo "[WARN] $DEST_DIR is not on PATH. Either:"
    echo "       1) add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to ~/.bashrc"
    echo "       2) or call by full path: $DEST_BIN"
else
    echo "[OK] 'codedungeon' resolves — $(command -v codedungeon)"
fi

# --- 7. Hint ---
echo ""
echo "=== Install complete ==="
echo ""
echo "Next steps:"
echo "  1. cd into any git project"
echo "  2. codedungeon setup"
echo "  3. Claude Code slash commands become available:"
echo "     /minidungeon, /codedungeon-dev-cycle, /code-review"
echo ""
"$DEST_BIN" version --human
