# SPIFFE Workload API

Module: `github.com/cetrictech/go-platform-kit/spiffe`. Go 1.26.0. Depends on `github.com/spiffe/go-spiffe/v2` v2.9.0 and `google.golang.org/grpc` v1.85.0. No dependency on the root kit module.

## Constructors

`New(ctx, Config) (*Source, error)` opens a rotating `workloadapi.X509Source`, waits for the first SVID, and checks it against `TrustDomain` and `ExpectedID`.

Required: `TrustDomain`. Required peer policy: non-empty `AllowedPeers`, non-nil `Authorize`, or `AllowTrustDomain`.

`Socket`:

- Empty: go-spiffe selects `SPIFFE_ENDPOINT_SOCKET` from the environment itself.
- Absolute path: prefixed with `unix://`.
- URL: must be `unix:///absolute/path` with no host, query, or fragment.

When a unix URL is configured, `New` stats the path before waiting. A missing file or a non-socket fails immediately. `Timeout`, if positive, wraps `ctx` for that wait only. A deadline error names `ExpectedID` and the socket.

`ExpectedID`, when set, must live in `TrustDomain`. The default SVID picker returns that ID if the agent issues several SVIDs.

Peer IDs in `AllowedPeers` must parse as SPIFFE IDs in `TrustDomain`. With a non-empty allowlist, only those IDs pass. With `AllowTrustDomain` and an empty allowlist, any ID in the domain passes. `Authorize` may accept additional IDs in the same domain.

## TLS

`ClientTLS` and `ServerTLS` return live `tls.Config` values from `tlsconfig.MTLSClientConfig` / `MTLSServerConfig`. MinVersion is TLS 1.3. Do not copy certificates into a static config. Clone if a caller must mutate. Keep `Source` open until dependent listeners and clients close.

`Identity()` returns the current ID. `GetX509SVID()` is the `x509svid.Source` implementation and rejects rotations that leave the trust domain or expected ID.

`Parse` validates a SPIFFE ID string.

`Source.Allows(peer)` applies the same authorizer used in TLS.

`PeerID(state)` reads `VerifiedChains[0][0]`. `IDFromContext` reads gRPC `peer.AuthInfo` as `credentials.TLSInfo`. Both return `false` when the chain is missing.

## FromEnv

```go
cfg, enabled, err := spiffe.FromEnv()
if err != nil { return err }
if !enabled { /* plaintext gRPC */ }
source, err := spiffe.New(ctx, cfg)
```

| Variable | Effect |
| --- | --- |
| `GRPC_SECURITY_MODE` | `spire` enables. `plaintext` and `insecure` do not. Other values error. |
| `APP_ENABLE_SPIFFE` | `strconv.ParseBool`. True enables even when mode is plaintext. |
| `SPIRE_AGENT_SOCKET` | Socket, first choice. |
| `SPIFFE_ENDPOINT_SOCKET` | Socket, second choice. Default `unix:///run/spire/agent/public/api.sock` when enabled. |
| `SPIFFE_TRUST_DOMAIN` | Trust domain, first choice. |
| `APP_SPIFFE_TRUST_DOMAIN` | Trust domain, second choice. Required when enabled. |
| `SPIFFE_ID` | Expected ID, first choice. |
| `APP_SPIFFE_ID` | Expected ID, second choice. Required when enabled. |
| `APP_SPIFFE_ALLOWED_IDS` | CSV exact peers. Empty entries are rejected. |
| `SPIFFE_ALLOWED_CLIENT_IDS` | CSV peers merged after the previous list. |
| `APP_SPIFFE_STARTUP_TIMEOUT` | Positive Go duration. Default `10s` when enabled. |

When enabled and both allowlists are empty, `AllowTrustDomain` is true. Names match the Staypace Cloud services that will import this module. This repository does not ship those services.

Call `New` only when `enabled` is true. `FromEnv` does not dial the agent.

## Ownership

Concurrent handshakes may call `GetX509SVID`. Do not mutate returned SVID keys or certificates. Close `Source` after gRPC/HTTP servers that used its TLS configs have shut down. Native `workloadapi.X509Source` reconnection stays inside `Source`.

TLS peer authorization is not application authorization. Map `IDFromContext` onto service allowlists in the consuming authenticator.

## Tests

`spiffe/spiffe_test.go` runs a unix-domain Workload API, issues ECDSA certificates, completes mTLS, rotates an SVID, rejects a missing policy, rejects a missing socket, and checks `FromEnv`. `integration/spiffe_test.go` (tag `integration`) uses `grpcx` with those TLS configs.

That is protocol coverage, not SPIRE node attestation. Mount the agent socket and join its group in the consuming Compose file.

## Errors

Public strings include `spiffe: peer authorization policy required`, `spiffe: peer outside configured trust domain`, `spiffe: expected identity must belong to the trust domain`, `spiffe: socket must be an absolute unix:/// URL`, `spiffe: workload identity mismatch`, `spiffe: timed out waiting for …`, `spiffe: GRPC_SECURITY_MODE must be spire, plaintext, or insecure`, and `spiffe: SPIFFE_TRUST_DOMAIN and SPIFFE_ID are required when SPIRE is enabled`.
