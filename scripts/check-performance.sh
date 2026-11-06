#!/usr/bin/env bash
# Compare base/candidate on the same runner instead of comparing unrelated CPUs.
set -euo pipefail
cd "$(dirname "$0")/.."
release=false
if [[ "${1:-}" == --release ]]; then release=true; shift; fi
if [[ $# -gt 1 ]]; then echo 'usage: check-performance.sh [--release] [BASE_REF]' >&2; exit 2; fi
base_ref="${1:-$(python3 -c 'import json;print(json.load(open("release/modules.json"))["bootstrap_ref"])')}"
# Resolve to a commit before using it as a command argument; no shell evaluation.
base_commit=$(git rev-parse --verify --end-of-options "$base_ref^{commit}")
if [[ "$base_commit" == "$(git rev-parse HEAD)" ]] && { [[ "$release" == true ]] || [[ -z "$(git status --porcelain)" ]]; }; then
 echo 'Performance baseline must precede a clean release candidate' >&2
 exit 1
fi
git merge-base --is-ancestor "$base_commit" HEAD
repo="$PWD"
output="${SDK_PERFORMANCE_OUTPUT:-$repo/.cache/performance}"
mkdir -p "$output"
output=$(cd "$output" && pwd)
scratch=$(mktemp -d)
trap 'rm -rf "$scratch"' EXIT
# A separate clone keeps .git metadata and source_dirty meaningful without
# modifying the user's checkout, refs, index or worktree registrations.
git clone --quiet --shared --no-checkout "$repo" "$scratch/base"
git -C "$scratch/base" checkout --quiet --detach "$base_commit"
SDK_BENCH_ROOT="$scratch/base" "$repo/scripts/benchmark-run.sh" "$output/base"
# Retain the exact trusted runner and suite even if the previous release predates it.
SDK_BENCH_ROOT="$repo" "$repo/scripts/benchmark-run.sh" "$output/current"
# Candidate files are immutable; only a paired run may create this temporary file.
baseline="$scratch/baseline.json"
python3 scripts/check-benchmarks.py --record --input "$output/base/benchmarks.log" --metadata "$output/base/metadata.json" --candidate "$baseline"
if [[ "$release" == true ]]; then
 python3 scripts/check-benchmarks.py --input "$output/current/benchmarks.log" --metadata "$output/current/metadata.json" --baseline "$baseline" --release
else
 python3 scripts/check-benchmarks.py --input "$output/current/benchmarks.log" --metadata "$output/current/metadata.json" --baseline "$baseline"
fi
cp "$baseline" "$output/paired-baseline.json"
