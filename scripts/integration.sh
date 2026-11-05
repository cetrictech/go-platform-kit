#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
export POSTGRES_URL="${POSTGRES_URL:-postgres://postgres:sdk-test-only@127.0.0.1:15432/sdk?sslmode=disable}"
export MYSQL_DSN="${MYSQL_DSN:-root:sdk-test-only@tcp(127.0.0.1:13306)/sdk?parseTime=true}"
export REDIS_ADDRESS="${REDIS_ADDRESS:-127.0.0.1:16379}"
export KAFKA_BROKER="${KAFKA_BROKER:-127.0.0.1:19092}"
export GPK_TLS_DIR="${GPK_TLS_DIR:-$PWD/integration/.runtime/certs}"
export REDIS_CLUSTER_ADDRESS="${REDIS_CLUSTER_ADDRESS:-127.0.0.1:17001}"
export REDIS_SENTINEL_ADDRESS="${REDIS_SENTINEL_ADDRESS:-127.0.0.1:17103}"
for module in database/postgres database/mysql cache/redis messaging/kafka/franz messaging/kafka/sarama integration; do
  (cd "$module" && GOWORK=off go test -v -cover -race -tags=integration -count=1 -timeout=240s ./...)
done
