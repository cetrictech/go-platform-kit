#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export GOWORK=off
export GOCACHE="${GOCACHE:-$PWD/.cache/go-build}"
exec go run ./scripts/api-surface/main.go --root "$PWD" "$@"
