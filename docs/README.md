# Documentation

Index of this repository. Paths are relative to `docs/` unless noted.

## Start here

| Page | Contents |
| --- | --- |
| [library.md](library.md) | End-to-end library guide: modules, construction order, SPIFFE, gRPC, env, tests |
| [architecture.md](architecture.md) | Root vs nested modules, lifecycle, no import-time connections |
| [modules.md](modules.md) | Public constructors and owned resources |
| [../CHANGELOG.md](../CHANGELOG.md) | Unreleased changes |

## Setup and quality

| Page | Contents |
| --- | --- |
| [testing.md](testing.md) | Make targets, Compose fixtures, ports, what unit vs integration covers |
| [validation.md](validation.md) | Recorded local command results |
| [implementation-status.md](implementation-status.md) | Implemented behavior vs remaining operator work |
| [versioning.md](versioning.md) | Independent module tags, v0 rules |
| [compatibility-matrix.md](compatibility-matrix.md) | Proposed versions and exercised server lines |
| [public-api-contracts.md](public-api-contracts.md) | Concurrency, cancellation, ownership |
| [support-policy.md](support-policy.md) | What a passing test suite does not certify |
| [deprecations.md](deprecations.md) | Removal policy |
| [dependency-policy.md](dependency-policy.md) | Driver upgrades and advisories |
| [dependencies.md](dependencies.md) | Exact go.mod selections |
| [migration-guide.md](migration-guide.md) | Adoption notes |
| [completion-checklist.md](completion-checklist.md) | Spec checklist |
| [release-notes.md](release-notes.md) | Proposed v0.1.0 notes (unpublished) |

## Packages

| Page | Module |
| --- | --- |
| [configuration.md](configuration.md) | root `config` |
| [logging.md](logging.md) | root `logger` |
| [errors.md](errors.md) | root `errors` |
| [http.md](http.md) | root `httpx`, `response` |
| [pagination.md](pagination.md) | root `pagination` |
| [postgres.md](postgres.md) | `database/postgres` |
| [mysql.md](mysql.md) | `database/mysql` |
| [redis.md](redis.md) | `cache/redis` |
| [kafka.md](kafka.md) | Kafka contracts |
| [kafka-franz.md](kafka-franz.md) | `messaging/kafka/franz` |
| [kafka-sarama.md](kafka-sarama.md) | `messaging/kafka/sarama` |
| [grpc.md](grpc.md) | `grpcx` |
| [spiffe.md](spiffe.md) | `spiffe` |
| [jwt.md](jwt.md) | `security/jwt` |
| [security.md](security.md) | root `security` |
| [observability.md](observability.md) | `observability` |
| [object-storage.md](object-storage.md) | `storage/s3` |
| [health.md](health.md) | root `health` |
| [retries.md](retries.md) | root `retry` |
| [idempotency.md](idempotency.md) | root `idempotency` |
| [performance.md](performance.md) | budgets |
| [performance-regressions.md](performance-regressions.md) | regression gate |

SQL outbox schemas: [outbox-postgres.sql](outbox-postgres.sql), [outbox-mysql.sql](outbox-mysql.sql).

Repository policies outside `docs/`: [../CONTRIBUTING.md](../CONTRIBUTING.md), [../SECURITY.md](../SECURITY.md), [../LICENSE](../LICENSE).
