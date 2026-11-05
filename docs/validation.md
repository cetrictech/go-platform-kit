# Executed validation

Validation date: **2026-11-05 (Africa/Nairobi)**. Host: macOS arm64, Apple M3 Pro. Primary toolchain: Go 1.27.3. Supported-line test toolchain: Go 1.26.10. Staticcheck: 2026.2.2 (v0.8.2). Govulncheck: v1.9.0, using the public Go vulnerability database.

## Outcomes

| Check | Command | Actual result |
| --- | --- | --- |
| Unit tests and race detection | `./scripts/modules.sh go test -race -count=1 ./...` | PASS: 31 tested packages, ten modules visited; internal redaction/test-only integration have no ordinary tests |
| Minimum supported Go line | `GOTOOLCHAIN=go1.26.10 ./scripts/modules.sh go test -count=1 ./...` | PASS: same 31 tested packages |
| Build | `./scripts/modules.sh go build ./...` | PASS: all ten modules |
| Vet | `./scripts/modules.sh go vet ./...` | PASS |
| Staticcheck | `./scripts/modules.sh staticcheck ./...` | PASS |
| Vulnerabilities | `./scripts/modules.sh govulncheck ./...` | PASS at reachable-symbol level; see advisory note below |
| Formatting | `make check-fmt` | PASS |
| Dependency reconciliation | `./scripts/modules.sh go mod tidy` | Completed independently for every module |
| Workspace compatibility | one `go test` invocation including core and every adapter/integration package | PASS: 31 tested packages |
| Independent consumer | `./scripts/check-consumer.sh` | PASS: logging-only graph excludes optional drivers; all adapter imports compile in another module with explicit local replacements |
| Integration | `./scripts/integration.sh` | PASS: six package suites against real PostgreSQL/MySQL/Redis/Redpanda; no skips |
| Coverage | `./scripts/modules.sh go test -cover ./...` | Executed; per-package results below and in logs |
| Local benchmarks | `make bench` equivalent per-module commands | Executed; recorded allocation results |
| Network benchmarks | tagged Redis/Franz/Sarama benchmarks, `-benchtime=100x` | Executed against Docker services; includes network/server latency |

The ordinary inventory contains **54 top-level tests**; integration suites add real-service tests and the four cross-adapter subtests. Exact output is preserved in [the log directory](validation/). These are executed local results, not GitHub Actions results. CI workflows are configured but have not been run on GitHub here.

## Real-service coverage

- PostgreSQL/MySQL: real connection, commit, rollback, bound inserts, duplicate-key failure, struct/primitive scanning, context cancellation, transactional batches, composite forward/reverse keysets with NULLs and duplicates, SQL outbox successful delivery state and failed/dead attempt state.
- Redis: real JSON serialization, ownership-token lock contention/renewal/release, and concurrent atomic idempotency ownership/mismatch behavior.
- Kafka: acknowledged production, consumption, handler-failure propagation, topic/partition/group/offset inspection, and Franz-to-Franz, Franz-to-Sarama, Sarama-to-Franz and Sarama-to-Sarama delivery. Cross tests prove successfully committed records are skipped while the failed uncommitted record is redelivered.
- Local gRPC: real mutual TLS health RPCs and rejection of a missing client certificate. Local Workload API endpoint: actual X.509-SVID acquisition, mutual TLS, peer rejection and certificate rotation. OpenTelemetry: real gRPC trace propagation and matching trace IDs. These local protocol tests are included in ordinary unit/race runs.

Service versions captured from the running containers:

```text
postgres (PostgreSQL) 18.6 (Debian 18.6-1.pgdg13+2)
mysql  Ver 8.4.13 for Linux on aarch64 (MySQL Community Server - GPL)
Redis server v=8.10.3 sha=00000000:1 malloc=jemalloc-5.3.0 bits=64 build=418e78fb81cd4847
v25.3.1 - 9cd8d8c8e32057bc7f41d690a01ae29552b58a7f
```

## Security scan detail

The first independent SPIFFE scan found reachable vulnerabilities inherited from older gRPC and x/net requirements. Those dependencies were upgraded to stable gRPC v1.85.0 and x/net v0.62.0, followed by tests and a fresh scan. The final scan reports zero reachable vulnerabilities across the independently checked modules.

