# Testing

`make test`, `make race`, `make vet`, `make staticcheck`, `make vuln` and `make coverage` run all modules independently with workspace disabled. Install compatible staticcheck and govulncheck before those targets. `make check-fmt` validates formatting. `make services` provisions isolated PostgreSQL/MySQL/Redis/Redpanda on loopback using Docker Compose. `make integration` sets the test connection settings and runs tagged tests with race detection. `make services-down` removes the test containers.

Unit tests require no external infrastructure; local HTTP/gRPC and SPIFFE protocol endpoints are created inside tests. Integration tests skip when their individual connection environment variable is absent; the integration script always supplies all required variables and therefore attempts real connections. Never count skipped tests as passed infrastructure validation.

See validation.md for the actual commands/results from this implementation session. The integration suite is representative and does not yet cover every deployment failure mode, multi-node cluster, production SPIRE attestation, or every optional driver feature. Release qualification must include those scenarios relevant to the consuming application.

The test-only integration module checks all Franz/Sarama producer-consumer combinations, message headers/keys and recovery from the last uncommitted record. Database integration tests additionally exercise NULL-aware reverse keysets, batch execution and SQL outbox claim/delivery-state persistence. gRPC unit tests include real verified mutual TLS and rejection of an absent client certificate.

`make services` also generates short-lived test-only certificates under ignored `integration/.runtime/certs`, starts the extended TLS/Redis-topology fixtures, and bootstraps local SCRAM users. Existing unexpired certificates are reused so running services retain the same trust root. `make services-down` stops both Compose projects. `make integration` runs real-service tests with the integration build tag and race detector. It requires those fixtures; ordinary `make test` does not.

The extended test ports are loopback-only: Redis cluster 17001–17003, Sentinel 17101–17103, Redis TLS 17479, PostgreSQL TLS 25432, MySQL TLS 23306, Kafka TLS 29092 and local bootstrap admin 29644. Fixture passwords and keys are disposable development material. Do not reuse them in production. Regenerate certificates by stopping services and removing the ignored certificate directory before `make services`.
