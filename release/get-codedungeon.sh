#!/usr/bin/env bash
# get-codedungeon.sh — Download codedungeon binary + quickstart guide into CWD.
#
# Usage:
#   # With GitHub repo URL:
#   ./get-codedungeon.sh https://github.com/USER/REPO
#
#   # Pipe from GitHub (replace USER/REPO):
#   curl -fsSL https://raw.githubusercontent.com/USER/REPO/main/release/get-codedungeon.sh | bash -s -- https://github.com/USER/REPO
#
#   # Agent shorthand — pass URL, script does the rest:
#   bash get-codedungeon.sh https://github.com/USER/REPO

set -euo pipefail

REPO_URL="${1:-}"
if [ -z "$REPO_URL" ]; then
    echo "[ERROR] Usage: $0 <github-repo-url>"
    echo "        Example: $0 https://github.com/loldinis/codedungeon"
    exit 1
fi

# Strip trailing slash + .git
REPO_URL="${REPO_URL%/}"
REPO_URL="${REPO_URL%.git}"

# Build raw URL base
OWNER_REPO=$(echo "$REPO_URL" | sed 's|https://github.com/||')
RAW_BASE="https://raw.githubusercontent.com/${OWNER_REPO}/main/release"

# --- Detect OS/arch ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)
case "$OS:$ARCH" in
    linux:x86_64)             BIN="codedungeon" ;;
    linux:aarch64)            BIN="codedungeon" ;; # amd64 via emulation
    darwin:arm64)             BIN="codedungeon-darwin-arm64" ;;
    darwin:x86_64)            BIN="codedungeon-darwin-amd64" ;;
    *mingw*|*cygwin*|*msys*)  BIN="codedungeon.exe" ;;
    *)
        echo "[ERROR] Unsupported OS/arch: $OS/$ARCH"
        exit 1
        ;;
esac

echo "[1/3] Downloading codedungeon ($OS/$ARCH)..."
if ! curl -fsSL "${RAW_BASE}/bin/${BIN}" -o codedungeon; then
    echo "[ERROR] Failed to download binary from ${RAW_BASE}/bin/${BIN}"
    exit 1
fi
chmod +x codedungeon
echo "  -> ./codedungeon"

echo "[2/3] Downloading quickstart guide..."
if ! curl -fsSL "${RAW_BASE}/QUICKSTART.md" -o QUICKSTART.md; then
    echo "[WARN] QUICKSTART.md not found — continuing without guide"
fi
echo "  -> ./QUICKSTART.md"

echo "[3/3] Verifying binary..."
if ./codedungeon version --human 2>/dev/null; then
    echo ""
else
    echo "[WARN] Binary runs but version check returned non-zero"
fi

echo "=== Download complete ==="
echo ""
echo "Next: ./codedungeon setup"
echo ""
echo "Or read QUICKSTART.md for the full guide."
