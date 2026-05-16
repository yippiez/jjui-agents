#!/usr/bin/env bash
set -euo pipefail

# Installs jjui from this checkout as `jjui-agents`.
#
# Usage:
#   scripts/install.sh                  # installs to $PREFIX/bin (default: ~/.local)
#   PREFIX=/usr/local scripts/install.sh
#   BIN_NAME=jjui-agents scripts/install.sh

BIN_NAME="${BIN_NAME:-jjui-agents}"
PREFIX="${PREFIX:-$HOME/.local}"
INSTALL_DIR="$PREFIX/bin"

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  echo "error: go is required but was not found in PATH" >&2
  exit 1
fi

VERSION="$(git -C "$REPO_ROOT" describe --tags --always --dirty 2>/dev/null || echo dev)"

mkdir -p "$INSTALL_DIR"

echo "Building $BIN_NAME ($VERSION)..."
go build -ldflags "-X main.Version=$VERSION" -o "$INSTALL_DIR/$BIN_NAME" ./cmd/jjui

echo "Installed $INSTALL_DIR/$BIN_NAME"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *) echo "note: $INSTALL_DIR is not on your PATH" ;;
esac
