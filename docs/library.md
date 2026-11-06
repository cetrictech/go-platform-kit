# Library guide

This document describes `github.com/cetrictech/go-platform-kit` as it exists in this repository. It does not describe Staypace Cloud or Darena deploy hosts. Those applications may import these modules later.

## What this library is

A set of independently versioned Go modules for process infrastructure: configuration, logs, HTTP, SQL, Redis, Kafka, gRPC, SPIFFE/SPIRE workload mTLS, JWT, OpenTelemetry, and optional S3. Importing the root module does not pull optional drivers. Nested adapters declare their own `go.mod`.

There are no connections, signal handlers, migrations, or OpenTelemetry globals at import time. Call constructors. Close servers and workers before clients.

## Toolchain and workspace

| Fact | Value |
| --- | --- |
| Root module | `github.com/cetrictech/go-platform-kit` |
| Nested modules | `database/postgres`, `database/mysql`, `cache/redis`, `messaging/kafka/franz`, `messaging/kafka/sarama`, `grpcx`, `spiffe`, `observability`, `security/jwt`, `storage/s3` |
| Test-only module | `integration` |
| `go.work` | lists every nested module plus `integration` |
| Minimum Go | 1.26.0 (`go.mod`) |
| Toolchain | 1.27.3 |
| License | MIT, Copyright 2026 Cetric Okola |
| Proposed versions | `v0.1.0` in `release/modules.json` |
| Tags | none |

`scripts/modules.sh` runs commands with `GOWORK=off` so each module resolves like a published consumer.

## Construction and shutdown

1. Load configuration (`config.Load`).
2. Construct logger, then clients (SQL, Redis, Kafka, SPIFFE source, JWT verifier).
3. Construct gRPC/HTTP servers with TLS from the SPIFFE source when SPIRE is enabled.
4. Register `lifecycle.Manager` stop hooks in reverse dependency order.
5. On shutdown: stop servers and workers, then close the SPIFFE source, then close data clients, then close the logger.

`lifecycle.Manager` starts hooks in order and stops successful hooks in reverse. Hooks must honor context cancellation. The SDK does not kill goroutines that ignore cancellation.

SPIFFE `Source` must stay open for the lifetime of every TLS listener and client that used `ClientTLS` or `ServerTLS`. Close it after those connections drain.

## Root packages

| Package | Responsibility |
| --- | --- |
| `config` | Typed env/JSON load. Tags: `env`, `default`, `required`, `secret`. No `.env` file loader. |
| `logger` | JSON slog, service fields, file rotation via lumberjack when configured. Caller closes. |
| `errors` | `AppError` with public code/message; diagnostics stay off the wire. |
| `response` | HTTP JSON envelopes, pagination wrappers, 1 MiB default atomic limit. |
| `httpx/middleware` | Recovery, IDs, CORS, body limit, auth hooks. |
| `httpx/client` | Bounded HTTP client; caller closes streaming bodies. |
| `validation` | Bounded JSON decode, unknown-field rejection. |
| `pagination` | Offset and signed cursors. |
| `database` | Shared scan/JSON helpers used by SQL adapters. |
| `messaging/kafka` | Driver-free producer/consumer contracts. |
| `messaging/outbox` | SQL outbox processing; at-least-once. |
| `lifecycle`, `health`, `worker` | Ordered start/stop, bounded checks, named workers. |
| `retry`, `resilience`, `ratelimit`, `idempotency` | Backoff, breaker/bulkhead, limits, leases. |
| `security`, `id`, `clock`, `encodingx`, `instrument` | crypto helpers, UUIDs, clocks, codecs, metrics hooks. |

Remote production transports require verified TLS unless the adapter documents an explicit plaintext opt-in for local tests.

## Nested adapters

| Import | Driver | Notes |
| --- | --- | --- |
| `.../database/postgres` | pgx v5 | Pool, tx, COPY, listen, advisory locks. |
| `.../database/mysql` | go-sql-driver/mysql | Pool, tx, prepared statements. Zero `MaxOpenConnections` becomes 32. |
| `.../cache/redis` | go-redis v9 | Standalone, Cluster, Sentinel. `PoolSize` bounds commands per node. |
| `.../messaging/kafka/franz` | franz-go | One `Consume` loop per consumer. |
| `.../messaging/kafka/sarama` | Sarama | Rotating PLAIN is rejected. |
| `.../grpcx` | google.golang.org/grpc | TLS or `AllowPlaintext`. Caller owns the listener. |
| `.../spiffe` | go-spiffe v2 | Workload API source. No core dependency. |
| `.../security/jwt` | golang-jwt | JWKS or static keys. No core dependency. |
| `.../observability` | OpenTelemetry | Injected providers; no globals. |
| `.../storage/s3` | aws-sdk-go-v2 | S3-compatible HTTP. No ambient credentials. |

## SPIFFE and gRPC

`spiffe` is the Workload API client. `grpcx` is the gRPC factory. Wire them by passing `source.ServerTLS()` / `source.ClientTLS()` into `grpcx.ServerConfig.TLS` and `grpcx.ClientConfig.TLS`.

### `spiffe.Config`

