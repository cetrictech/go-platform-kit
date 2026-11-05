# Implementation and release status

This is a working pre-1.0 SDK implementation, **not a production readiness certification**. The original request is broader than what the current qualification suite establishes. There are no placeholder source methods; the following delivered components execute real operations:

| Area | Implemented behavior | Executed verification |
| --- | --- | --- |
| Foundation | typed env/JSON configuration, recursive secret dumps, slog/rotation, application errors, UUIDs, validation, explicit lifecycle | unit and race tests |
| HTTP | bounded JSON envelopes, safe error policy, middleware/compression, client, validation decoding | local HTTP tests, encoding/security boundaries |
| Pagination | bounded offsets, signed/versioned expiring cursors, key rotation, query/filter binding, NULL-aware composite seek SQL | tamper/binding tests and PostgreSQL/MySQL forward/reverse queries |
| SQL adapters | verified TLS policy, pools, scanning, queries, transactions/rollback, batches, PostgreSQL COPY/listen/advisory primitives | real transaction/batch/scan/pagination/outbox-state tests; not every native capability exercised |
| Redis | universal connections, data structures, scripts, pipelines/transactions, streams, subscriptions, token locks, rate limiting, idempotency | real JSON, ownership renewal/release and concurrent idempotency tests |
| Kafka | independent adapters, sync/async delivery, shared metadata/ack intent, manual commits, bounded workers, retries/DLQ helper, administration, SQL outbox | real publish/consume/admin tests and all four cross-driver offset combinations |
| gRPC/SPIFFE | secure factories, interceptors, request IDs, rate hook, connection bounds, health/shutdown, rotating Workload API identities and exact peer policy | real gRPC mTLS, rejected missing client certificate, protocol-compatible workload endpoint, TLS handshake and certificate rotation |
| Production utilities | retry, breaker/bulkhead, clocks, serialization, bounded workers, health, OpenTelemetry trace/metric/OTLP construction and propagation | unit/race tests, local trace propagation tests |
| Quality/release | separate-module checks, supported-version CI, static/security targets, release validation, docs and nested tag rules | see validation.md and recorded logs |

## Remaining qualification and limitations

- The entire original production acceptance specification is **not yet fully qualified**. Unit coverage is recorded honestly; many adapter success paths are covered through integration rather than isolated unit tests. No blanket production-readiness claim is made.
- Multi-node PostgreSQL routing, MySQL failover, Redis Sentinel/Cluster failover, Kafka broker loss/rebalance/poison-message fault matrices, SASL/OAuth deployment handshakes and cloud workload-attestation policies still need application-specific integration runs. The current broker is a single Redpanda node. Current Redis integration uses standalone mode.
- Automatic primary/replica routing and a uniform transaction-hook registration API are not implemented. Applications can compose explicitly configured clients and transaction callbacks. PostgreSQL prepared statements use pgx's native cache; advanced native configuration remains driver-specific.
- The scanner uses safe reflection without a metadata cache, does not flatten embedded pointer structs, and does not promise arbitrary JSON-to-struct conversion. Mapping rules are documented.
- Kafka header maps cannot preserve duplicate names. Acknowledgment intent requests a post-handler commit; it is not a commit receipt. DLQ publishing and offset commits are not atomic. SQL outbox delivery is at least once and can duplicate after a broker acknowledgment/SQL commit failure.
- A durable PostgreSQL idempotency store is optional and not implemented. Redis/memory processing leases cannot fence an expired owner. HTTP idempotency stores response metadata and intentionally does not replay sensitive bodies.
- Strict hard termination of context-ignoring user callbacks is not possible. Hooks, workers, handlers, health checks and instrumentation callbacks must respect their documented cancellation/lifecycle contracts.
- Arbitrary free-text secrets cannot be reliably redacted. Structured redaction and default-hidden diagnostics are implemented; callers must keep credentials out of free-text messages and public fields. Cursor payloads are authenticated, not encrypted.
- Object storage is a documented optional design, not an implemented cloud adapter. Protobuf/Avro/schema-registry serializers, JWT/JWKS verification and a distributed scheduler are not included; generic serialization/authentication hooks remain available.
- Consumer-throughput benchmarks, complete network failure/cleanup matrices, long-running soak tests, external OTLP collector tests, and actual SPIRE deployment tests remain. CI workflows are configured but have not been executed on GitHub in this session.

Recommended initial version after maintainer qualification: **v0.1.0**. Do not publish a production release until relevant remaining qualification is complete. No Git tags or releases are created by the implementation process.
