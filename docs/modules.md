# Modules and imports

| Module suffix | Primary API | Owned resources |
| --- | --- | --- |
| root | config, logger, errors, response, pagination, validation, lifecycle, health, retry, resilience, ratelimit, idempotency, clock, encodingx, security, id, instrument, worker, messaging/kafka, messaging/outbox, database | File logger, worker pool, lifecycle registrations |
| database/postgres | New, Exec, Query, QueryRow, Get, Select, BeginTx, WithinTx, Batch, CopyFrom, Listen, Notify, AdvisoryLock, SQLDB | pgx pool; SQL bridge must close first |
| database/mysql | New, Exec, Query, QueryRow, Get, Select, BeginTx, WithinTx, Batch, Prepare | database/sql pool and prepared statements |
| cache/redis | New, data structures, streams, Subscribe, Eval, Pipeline, Acquire, IdempotencyStore, Limiter | universal client, subscriptions |
| messaging/kafka/franz | NewProducer, NewConsumer, NewAdmin | franz clients |
| messaging/kafka/sarama | NewProducer, NewConsumer, NewAdmin | Sarama clients and dispatch goroutine |
| grpcx | NewServer, NewClient, interceptors | server, client connection; caller creates listener |
| spiffe | New, ClientTLS, ServerTLS, Identity | Workload API source |
| security/jwt | New, Verify, Authenticate, Middleware, Refresh | JWKS cache and owned HTTP transport |
| observability | New, HTTP, Transport, Hook, Producer, Consumer | trace and metric providers |

Prefix each suffix with `github.com/cetrictech/go-platform-kit`; root packages use that same prefix. Optional driver dependencies are transitive dependencies of their adapters. They do not disappear, and importing the root module does not install those drivers.

Clients are designed for concurrent operations. Configure them before sharing. Configuration maps, hooks, TLS configs, logging outputs and callbacks must not be mutated concurrently by callers. A Kafka consumer permits one active Consume call. Row streams, transactions, response writers and individual HTTP requests are not concurrency-safe.

`integration/` is an additional test-only module, never a published SDK adapter. It verifies all four Kafka producer/consumer combinations and committed/uncommitted offset behavior.
