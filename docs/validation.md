# Executed validation

Validation date: **2026-11-05 (Africa/Nairobi)**. Host: macOS arm64, Apple M3 Pro. Primary Go 1.27.3; minimum supported line checked with Go 1.26.10. Staticcheck v0.8.2 and govulncheck v1.9.0. All commands use independent module graphs unless explicitly described as workspace checks.

| Check | Command | Executed result |
| --- | --- | --- |
| Unit/race | `./scripts/modules.sh go test -race -count=1 ./...` | PASS: 33 tested packages across 11 modules |
| Minimum Go | `GOTOOLCHAIN=go1.26.10 ./scripts/modules.sh go test -count=1 ./...` | PASS; final logger/Sarama follow-ups also checked on this line |
| Build/vet | modules script with `go build ./...` and `go vet ./...` | PASS |
| Staticcheck | `./scripts/modules.sh staticcheck ./...` | PASS |
| Vulnerabilities | `./scripts/modules.sh govulncheck ./...` | PASS: zero reachable vulnerabilities; advisory detail below |
| Formatting/docs | `make check-fmt`, `./scripts/check-docs.sh`, `git diff --check` | PASS |
| Dependencies | `./scripts/modules.sh go mod tidy` | Completed independently for all 11 modules |
| Workspace | `go test` with core and all adapter/integration paths | PASS |
| Clean consumer | `./scripts/check-consumer.sh` | PASS: logger-only graph excludes drivers, telemetry and JWT; all adapter imports compile without raw driver imports |
| Real services | `make services`, `./scripts/integration.sh`, `make services-down` | PASS: six tagged package suites with race detection, no skips, both Compose projects stopped afterward |
| Coverage | `./scripts/modules.sh go test -cover ./...` | Executed; exact per-package results below |
| Benchmarks | per-module `go test -run '^$' -bench . -benchtime=100x -benchmem` | Executed for core overhead and real network operations |

Ordinary inventory: **84 top-level tests**. Tagged suites add real-service tests and adapter/mechanism/commit-mode subtests. Exact outputs are in [validation logs](validation/). CI is configured but was not run on GitHub in this session. No tags or publication were performed.

## Real-service verification

- PostgreSQL/MySQL: queries, prepared operations, JSON/scanning, hooks/safe query logging, commit/rollback, batches, forward/reverse typed cursor queries with duplicate/NULL values and int64 precision, cancellation and mutual TLS. PostgreSQL additionally covers COPY, strict named arguments, nested/savepoint/panic rollback, advisory locks, LISTEN/NOTIFY, durable concurrent idempotency, explicit read-only replica selection and recovery after backend termination.
- Redis: data structures, pipelines/WATCH, scripts, streams/pending recovery, subscriptions, expiration, atomic distributed limiting, stale-owner locks and concurrent idempotency; three-node Cluster routing, Sentinel failover and mutual TLS including rejection without a client certificate.
- Kafka: acknowledged publishing, admin/offset inspection, all four producer-consumer combinations with manual and automatic commit policies, same-consumer replay after failure, bounded partition workers/order/draining, real retry/DLQ delivery and safe metadata, SQL outbox rollback/outage/retry-to-consumer delivery, cross-adapter SCRAM256/512 over mutual TLS, live credentials refreshed after forced network interruption and successful recovery.
- gRPC/SPIFFE/JWT: secure health RPCs, rejected client identities, unary/stream deadline and shutdown policies, safe public error details, JWT identity/authorization in unary and streaming RPCs, protocol-compatible Workload API identity rotation and peer rejection through SDK factories.
- OpenTelemetry: injected providers and correlation plus a real HTTPS OTLP receiver decoding protobuf trace/metric exports; Flush and Shutdown deliver the recorded data.
- Logging: file rotation/compression/retention, output ownership, shared close/level state, redaction and eager file failure.

Fixture images are PostgreSQL 18, MySQL 8.4, Redis 8 and Redpanda v25.3.1. Exact original captured server version outputs remain in the log directory. Local TLS certificates are short-lived and ignored by Git. These tests do not certify every production topology or deployment policy; see [implementation status](implementation-status.md).

## Security scan detail

All independently scanned modules report zero reachable vulnerabilities. Some graphs report GO-2026-5932 in the unused `golang.org/x/crypto/openpgp` package. The SDK uses bcrypt and does not import OpenPGP. Stable x/crypto v0.58.0 is retained rather than silently selecting the advisory's pseudo-version fix. See the [official advisory](https://pkg.go.dev/vuln/GO-2026-5932) and [exact output](validation/vulnerabilities.log). Zero reachable does not mean every required module is free of advisories.

## Ordinary coverage

