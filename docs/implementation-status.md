# Implementation and release status

The SDK implements the required packages as working, independently importable modules. The follow-up implementation adds strict JWT/JWKS authentication, durable PostgreSQL idempotency, typed query pagination, SQL hooks/prepared operations/replica selection, rotating Kafka authentication, processed-only automatic commits, HTTP retry/file transfer, bounded workers and health checks, and broader real-service verification.

| Area | Implemented behavior | Executed verification |
| --- | --- | --- |
| Foundation | transactional configuration/profiles/validation, safe bounded dumps, logging ownership/flush/rotation, errors, UUIDs, validation | unit/race tests, output failure/ownership and secret boundaries |
| HTTP | safe response envelopes, middleware/authentication, bounded/replay-aware retries, Retry-After, streaming upload and atomic download | real local HTTP tests, cancellation, body limits, retry safety and file preservation |
| Pagination | bounded offsets, signed/versioned expiring cursors, key rotation/providers, typed int64-safe keys, forward/backward query helpers | tamper/binding tests and real PostgreSQL/MySQL pagination with duplicates and NULLs |
| SQL | verified TLS, pools, cached scanning/JSON, named parameters, hooks, safe logs, prepared operations, transactions/savepoints, batches, explicit primary/replica pools | real PostgreSQL/MySQL queries, rollback, PostgreSQL COPY/listen/advisory operations and mutual TLS |
| Redis | standalone/Cluster/Sentinel, structures, scripts, pipelines/WATCH, streams/pending, subscriptions, token locks, rate limiting, idempotency | real atomic/structure tests, three-node slot routing, Sentinel failover and mutual TLS |
| Kafka | independent adapters, acknowledged/async delivery, manual/processed-only automatic commits, bounded partition workers, retry/DLQ, authentication providers, administration, SQL outbox | eight adapter/commit-mode offset and same-consumer restart cases, bounded worker/order/drain tests, real DLQ/outbox delivery, SCRAM256/512 mTLS, credential rotation and network interruption/recovery |
| gRPC/SPIFFE | secure factories, unary/stream authentication/deadlines/rate policies, safe errors, correlation, health, bounded shutdown, rotating workload identities | real gRPC mTLS; Workload API certificate rotation and peer rejection through SDK factories; stream deadlines and forced shutdown |
| JWT | isolated module, strict claims/algorithm policy, static keys or bounded HTTPS JWKS cache, refresh/rotation, HTTP/generic gRPC authentication integration | signature/tamper/claim tests, real TLS JWKS rotation/outage/throttle/cancellation |
| Idempotency | memory/Redis leases and durable PostgreSQL ownership-token transitions | concurrent acquisition, stale ownership, renewal/failure/completion and cleanup |
| Utilities/observability | retry, breaker/bulkhead, clocks/codecs, named workers/statistics, health/lifecycle, injected OpenTelemetry providers | unit/race tests, trace propagation, real TLS OTLP protobuf trace/metric export and flush/shutdown |
| Quality | isolated modules, supported toolchain checks, static/security checks, clean consumer imports and documented benchmarks | current evidence in validation.md; CI is configured but was not executed on GitHub in this session |

## Operational boundaries

Integration fixtures are disposable local services, including a single Redpanda broker. They establish behavior in the recorded environments; they do not certify every database topology, broker deployment, workload attestation policy or production capacity. Replica promotion, cloud/SPIRE deployment, long-running soak tests and application-specific authorization remain operator/application responsibilities. Cluster routing is explicit; the SDK does not parse SQL to infer consistency.

Kafka publish cancellation after enqueue has an unknown delivery outcome. Kafka headers use a map and cannot preserve duplicate names. DLQ publication and source commit are separate operations. SQL outbox delivery is at least once; a broker acknowledgment followed by SQL commit failure may duplicate delivery. Deduplicate using event IDs. Sarama rotating PLAIN credentials are explicitly rejected; rotating SCRAM and OAuth callbacks are supported. OAuth token validation/callbacks are implemented, while an external OAuth identity-provider/broker deployment is not part of the local fixture.

Context deadlines bound SDK waits. Arbitrary user callbacks that ignore cancellation cannot be forcibly terminated. Health checks cap each registered check to one in-flight invocation. Workers and handlers must cooperate with shutdown. Structured redaction and common labeled credential masking cannot reliably discover arbitrary unlabeled secrets. Cursor payloads are signed, not encrypted. HTTP idempotency intentionally stores response metadata rather than sensitive replay bodies.

Object storage remains an optional [design](object-storage.md), as allowed by the request. Protobuf/Avro/schema-registry codecs can implement the serializer contract; no mandatory cloud or schema-registry dependency is added. No distributed scheduler is claimed. No tags, releases or publication are performed. Initial release version remains v0.1.0, subject to maintainer review and deployment qualification.
