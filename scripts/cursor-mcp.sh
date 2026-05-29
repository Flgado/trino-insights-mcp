#!/bin/sh
# Launcher for Cursor MCP: always run from repo root (go.mod) so `go run` works
# regardless of the process cwd Cursor uses.
set -eu
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
exec go run ./cmd stdio