| Field | Meaning |
| --- | --- |
| `Socket` | `unix:///absolute/path.sock`. A leading `/` is prefixed with `unix://`. Empty uses go-spiffe endpoint discovery. |
| `TrustDomain` | Required. Example: `darena.internal`. |
| `ExpectedID` | Optional pin of this process’s SPIFFE ID. When set, that SVID is selected if the agent issues several. |
| `AllowedPeers` | Exact caller IDs in the same trust domain. |
| `Authorize` | Optional callback `func(string) bool`. |
| `AllowTrustDomain` | When true and `AllowedPeers` is empty, any ID in `TrustDomain` is accepted. |
| `Timeout` | Bounds `New` only. Zero uses the caller context as-is. |

`New` requires `TrustDomain` and one peer policy: `AllowedPeers`, `Authorize`, or `AllowTrustDomain`. A configured unix socket is `stat`’d before the Workload API wait. Missing paths fail immediately. `ClientTLS` and `ServerTLS` set TLS 1.3.

### `FromEnv`

`FromEnv() (Config, bool, error)` reads process environment. It does not open a source.

Enabled when `GRPC_SECURITY_MODE=spire` or `APP_ENABLE_SPIFFE=true`.

| Variable | Role |
| --- | --- |
| `GRPC_SECURITY_MODE` | `spire`, `plaintext`, or `insecure`. Other values fail. Empty is off. |
| `APP_ENABLE_SPIFFE` | Boolean. Also enables the source. |
| `SPIRE_AGENT_SOCKET` | Preferred socket. Absolute paths get `unix://`. |
| `SPIFFE_ENDPOINT_SOCKET` | Fallback socket. Default when enabled: `unix:///run/spire/agent/public/api.sock`. |
| `SPIFFE_TRUST_DOMAIN` | Trust domain. |
| `APP_SPIFFE_TRUST_DOMAIN` | Fallback trust domain. |
| `SPIFFE_ID` | Expected workload ID. Required when enabled. |
| `APP_SPIFFE_ID` | Fallback expected ID. |
| `APP_SPIFFE_ALLOWED_IDS` | Comma-separated exact peers. |
| `SPIFFE_ALLOWED_CLIENT_IDS` | Merged into the allowlist. |
| `APP_SPIFFE_STARTUP_TIMEOUT` | Positive duration. Default `10s` when enabled. |

An empty allowlist with SPIRE enabled sets `AllowTrustDomain`. A non-empty allowlist disables it. Peer IDs must belong to the trust domain.

### Peer helpers

- `PeerID(tls.ConnectionState) (string, bool)` — verified leaf URI.
- `IDFromContext(context.Context) (string, bool)` — gRPC `credentials.TLSInfo`.
- `Source.Allows(id string) bool` — configured authorizer, no extra RPC.

These identify a workload. They do not grant user, tenant, or OAuth rights.

### Example

```go
cfg, enabled, err := spiffe.FromEnv()
if err != nil {
    return err
}
var serverTLS *tls.Config
if enabled {
    source, err := spiffe.New(ctx, cfg)
    if err != nil {
        return err
    }
    defer source.Close()
    serverTLS = source.ServerTLS()
}
server, err := grpcx.NewServer(grpcx.ServerConfig{TLS: serverTLS, AllowPlaintext: !enabled})
```

gRPC plaintext still requires `AllowPlaintext: true`. Do not pass a nil TLS config into production.

### What this module does not do

It does not run a SPIRE agent, create Docker labels, set `SPIRE_GID`, or mount `/run/spire/agent/public`. Those belong in the consuming service’s Compose and deploy files. Unit tests speak a local Workload API; they do not attest a production agent.

## Configuration loading

`config.Load` updates a struct only after conversion and validation succeed. Precedence: defaults, existing values, JSON file, environment overlay, `Options.Values`. Unknown JSON fields fail. Secret-tagged fields are masked in dumps. Nested env names use underscores under `Options.Prefix`.

## Errors and HTTP

`errors.AppError` carries a stable public code. `response.Error` writes that code and message. Driver strings stay in logs. gRPC `grpcx` maps the same errors without leaking causes. Caller-supplied `status.Error` values are treated as intentional public errors.

## Kafka and outbox

Publish may complete after the caller context is canceled once the record is enqueued. Delivery is at-least-once. Deduplicate on event IDs. Outbox SQL schemas are `docs/outbox-postgres.sql` and `docs/outbox-mysql.sql`. A broker ack followed by a failed SQL commit can duplicate.

## Tests

| Target | Infrastructure |
| --- | --- |
| `make test` / `make race` | none |
| `make services` | Docker Compose in `integration/compose.yaml` plus extended TLS fixtures |
| `make integration` | requires `make services`; race, build tag `integration` |
| `make faults` | restarts, rebalance, network interruption |

SPIFFE unit tests create a unix socket Workload API inside `spiffe/spiffe_test.go`. Integration `integration/spiffe_test.go` exercises SDK gRPC plus that protocol.

## Release

`scripts/release-check.sh` is the prepublication gate. `scripts/check-api.sh` compares against the last module tag, or `bootstrap_ref` in `release/modules.json` while no tags exist. `release/consumers.json` is empty; v1 admission is blocked until real consuming projects are recorded.

This repository does not tag or publish as part of ordinary commits.
