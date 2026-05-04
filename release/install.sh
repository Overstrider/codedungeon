#!/usr/bin/env bash
# codedungeon installer - stage a provider-specific binary inside a git project.
#
# Usage:
#   ./install.sh --provider codex --target /path/to/project
#   ./install.sh --provider claude
#   ./install.sh codex
#   ./install.sh claude

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROVIDER=""
TARGET=""

normalize_provider() {
    case "$1" in
        codex) echo "codex" ;;
        claude|claude-code|claude-ce) echo "claude" ;;
        *) echo "[ERROR] provider must be codex or claude" >&2; exit 1 ;;
    esac
}

detect_target() {
    local requested="${1:-}"
    local target
    if [[ -n "$requested" ]]; then
        target="$(cd "$requested" && pwd -P)" || {
            echo "[ERROR] target does not exist: $requested" >&2
            exit 1
        }
        git -C "$target" rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
            echo "[ERROR] target is not inside a git project: $requested" >&2
            exit 1
        }
        echo "$target"
        return
    fi
    target="$(pwd -P)"
    git -C "$target" rev-parse --is-inside-work-tree >/dev/null 2>&1 || {
        echo "[ERROR] run from a git project or pass --target <project-root>" >&2
        exit 1
    }
    echo "$target"
}

while [[ $# -gt 0 ]]; do
    case "$1" in
        --provider) PROVIDER="${2:-}"; shift 2 ;;
        --target) TARGET="${2:-}"; shift 2 ;;
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
TARGET="$(detect_target "$TARGET")"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS:$ARCH" in
    linux:x86_64)   BIN="codedungeon-${PROVIDER}"; EXT="" ;;
    linux:aarch64)  echo "[WARN] linux-arm64 not built - using amd64 via emulation"; BIN="codedungeon-${PROVIDER}"; EXT="" ;;
    darwin:arm64)   BIN="codedungeon-${PROVIDER}-darwin-arm64"; EXT="" ;;
    darwin:x86_64)  BIN="codedungeon-${PROVIDER}-darwin-amd64"; EXT="" ;;
    *mingw*|*cygwin*|*msys*) BIN="codedungeon-${PROVIDER}.exe"; EXT=".exe" ;;
    *) echo "[ERROR] unsupported OS/arch: $OS/$ARCH"; exit 1 ;;
esac

SRC="$HERE/bin/$BIN"
[[ -x "$SRC" ]] || { echo "[ERROR] binary missing: $SRC"; exit 1; }
echo "[OK] detected $OS/$ARCH -> $BIN"

DEST_DIR="$TARGET/.codedungeon/bin"
mkdir -p "$DEST_DIR"
DEST_BIN="$DEST_DIR/codedungeon-${PROVIDER}${EXT}"
cp "$SRC" "$DEST_BIN"
chmod +x "$DEST_BIN"
echo "[OK] copied binary -> $DEST_BIN"

echo "[OK] running project-local setup in $TARGET"
CODEDUNGEON_PROVIDER="$PROVIDER" "$DEST_BIN" setup --target "$TARGET" --yes

echo ""
echo "=== Install complete ==="
echo ""
echo "Project binary:"
echo "  $DEST_BIN"
if [[ "$PROVIDER" == "codex" ]]; then
    echo "Codex workflow router:"
    echo "  $ ./ .agents/skills/codedungeon/SKILL.md is installed for project use"
else
    echo "Claude Code workflow router:"
    echo "  /codedungeon --full|--lite|--oneshot|--auto|--rules <prompt>"
fi
