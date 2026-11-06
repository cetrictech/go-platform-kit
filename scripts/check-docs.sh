#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
for doc in README library architecture modules configuration logging errors http pagination postgres mysql redis kafka kafka-sarama kafka-franz grpc spiffe security jwt observability health retries idempotency testing performance versioning migration-guide implementation-status validation dependencies public-api-contracts support-policy dependency-policy compatibility-matrix deprecations performance-regressions; do
  test -s "docs/$doc.md"
done
python3 - <<'PY'
from pathlib import Path
import re
for page in Path('.').rglob('*.md'):
    for target in re.findall(r'\]\(([^)]+)\)', page.read_text()):
        if target.startswith(('https:', 'http:', '#', 'mailto:')):
            continue
        target = target.split('#', 1)[0]
        if target and not (page.parent / target).exists():
            raise SystemExit(f'Broken documentation link: {page}: {target}')
PY
