#!/usr/bin/env bash
# Double-clickable rebuild (macOS Finder opens .command in Terminal). Runs
# build_bin.sh from the repo root regardless of the working directory, then
# holds the window open so the result stays visible.
set -euo pipefail

DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$DIR"

echo "=== Rebuilding Rays and Magic (bin/) ==="
if ./build_bin.sh; then
	echo ""
	echo "=== Rebuild OK ==="
else
	status=$?
	echo ""
	echo "=== Rebuild FAILED (exit $status) ==="
fi

echo ""
read -n 1 -s -r -p "Press any key to close..."
echo ""
