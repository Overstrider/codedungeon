#!/usr/bin/env bash
# codedungeon installer - detect OS, install provider-specific binary.
#
# Usage:
#   ./install.sh --provider codex
#   ./install.sh --provider claude
#   ./install.sh codex
#   ./install.sh claude

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROVIDER=""

normalize_provider() {
    case "$1" in
        codex) echo "codex" ;;
        claude|claude-code|claude-ce) echo "claude" ;;
        *) echo "[ERROR] provider must be codex or claude" >&2; exit 1 ;;
    esac
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --provider) PROVIDER="${2:-}"; shift 2 ;;
        codex|claude|claude-code|claude-ce) PROVIDER="$1"; shift ;;
        --yes) shift ;;
        *) echo "[ERROR] unknown arg: $1"; exit 1 ;;
    esac
done

if [[ -z "$PROVIDER" ]]; then
    if [[ -t 0 ]]; then
        read -r -p "Provider [codex/claude]: " PROVIDER
    else
        echo "[ERROR] provider required: ./install.sh --provider codex|claude"
        exit 1
    fi
fi
PROVIDER="$(normalize_provider "$PROVIDER")"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS:$ARCH" in
    linux:x86_64)   BIN="codedungeon-${PROVIDER}" ;;
    linux:aarch64)  echo "[WARN] linux-arm64 not built - using amd64 via emulation"; BIN="codedungeon-${PROVIDER}" ;;
    darwin:arm64)   BIN="codedungeon-${PROVIDER}-darwin-arm64" ;;
    darwin:x86_64)  BIN="codedungeon-${PROVIDER}-darwin-amd64" ;;
    *mingw*|*cygwin*|*msys*) BIN="codedungeon-${PROVIDER}.exe" ;;
    *) echo "[ERROR] unsupported OS/arch: $OS/$ARCH"; exit 1 ;;
esac

SRC="$HERE/bin/$BIN"
[[ -x "$SRC" ]] || { echo "[ERROR] binary missing: $SRC"; exit 1; }
echo "[OK] detected $OS/$ARCH -> $BIN"

DEST_DIR="$HOME/.local/bin"
mkdir -p "$DEST_DIR"
DEST_BIN="$DEST_DIR/codedungeon-${PROVIDER}"
[[ "$OS" == *mingw* || "$OS" == *msys* ]] && DEST_BIN="$HOME/.local/bin/codedungeon-${PROVIDER}.exe"
cp "$SRC" "$DEST_BIN"
chmod +x "$DEST_BIN"
echo "[OK] copied binary -> $DEST_BIN"

if [[ "$PROVIDER" == "claude" ]]; then
    PLUG="$HOME/.claude/plugins/local/codedungeon"
    mkdir -p "$PLUG/bin" "$PLUG/skills"

    PLUGIN_BIN="$PLUG/bin/codedungeon"
    [[ "$OS" == *mingw* || "$OS" == *msys* ]] && PLUGIN_BIN="$PLUG/bin/codedungeon.exe"
    cp "$SRC" "$PLUGIN_BIN"
    chmod +x "$PLUGIN_BIN"
    echo "[OK] plugin binary -> $PLUGIN_BIN"

    rm -rf "$PLUG/skills/grimoire-cli"
    cp -r "$HERE/skills/grimoire-cli" "$PLUG/skills/"
    echo "[OK] skill -> $PLUG/skills/grimoire-cli/"

    mkdir -p "$PLUG/.claude-plugin"
    cat > "$PLUG/.claude-plugin/plugin.json" <<'JSON'
{
  "name": "codedungeon",
  "version": "0.8.0",
  "description": "Deterministic Go CLI for project pipelines: phase state, review, repo discovery, QA, code-review, task decomposition. SQLite FTS5 backend, embedded prompts.",
  "author": { "name": "loldinis" }
}
JSON
    echo "[OK] manifest -> $PLUG/.claude-plugin/plugin.json"
else
    echo "[OK] Codex provider selected; no global Claude plugin installed"
fi

BIN_CMD="$(basename "$DEST_BIN")"
if ! command -v "$BIN_CMD" >/dev/null 2>&1; then
    echo ""
    echo "[WARN] $DEST_DIR is not on PATH. Either:"
    echo "       1) add 'export PATH=\"\$HOME/.local/bin:\$PATH\"' to ~/.bashrc"
    echo "       2) or call by full path: $DEST_BIN"
else
    echo "[OK] '$BIN_CMD' resolves -> $(command -v "$BIN_CMD")"
fi

echo ""
echo "=== Install complete ==="
echo ""
echo "Next steps:"
echo "  1. cd into any git project"
echo "  2. $BIN_CMD setup"
if [[ "$PROVIDER" == "codex" ]]; then
    echo "  3. Codex workflow router becomes available:"
    echo "     \$codedungeon --full|--lite|--oneshot <prompt>"
    echo "     Compatibility aliases: \$main-quest, \$side-quest, \$one-shot"
else
    echo "  3. Claude Code workflow router becomes available:"
    echo "     /codedungeon --full|--lite|--oneshot <prompt>"
    echo "     Compatibility aliases: /main-quest, /side-quest, /one-shot"
fi
echo ""
"$DEST_BIN" version --human
