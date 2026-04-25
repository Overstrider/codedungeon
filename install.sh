#!/usr/bin/env bash
# Root installer wrapper. The release installer remains the canonical implementation.
#
# Usage:
#   ./install.sh --provider codex
#   ./install.sh --provider claude

set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
exec "$HERE/release/install.sh" "$@"
