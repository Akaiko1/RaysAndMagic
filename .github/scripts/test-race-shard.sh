#!/usr/bin/env bash
set -euo pipefail

# Every discovered top-level test, example and fuzz seed belongs to exactly one
# shard. Keep subtests with their parent and preserve Go's execution order.
if [[ $# != 2 || ! $1 =~ ^(0|[1-9][0-9]*)$ || ! $2 =~ ^[1-9][0-9]*$ ]]; then
  echo "Usage: $0 SHARD_INDEX SHARD_COUNT (zero-based index)" >&2
  exit 2
fi
shard=$1
shards=$2
if (( shard >= shards )); then
  echo "Shard index must be smaller than shard count" >&2
  exit 2
fi

game_package=./internal/game
listing=$(go test -race -list '^(Test|Example|Fuzz)' "$game_package")
selected=$(printf '%s\n' "$listing" | awk -v shard="$shard" -v shards="$shards" '
  /^(Test|Example|Fuzz)[^[:space:]]*$/ {
    if (count++ % shards == shard) print
  }
')
if [[ -z $selected ]]; then
  echo "No game tests selected for shard $shard/$shards" >&2
  exit 1
fi
pattern="^($(printf '%s\n' "$selected" | paste -sd '|' -))$"
echo "Running game shard $shard/$shards ($(printf '%s\n' "$selected" | wc -l | tr -d ' ') top-level tests)"
go test -race -count=1 -timeout 15m -run "$pattern" "$game_package"

if (( shard == 0 )); then
  # Resolve import paths instead of hardcoding the module name. Discovery must
  # succeed before starting tests, so a failed go list cannot drop coverage.
  game_import=$(go list "$game_package")
  package_listing=$(go list ./...)
  packages=()
  while IFS= read -r package; do
    if [[ -n $package && $package != "$game_import" ]]; then
      packages+=("$package")
    fi
  done <<< "$package_listing"
  if (( ${#packages[@]} )); then
    go test -race -count=1 -timeout 15m "${packages[@]}"
  fi
fi