Some module graphs still report **GO-2026-5932** in the unused `golang.org/x/crypto/openpgp` package. The SDK uses bcrypt and does not import OpenPGP. Stable crypto v0.58.0 was retained rather than silently switching to the advisory's pseudo-version fix. See the [official advisory](https://pkg.go.dev/vuln/GO-2026-5932) and [exact scan output](validation/vulnerabilities.log). “Zero reachable” is not a claim that every transitive module is free of advisories.

## Recorded ordinary coverage

```text
ok  	github.com/cetrictech/go-platform-kit/clock	(cached)	coverage: 52.4% of statements
ok  	github.com/cetrictech/go-platform-kit/config	0.366s	coverage: 47.3% of statements
ok  	github.com/cetrictech/go-platform-kit/database	(cached)	coverage: 66.7% of statements
ok  	github.com/cetrictech/go-platform-kit/encodingx	(cached)	coverage: 34.4% of statements
ok  	github.com/cetrictech/go-platform-kit/errors	(cached)	coverage: 44.2% of statements
ok  	github.com/cetrictech/go-platform-kit/health	(cached)	coverage: 80.0% of statements
ok  	github.com/cetrictech/go-platform-kit/httpx/client	(cached)	coverage: 74.1% of statements
ok  	github.com/cetrictech/go-platform-kit/httpx/middleware	0.375s	coverage: 38.8% of statements
ok  	github.com/cetrictech/go-platform-kit/id	(cached)	coverage: 100.0% of statements
ok  	github.com/cetrictech/go-platform-kit/idempotency	0.373s	coverage: 30.8% of statements
ok  	github.com/cetrictech/go-platform-kit/instrument	(cached)	coverage: 66.7% of statements
	github.com/cetrictech/go-platform-kit/internal/redact		coverage: 0.0% of statements
ok  	github.com/cetrictech/go-platform-kit/lifecycle	(cached)	coverage: 78.6% of statements
ok  	github.com/cetrictech/go-platform-kit/logger	0.358s	coverage: 45.9% of statements
ok  	github.com/cetrictech/go-platform-kit/messaging/kafka	(cached)	coverage: 37.2% of statements
ok  	github.com/cetrictech/go-platform-kit/messaging/outbox	(cached)	coverage: 15.4% of statements
ok  	github.com/cetrictech/go-platform-kit/pagination	(cached)	coverage: 31.6% of statements
ok  	github.com/cetrictech/go-platform-kit/ratelimit	(cached)	coverage: 65.3% of statements
ok  	github.com/cetrictech/go-platform-kit/resilience	(cached)	coverage: 77.6% of statements
ok  	github.com/cetrictech/go-platform-kit/response	0.374s	coverage: 46.3% of statements
ok  	github.com/cetrictech/go-platform-kit/retry	(cached)	coverage: 59.5% of statements
ok  	github.com/cetrictech/go-platform-kit/security	0.593s	coverage: 48.0% of statements
ok  	github.com/cetrictech/go-platform-kit/validation	(cached)	coverage: 65.8% of statements
ok  	github.com/cetrictech/go-platform-kit/worker	(cached)	coverage: 77.1% of statements
ok  	github.com/cetrictech/go-platform-kit/database/postgres	(cached)	coverage: 1.0% of statements
ok  	github.com/cetrictech/go-platform-kit/database/mysql	(cached)	coverage: 1.5% of statements
ok  	github.com/cetrictech/go-platform-kit/cache/redis	0.426s	coverage: 0.8% of statements
ok  	github.com/cetrictech/go-platform-kit/messaging/kafka/franz	(cached)	coverage: 16.1% of statements
ok  	github.com/cetrictech/go-platform-kit/messaging/kafka/sarama	(cached)	coverage: 19.0% of statements
ok  	github.com/cetrictech/go-platform-kit/grpcx	0.462s	coverage: 62.6% of statements
ok  	github.com/cetrictech/go-platform-kit/spiffe	(cached)	coverage: 66.0% of statements
ok  	github.com/cetrictech/go-platform-kit/observability	0.513s	coverage: 31.7% of statements
```

Adapter unit coverage is lower than their integration coverage; neither implies full failure-mode coverage. Tagged integration results include their own per-adapter coverage. See [implementation status](implementation-status.md) for unverified scenarios and remaining extensions. No production readiness certification, release publication, or tag creation follows from these results.
