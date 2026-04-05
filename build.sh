#!/usr/bin/env bash
set -euo pipefail

BUILDNUM_FILE="$(dirname "$0")/.buildnum"
BUILD=$(cat "$BUILDNUM_FILE" 2>/dev/null || echo 1)

echo "Building Ruuvi Listener (build $BUILD)…"
~/go/bin/fyne package --app-build "$BUILD"

# Increment the local build counter.
echo $((BUILD + 1)) > "$BUILDNUM_FILE"

echo "Done. Next build will be $((BUILD + 1))."
