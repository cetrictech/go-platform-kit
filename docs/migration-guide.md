# Migration guide

The repository previously contained a README but no Go module or implementation. There is no prior SDK API to migrate. Existing identity-service README text was replaced with SDK documentation; LICENSE and unrelated files remain intact.

For initial adoption, import only the required module, construct its client with explicit configuration, pass context to operations and register cleanup with the application's lifecycle. Move request-specific data into bounded logging/tracing attributes, never into reusable client state. Replace raw client-facing error strings with safe AppError messages and approved Details. Establish stable ordering before issuing pagination cursors. Treat Kafka handlers and outbox consumers as idempotent.

For local development, use the workspace or explicit consumer replace directives. Release tags do not exist until an authorized maintainer publishes them. Future breaking changes must include before/after snippets and compatibility notes here.
