#!/usr/bin/env bash
# get-codedungeon.sh - Download a provider-specific codedungeon binary + quickstart guide into CWD.
#
# Usage:
#   ./get-codedungeon.sh https://github.com/USER/REPO codex
#   ./get-codedungeon.sh https://github.com/USER/REPO claude
#
# Pipe from GitHub:
#   curl -fsSL https://raw.githubusercontent.com/USER/REPO/main/release/get-codedungeon.sh | bash -s -- https://github.com/USER/REPO codex

set -euo pipefail

normalize_provider() {
    case "$1" in
        codex) echo "codex" ;;
        claude|claude-code|claude-ce) echo "claude" ;;
        *) echo "[ERROR] provider must be codex or claude" >&2; exit 1 ;;
    esac
}

REPO_URL="${1:-}"
PROVIDER="${2:-}"
if [ -z "$REPO_URL" ] || [ -z "$PROVIDER" ]; then
    echo "[ERROR] Usage: $0 <github-repo-url> <codex|claude>"
    echo "        Example: $0 https://github.com/loldinis/codedungeon codex"
    exit 1
fi
PROVIDER="$(normalize_provider "$PROVIDER")"

REPO_URL="${REPO_URL%/}"
REPO_URL="${REPO_URL%.git}"
OWNER_REPO=$(echo "$REPO_URL" | sed 's|https://github.com/||')
RAW_BASE="https://raw.githubusercontent.com/${OWNER_REPO}/main/release"

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS:$ARCH" in
    linux:x86_64)             BIN="codedungeon-${PROVIDER}" ;;
    linux:aarch64)            BIN="codedungeon-${PROVIDER}" ;;
    darwin:arm64)             BIN="codedungeon-${PROVIDER}-darwin-arm64" ;;
    darwin:x86_64)            BIN="codedungeon-${PROVIDER}-darwin-amd64" ;;
    *mingw*|*cygwin*|*msys*)  BIN="codedungeon-${PROVIDER}.exe" ;;
    *) echo "[ERROR] Unsupported OS/arch: $OS/$ARCH"; exit 1 ;;
esac

OUT="codedungeon-${PROVIDER}"
[[ "$BIN" == *.exe ]] && OUT="${OUT}.exe"

echo "[1/3] Downloading codedungeon ${PROVIDER} ($OS/$ARCH)..."
if ! curl -fsSL "${RAW_BASE}/bin/${BIN}" -o "$OUT"; then
    echo "[ERROR] Failed to download binary from ${RAW_BASE}/bin/${BIN}"
    exit 1
fi
chmod +x "$OUT"
echo "  -> ./${OUT}"

echo "[2/3] Downloading quickstart guide..."
if ! curl -fsSL "${RAW_BASE}/QUICKSTART.md" -o QUICKSTART.md; then
    echo "[WARN] QUICKSTART.md not found - continuing without guide"
fi
echo "  -> ./QUICKSTART.md"

echo "[3/3] Verifying binary..."
if "./${OUT}" version --human 2>/dev/null; then
    echo ""
else
    echo "[WARN] Binary runs but version check returned non-zero"
fi

echo "=== Download complete ==="
echo ""
echo "Next: ./${OUT} setup"
echo ""
echo "Or read QUICKSTART.md for the full guide."
