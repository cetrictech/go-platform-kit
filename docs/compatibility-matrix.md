# Module compatibility matrix

`release/modules.json` is the machine-readable source of proposed module versions and core requirements. A module version is independent: updating an adapter does not require tagging unrelated adapters. Rows below are development candidates, not published or certified production versions. There are currently **no previous release tags**.

| Module suffix | Proposed version | Declared core requirement | Go minimum | Native API dependency / exercised infrastructure line |
| --- | --- | --- | --- | --- |
| root | v0.1.0 | not applicable | 1.26.0 | standard library; UUID, bcrypt and log rotation dependencies |
| database/postgres | v0.1.0 | v0.1.0 | 1.26.0 | pgx v5.12.0; PostgreSQL 18 |
| database/mysql | v0.1.0 | v0.1.0 | 1.26.0 | go-sql-driver/mysql v1.10.1; MySQL 8.4 |
| cache/redis | v0.1.0 | v0.1.0 | 1.26.0 | go-redis v9.23.0; Redis 8 standalone, Cluster, Sentinel and TLS |
| messaging/kafka/franz | v0.1.0 | v0.1.0 | 1.26.0 | franz-go v1.22.2; Redpanda v25.3.1, Kafka protocol |
| messaging/kafka/sarama | v0.1.0 | v0.1.0 | 1.26.0 | IBM Sarama v1.61.1; Redpanda v25.3.1, Kafka protocol |
| grpcx | v0.1.0 | v0.1.0 | 1.26.0 | gRPC v1.85.0 and protobuf v1.36.12 |
| spiffe | v0.1.0 | none | 1.26.0 | go-spiffe v2.9.0; protocol-compatible local Workload API, gRPC v1.85.0 |
| observability | v0.1.0 | v0.1.0 | 1.26.0 | OpenTelemetry v1.47.0; HTTPS OTLP protobuf receiver |
| security/jwt | v0.1.0 | none | 1.26.0 | golang-jwt v5.3.1; HTTPS JWKS |
| storage/s3 | v0.1.0 | none | 1.26.0 | aws-sdk-go-v2 service/s3 v1.114.7; S3-compatible HTTP fixture |

Use the prefix `github.com/cetrictech/go-platform-kit` for module paths. Integration is a separate **test-only** module and is never a consumer release. Exact dependency selections are in [dependencies](dependencies.md) and each go.mod. `spiffe`, `security/jwt` and `storage/s3` deliberately have no core dependency; assigning them a core compatibility requirement would be inaccurate.

All seven declared core-dependent adapters are tested with the exact declared core requirement and the candidate core in independent module graphs. Go's minimal version selection may select newer core versions when a consuming application adds other modules; the consumer must run compatibility/integration checks for its selected graph. A successful workspace build alone does not establish adapter/core compatibility. `scripts/check-independent.sh` exercises copied modules outside the workspace; clean-consumer checks ensure SDK imports work without importing raw drivers.

The exercised server lines above are the initial supported test matrix, not a promise that every wire-compatible implementation or older server version is supported. Expansion requires a real-infrastructure CI job, an updated row, and validation evidence. In particular, the Kafka fixture is a real single-node Redpanda deployment; upstream Apache Kafka versions and clustered production brokers need explicit additional qualification. The Workload API test does not attest a real SPIRE deployment. See the [support policy](support-policy.md).
