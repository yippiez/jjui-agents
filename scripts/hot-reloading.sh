#!/usr/bin/env bash
set -euo pipefail

# Hot-reloads jjui during development using air.
# Watches .go and .toml files, rebuilds and restarts on change.
#
# Usage:
#   scripts/hot-reloading.sh
#   AIR_FLAGS="--build.args_bin='--help'" scripts/hot-reloading.sh

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required but was not found in PATH" >&2
  exit 1
fi

if ! command -v air >/dev/null 2>&1; then
  echo "air not found, installing..."
  go install github.com/air-verse/air@latest
fi

exec air -c /dev/stdin ${AIR_FLAGS:-} <<'AIRCONF'
root = "."
tmp_dir = "tmp"

[build]
  bin = "./tmp/jjui"
  cmd = "go build -o ./tmp/jjui ./cmd/jjui"
  delay = 500
  exclude_dir = ["tmp", "vendor", ".git", "test"]
  exclude_regex = ["_gen\\.go$"]
  include_ext = ["go", "toml"]
  kill_delay = 500
  send_interrupt = true
  stop_on_error = true

[log]
  time = false

[misc]
  clean_on_exit = true
AIRCONF
