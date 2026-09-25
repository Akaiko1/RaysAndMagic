#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ $# -eq 0 ]]; then
  echo "Usage: $0 SCENARIO [game arguments]"
  echo "See assets/test_scenarios.yaml for available scenarios."
  exit 2
fi
SCENARIO="$1"
shift
cd "$DIR/.."
exec "$DIR/raysandmagic" --test-scenario "$SCENARIO" "$@"
