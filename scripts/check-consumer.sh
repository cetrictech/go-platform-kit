#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
repo=$(pwd)
consumer=$(mktemp -d)
trap 'rm -rf "$consumer"' EXIT
cat > "$consumer/go.mod" <<MOD
module sdk-consumer-check

go 1.26.0

require github.com/cetrictech/go-platform-kit v0.1.0
replace github.com/cetrictech/go-platform-kit => $repo
MOD
cat > "$consumer/main_test.go" <<'GO'
package consumer
import("context";"io";"testing";"github.com/cetrictech/go-platform-kit/logger")
func TestImport(t *testing.T){l:=logger.New(logger.Config{Output:io.Discard});defer l.Close();l.InfoContext(context.Background(),"import works")}
GO
(cd "$consumer" && GOWORK=off go mod tidy && GOWORK=off go test ./...)
# Check that optional infrastructure modules did not enter this consumer graph.
if (cd "$consumer" && GOWORK=off go list -m all) | rg 'jackc/pgx|go-sql-driver|redis/go-redis|twmb/franz|IBM/sarama|google.golang.org/grpc|spiffe/go-spiffe|go.opentelemetry.io|golang-jwt'; then
  echo 'Optional driver leaked into core consumer' >&2
  exit 1
fi
# Compile normal adapter usage without importing raw driver packages.
for module in database/postgres database/mysql cache/redis messaging/kafka/franz messaging/kafka/sarama grpcx spiffe observability security/jwt; do
  (cd "$consumer" && GOWORK=off go mod edit "-require=github.com/cetrictech/go-platform-kit/$module@v0.1.0" "-replace=github.com/cetrictech/go-platform-kit/$module=$repo/$module")
done
cat > "$consumer/adapters_test.go" <<'GO'
package consumer
import("testing";"github.com/cetrictech/go-platform-kit/database/postgres";"github.com/cetrictech/go-platform-kit/database/mysql";"github.com/cetrictech/go-platform-kit/cache/redis";"github.com/cetrictech/go-platform-kit/messaging/kafka/franz";"github.com/cetrictech/go-platform-kit/messaging/kafka/sarama";"github.com/cetrictech/go-platform-kit/grpcx";"github.com/cetrictech/go-platform-kit/spiffe";"github.com/cetrictech/go-platform-kit/observability";jwt "github.com/cetrictech/go-platform-kit/security/jwt")
func TestAdapterImports(t *testing.T){_ = postgres.Config{};_ = mysql.Config{};_ = redis.Config{};_ = franz.Config{};_ = sarama.Config{};_ = grpcx.ServerConfig{};_ = spiffe.Config{};_ = observability.Config{};_ = jwt.Config{}}
GO
(cd "$consumer" && GOWORK=off go mod tidy && GOWORK=off go test ./...)
