#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
modules=(. database/postgres database/mysql cache/redis messaging/kafka/franz messaging/kafka/sarama grpcx spiffe observability security/jwt storage/s3 integration)
for module in "${modules[@]}"; do
  echo "Module: $module"
  (cd "$module" && GOWORK=off "$@")
done
