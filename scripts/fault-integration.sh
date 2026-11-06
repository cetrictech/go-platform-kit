#!/usr/bin/env bash
# Restart tests deliberately target only the disposable baseline Compose project.
set -euo pipefail
cd "$(dirname "$0")/.."
command -v docker >/dev/null
# Refuse remote Docker contexts even if a remote daemon happens to have the
# same project name. Fault injection is restricted to a local Unix socket.
docker_endpoint="${DOCKER_HOST:-$(docker context inspect --format '{{.Endpoints.docker.Host}}')}"
case "$docker_endpoint" in
  unix://*) ;;
  *) echo 'Fault tests require a local Docker Unix socket' >&2; exit 1 ;;
esac
# Reject accidental reuse of an application's infrastructure. Tests verify these
# exact endpoints again before issuing any Docker restart command.
export POSTGRES_URL='postgres://postgres:sdk-test-only@127.0.0.1:15432/sdk?sslmode=disable'
export MYSQL_DSN='root:sdk-test-only@tcp(127.0.0.1:13306)/sdk?parseTime=true'
export REDIS_ADDRESS='127.0.0.1:16379'
export KAFKA_BROKER='127.0.0.1:19092'
export SDK_FAULT_TESTS=1
# Services must be running already (make services). A gate must fail on absent
# infrastructure instead of turning missing service coverage into test skips.
for service in postgres mysql redis redpanda; do
  if [[ -z "$(docker compose -f integration/compose.yaml ps --status running -q "$service")" ]]; then
    echo "Missing local fixture: $service; run make services before fault tests" >&2
    exit 1
  fi
done
GOWORK=off go -C integration test -race -tags=integration -run '^(TestKafkaConcurrentConsumerRebalance|TestDependencyProcessRestartRecovery|TestInfrastructureResourceBounds)$' -count=1 -timeout=600s -v ./...

GOWORK=off go -C cache/redis test -race -tags=integration -run '^TestPoolHasHardConnectionBound$' -count=1 -timeout=60s -v ./...

GOWORK=off go -C database/mysql test -race -tags=integration -run '^TestDefaultPoolHasHardConnectionBound$' -count=1 -timeout=60s -v ./...
