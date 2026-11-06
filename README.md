# go-platform-kit

Modular Go infrastructure SDK. Module path: `github.com/cetrictech/go-platform-kit`. License: MIT. Default branch on GitHub is `main`; this work lives on `develop`.

**Status:** v1.1.0 release candidate. Tagging and publication are maintainer-controlled. Go 1.26.0 minimum; the workspace toolchain is Go 1.27.3. CI matrices Go 1.26.x and 1.27.x.

The root module has no database, Kafka, gRPC, Redis, SPIFFE, JWT, OpenTelemetry, or S3 drivers. Those live in nested modules listed in `go.work`. `integration/` is test-only and is not a consumer module.

## Setup

```sh
git clone git@github.com:cetrictech/go-platform-kit.git
cd go-platform-kit
git checkout develop
```

Requires a patched Go 1.26+ toolchain, Git, and Make. Integration and fault tests also need Docker Compose.

```sh
make test          # all modules, GOWORK=off, no infrastructure
make race
make vet
make check-fmt
make services      # loopback PostgreSQL 18, MySQL 8.4, Redis 8, Redpanda v25.3.1
make integration
make services-down
```

Release-oriented targets in this tree: `make verify`, `make api`, `make api-surface`, `make independent`, `make faults`, `make performance`, `make gates`, `make release-check`.

## Modules

| Path | Role |
| --- | --- |
| `github.com/cetrictech/go-platform-kit` | config, logger, errors, HTTP, pagination, validation, lifecycle, health, retry, resilience, ratelimit, idempotency, clock, encodingx, security, id, instrument, worker, Kafka contracts, outbox, database scan |
| `.../database/postgres` | pgx pool |
| `.../database/mysql` | database/sql MySQL |
| `.../cache/redis` | go-redis |
| `.../messaging/kafka/franz` | franz-go |
| `.../messaging/kafka/sarama` | IBM Sarama |
| `.../grpcx` | gRPC server and client |
| `.../spiffe` | SPIRE Workload API source and mTLS |
| `.../security/jwt` | JWT / JWKS |
| `.../observability` | OpenTelemetry |
| `.../storage/s3` | S3-compatible object client |

The root release candidate is `v1.1.0`. Nested modules remain independently versioned and use directory-prefixed tags when published (`spiffe/v1.1.0`). A root tag does not release nested modules.

Consumer import (after a tag exists):

```sh
go get github.com/cetrictech/go-platform-kit@v1.1.0
go get github.com/cetrictech/go-platform-kit/spiffe@v1.1.0
```

Until tags exist, consume from a commit on this repository. Local `replace` directives in nested `go.mod` files are for this workspace only; consumers do not inherit them.

## Repository layout

```text
config/ logger/ errors/ httpx/ ...   root packages
database/postgres mysql              SQL adapters
cache/redis                          Redis adapter
messaging/kafka/franz sarama         Kafka adapters
grpcx/ spiffe/ security/jwt          RPC and identity
observability/ storage/s3            telemetry and objects
integration/                         Compose fixtures and tagged tests
docs/                                module documentation
release/                             module manifest and API contracts
scripts/                             test, API, and release gates
.github/workflows/                   CI, benchmarks, release validation
```

Integration Compose: `integration/compose.yaml`. Loopback ports: PostgreSQL `127.0.0.1:15432`, MySQL `13306`, Redis `16379`, Redpanda `19092`. Fixture passwords are test-only. Generated TLS material is gitignored under `integration/.runtime/`.

## Documentation

Start at [docs/README.md](docs/README.md). Library behavior, ownership, and SPIFFE/gRPC wiring: [docs/library.md](docs/library.md). Changelog: [CHANGELOG.md](CHANGELOG.md).
