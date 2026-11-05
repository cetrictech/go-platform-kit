# Testing

`make test`, `make race`, `make vet`, `make staticcheck`, `make vuln` and `make coverage` run all modules independently with workspace disabled. Install compatible staticcheck and govulncheck before those targets. `make check-fmt` validates formatting. `make services` provisions isolated PostgreSQL/MySQL/Redis/Redpanda on loopback using Docker Compose. `make integration` sets the test connection settings and runs tagged tests with race detection. `make services-down` removes the test containers.

Unit tests require no external infrastructure; local HTTP/gRPC and SPIFFE protocol endpoints are created inside tests. Integration tests skip when their individual connection environment variable is absent; the integration script always supplies all required variables and therefore attempts real connections. Never count skipped tests as passed infrastructure validation.

See validation.md for the actual commands/results from this implementation session. The integration suite is representative and does not yet cover every deployment failure mode, multi-node cluster, production SPIRE attestation, or every optional driver feature. Release qualification must include those scenarios relevant to the consuming application.

The test-only integration module checks all Franz/Sarama producer-consumer combinations, message headers/keys and recovery from the last uncommitted record. Database integration tests additionally exercise NULL-aware reverse keysets, batch execution and SQL outbox claim/delivery-state persistence. gRPC unit tests include real verified mutual TLS and rejection of an absent client certificate.
