# Proposed v0.1.0 release notes

Initial development release of the modular SDK. Core utilities and isolated PostgreSQL, MySQL, Redis, Franz-go, Sarama, gRPC, SPIFFE and OpenTelemetry modules are implemented. The integration module is private test infrastructure and must not be tagged as a consumer adapter.

Default remote infrastructure transport requires verified TLS; local plaintext must be explicitly enabled. Customer responses hide technical diagnostics. Redis/idempotency leases and SQL outbox processing do not promise end-to-end exactly-once execution. No automatic global initialization or migrations occur.

There is no previous SDK API to migrate. Read versioning.md for directory-prefixed adapter tags and migration-guide.md for adoption. Exact dependency pins are in dependencies.md. Test commands and actual outcomes are in validation.md; remaining qualification is in implementation-status.md. Maintainers must qualify relevant deployment scenarios before publishing. These are proposed notes, not evidence of a published release.
