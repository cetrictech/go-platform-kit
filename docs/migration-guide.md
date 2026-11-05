# Migration guide

The repository previously contained a README but no Go module or implementation. There is no prior SDK API to migrate. Existing identity-service README text was replaced with SDK documentation; LICENSE and unrelated files remain intact.

For initial adoption, import only the required module, construct its client with explicit configuration, pass context to operations and register cleanup with the application's lifecycle. Move request-specific data into bounded logging/tracing attributes, never into reusable client state. Replace raw client-facing error strings with safe AppError messages and approved Details. Establish stable ordering before issuing pagination cursors. Treat Kafka handlers and outbox consumers as idempotent.

For local development, use the workspace or explicit consumer replace directives. Release tags do not exist until an authorized maintainer publishes them. Future breaking changes must include before/after snippets and compatibility notes here.

The end-to-end follow-up makes these pre-release API changes:

- MySQL Prepare returns an owned SDK Statement. Replace `stmt.ExecContext(ctx, args...)` with `stmt.Exec(ctx, args...)`; use `stmt.Native()` for explicit database/sql escape hatches. Close statements before their client.
- Offset HTTP input/output uses page_size. The old limit query parameter remains a deprecated alias; conflicting aliases fail. Response metadata now includes total_records, total_pages and type.
- QueryRow handles allow one Scan. Reusing a handle previously risked executing its SQL twice; create a new handle for a new query.
- OpenTelemetry EndpointURL denotes the collector base URL; traces and metrics use their standard separate paths. Remove signal-specific suffixes from a shared base configuration.
- Injected logging outputs remain caller-owned by default. Opt into OwnOutput for shared, exactly-once closure; use Open for eager file errors.

New capabilities are opt-in: AutoCommit, rotating authentication, JWT/JWKS, durable PostgreSQL idempotency, primary/replica pools, retry policy and streaming file helpers. Apply the durable idempotency and outbox schemas explicitly through your migration system.
