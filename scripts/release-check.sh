#!/usr/bin/env bash
# A pre-publication gate. Deliberately never tags, pushes or publishes artifacts.
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ $# -gt 2 || ( $# -gt 0 && "${1:-}" != --module ) || $# == 1 ]]; then
 echo 'usage: release-check.sh [--module MODULE_DIRECTORY]' >&2
 exit 2
fi
metadata=(python3 scripts/sdk_release.py metadata --release)
if [[ $# == 2 ]]; then metadata+=(--module "$2"); fi
"${metadata[@]}"
if [[ -n "$(git status --porcelain --untracked-files=normal)" ]]; then
 echo 'Release gate requires a clean committed candidate, including policy files' >&2
 exit 1
fi
candidate=$(git rev-parse HEAD)
output="$PWD/.cache/release-evidence/$candidate/$(date -u +%Y%m%dT%H%M%SZ)-$$"
mkdir -p "$output"
# tee + pipefail retains evidence while propagating any failing command.
check() { local name="$1"; shift; "$@" 2>&1 | tee "$output/$name.log"; }
check gates make gates
check formatting make check-fmt
check docs ./scripts/check-docs.sh
check api ./scripts/check-api.sh
check surface ./scripts/check-api-surface.sh
check vet make vet
check tests make test
check race make race
check minimum env GOTOOLCHAIN=go1.26.10 ./scripts/modules.sh go test -race -count=1 ./...
check static make staticcheck
check security make vuln
check independent ./scripts/check-independent.sh --online
check performance ./scripts/check-performance.sh --release
# Start and clean up only the SDK's explicitly named disposable Compose projects.
services_started=false
cleanup() { if [[ "$services_started" == true ]]; then make services-down > "$output/cleanup.log" 2>&1; fi; }
trap cleanup EXIT
services_started=true
check services make services
check integration make integration
check faults make faults
cleanup
services_started=false
# Guard against generators, tests or a concurrent editor changing the candidate.
if [[ "$(git rev-parse HEAD)" != "$candidate" || -n "$(git status --porcelain --untracked-files=normal)" ]]; then
 echo 'Candidate changed during release validation' >&2
 exit 1
fi
python3 - "$output" "$candidate" <<'PY_RECEIPT'
from pathlib import Path
import datetime,hashlib,json,sys
out=Path(sys.argv[1])
result={'schema_version':1,'candidate':sys.argv[2],'result':'passed','tested_at':datetime.datetime.now(datetime.timezone.utc).isoformat(),'logs':{p.name:hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(out.glob('*.log'))}}
(out/'result.json').write_text(json.dumps(result,indent=2)+'\n')
PY_RECEIPT
echo "All release gates passed for $candidate; evidence: $output/result.json"
echo 'No tag, GitHub release or publication was performed.'
