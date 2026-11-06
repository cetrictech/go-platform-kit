#!/usr/bin/env bash
set -euo pipefail
# Isolated build tool; never adds experimental dependencies to consumer modules.
export GOBIN="${SDK_TOOL_BIN:-$(pwd)/.cache/sdk-tools}"
mkdir -p "$GOBIN"
GOWORK=off go install golang.org/x/exp/cmd/apidiff@v0.0.0-20261014143641-f0251eb4aa3e