Adapter unit coverage is complemented by tagged real-service coverage, not a claim of complete fault coverage. Internal redaction is exercised through importing packages; ordinary per-package instrumentation does not attribute that indirect coverage to its own package.

```text
Module: .
ok  	github.com/cetrictech/go-platform-kit/clock	(cached)	coverage: 52.4% of statements
ok  	github.com/cetrictech/go-platform-kit/config	(cached)	coverage: 60.1% of statements
ok  	github.com/cetrictech/go-platform-kit/database	(cached)	coverage: 76.1% of statements
ok  	github.com/cetrictech/go-platform-kit/encodingx	(cached)	coverage: 34.4% of statements
ok  	github.com/cetrictech/go-platform-kit/errors	(cached)	coverage: 44.2% of statements
ok  	github.com/cetrictech/go-platform-kit/health	(cached)	coverage: 72.5% of statements
ok  	github.com/cetrictech/go-platform-kit/httpx/client	(cached)	coverage: 74.2% of statements
ok  	github.com/cetrictech/go-platform-kit/httpx/middleware	(cached)	coverage: 33.2% of statements
ok  	github.com/cetrictech/go-platform-kit/id	(cached)	coverage: 100.0% of statements
ok  	github.com/cetrictech/go-platform-kit/idempotency	(cached)	coverage: 30.8% of statements
ok  	github.com/cetrictech/go-platform-kit/instrument	(cached)	coverage: 66.7% of statements
ok  	github.com/cetrictech/go-platform-kit/internal/consumerlife	(cached)	coverage: 100.0% of statements
	github.com/cetrictech/go-platform-kit/internal/redact		coverage: 0.0% of statements
ok  	github.com/cetrictech/go-platform-kit/lifecycle	(cached)	coverage: 64.7% of statements
ok  	github.com/cetrictech/go-platform-kit/logger	0.443s	coverage: 62.3% of statements
ok  	github.com/cetrictech/go-platform-kit/messaging/kafka	(cached)	coverage: 21.3% of statements
ok  	github.com/cetrictech/go-platform-kit/messaging/outbox	(cached)	coverage: 15.4% of statements
ok  	github.com/cetrictech/go-platform-kit/pagination	(cached)	coverage: 41.0% of statements
ok  	github.com/cetrictech/go-platform-kit/ratelimit	(cached)	coverage: 65.3% of statements
ok  	github.com/cetrictech/go-platform-kit/resilience	(cached)	coverage: 76.8% of statements
ok  	github.com/cetrictech/go-platform-kit/response	(cached)	coverage: 49.0% of statements
ok  	github.com/cetrictech/go-platform-kit/retry	(cached)	coverage: 50.9% of statements
ok  	github.com/cetrictech/go-platform-kit/security	(cached)	coverage: 48.0% of statements
ok  	github.com/cetrictech/go-platform-kit/validation	(cached)	coverage: 65.8% of statements
ok  	github.com/cetrictech/go-platform-kit/worker	(cached)	coverage: 75.9% of statements
Module: database/postgres
ok  	github.com/cetrictech/go-platform-kit/database/postgres	(cached)	coverage: 0.6% of statements
Module: database/mysql
ok  	github.com/cetrictech/go-platform-kit/database/mysql	(cached)	coverage: 0.9% of statements
Module: cache/redis
ok  	github.com/cetrictech/go-platform-kit/cache/redis	(cached)	coverage: 0.7% of statements
Module: messaging/kafka/franz
ok  	github.com/cetrictech/go-platform-kit/messaging/kafka/franz	(cached)	coverage: 11.5% of statements
Module: messaging/kafka/sarama
ok  	github.com/cetrictech/go-platform-kit/messaging/kafka/sarama	(cached)	coverage: 18.1% of statements
Module: grpcx
ok  	github.com/cetrictech/go-platform-kit/grpcx	(cached)	coverage: 62.1% of statements
Module: spiffe
ok  	github.com/cetrictech/go-platform-kit/spiffe	(cached)	coverage: 66.0% of statements
Module: observability
ok  	github.com/cetrictech/go-platform-kit/observability	(cached)	coverage: 46.8% of statements
Module: security/jwt
ok  	github.com/cetrictech/go-platform-kit/security/jwt	(cached)	coverage: 62.4% of statements
Module: integration
?   	github.com/cetrictech/go-platform-kit/integration	[no test files]
	github.com/cetrictech/go-platform-kit/integration/cmd/fixtures		coverage: 0.0% of statements
```

## Benchmarks

[Core results](validation/benchmarks.log) separate local SDK overhead from [network results](validation/network-benchmarks.log), which include container/server latency. Recorded network samples use 100 iterations on this host and are reproducibility evidence, not throughput guarantees. Reproduce with commands in [performance](performance.md).
