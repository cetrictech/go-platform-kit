# Changelog

All notable changes to this repository are recorded here.

## [1.1.0] — release candidate

### Added

- Exported environment helpers in `config`: required and optional string, boolean, integer, duration, and CSV lookups, plus listen-address, port, service-URL, and origin validation.
- Shared PostgreSQL primitives in `database/postgres`: `Row`, `Rows`, `Executor`, `IsNoRows`, and `ConstraintError`.
- UUID helpers in `id`: `IsNil` and `IsV7`.
- Logger level and default-management helpers in `logger`: `Level`, `ParseLevel`, `Default`, and `SetDefault`.
- File logging configuration remains owned by the library, including lumberjack rotation, retention, compression, flush, close, and output ownership semantics.

### Changed

- Consumer services can use the root library packages for environment parsing, database error classification, UUID checks, and logger setup without copying infrastructure code or importing the underlying drivers directly.
- Release documentation now describes the v1.1.0 candidate and the independently versioned nested modules.

### Compatibility

- No gRPC wrapper was added to the root module; gRPC remains in the existing `grpcx` nested module.
- No automatic global initialization, network connections, migrations, or driver imports were added to the root module.

## [Unreleased]

### Added

- SPIFFE `FromEnv` for `GRPC_SECURITY_MODE`, `APP_ENABLE_SPIFFE`, `SPIFFE_*`, and `APP_SPIFFE_*`. Empty caller allowlists set `AllowTrustDomain`. Missing unix sockets fail before the Workload API wait. `ExpectedID` selects the matching SVID. `ClientTLS`/`ServerTLS` require TLS 1.3. `PeerID`, `IDFromContext`, and `Source.Allows` expose verified peer IDs.
- Optional `storage/s3` module (aws-sdk-go-v2). Not imported by the root module.
- Release gates: `release/modules.json`, API surface registry, `scripts/release-check.sh`, independent-module checks, contract tests, benchmark comparison, fault integration (restart, rebalance, resource bounds).
- Public contract docs: API ownership, compatibility matrix, support policy, deprecations, dependency policy, performance regressions.
- Library documentation index (`docs/README.md`) and end-to-end guide (`docs/library.md`).

### Changed

- Redis `PoolSize` is a hard command bound per node (zero selects `10*GOMAXPROCS`). MySQL zero `MaxOpenConnections` defaults to 32; idle count cannot exceed open count.
- Canceled workers drop queued jobs without running them and count Failed and Completed.
- Database scan helpers accept additional integer and null mappings used by pagination and outbox tests.
- CI runs fmt/gates, unit/race, API comparison, static/vuln, integration/faults, and performance jobs on Go 1.26.x and 1.27.x.

### Fixed

- Config load copies programmatic fields across JSON overlays without dropping non-tagged values.
- Worker shutdown no longer leaves queued closures blocked on a canceled context.

## [0.1.0] — unpublished

Initial modular SDK: root utilities, PostgreSQL, MySQL, Redis, Franz and Sarama Kafka adapters, gRPC, SPIFFE Workload API source, JWT/JWKS, OpenTelemetry, unit and Compose integration tests. No tags. No compatibility guarantee.
