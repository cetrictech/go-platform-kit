#!/usr/bin/env bash
# Repeated, reproducible core-only microbenchmarks. Network benchmarks remain separate.
set -euo pipefail
if [[ $# != 1 ]]; then
 echo "usage: $0 OUTPUT_DIRECTORY" >&2
 exit 2
fi
mkdir -p "$1"
output_dir=$(cd "$1" && pwd)
source_root="${SDK_BENCH_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
if [[ ! -f "$source_root/go.mod" ]]; then
 echo "SDK_BENCH_ROOT must contain the SDK root go.mod" >&2
 exit 2
fi
cd "$source_root"
export GOWORK=off GOMAXPROCS=2
export GOFLAGS=""
go test -run '^$' -bench . -benchmem -benchtime=200ms -count=10 -cpu=2 \
 ./database ./encodingx ./errors ./logger ./messaging/kafka ./pagination ./response \
 > "$output_dir/benchmarks.log"
python3 - "$output_dir" <<'PY_META'
import json, subprocess, sys
from pathlib import Path
output=Path(sys.argv[1])
info=json.loads(subprocess.check_output(['go','env','-json','GOVERSION','GOOS','GOARCH'],text=True))
cpus={line[5:].strip() for line in (output/'benchmarks.log').read_text().splitlines() if line.startswith('cpu: ')}
if len(cpus)!=1:raise SystemExit('Cannot identify one CPU from benchmark output; evidence would be incomparable')
metadata={'schema':1,'go_version':info['GOVERSION'],'goos':info['GOOS'],'goarch':info['GOARCH'],'cpu':cpus.pop(),'gomaxprocs':2,'race':False,'suite':'core-v1','benchtime':'200ms','commit':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'source_dirty':bool(subprocess.check_output(['git','status','--porcelain'],text=True).strip())}
(output/'metadata.json').write_text(json.dumps(metadata,indent=2,sort_keys=True)+'\n')
PY_META

echo "Collected core benchmark evidence in $output_dir"
