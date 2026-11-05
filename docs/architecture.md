# Architecture

The root module contains application-agnostic contracts and utilities. PostgreSQL, MySQL, Redis, each Kafka driver, gRPC, SPIFFE and OpenTelemetry are nested modules. There are no infrastructure connections, signal registrations, migrations or telemetry global registrations during import.

Use constructors explicitly. Construct dependencies before application servers; close servers and workers before clients. `lifecycle.Manager` starts registered hooks in order and stops successful hooks in reverse order. Hooks must respect cancellation; the SDK cannot forcibly terminate arbitrary application callbacks.

Adapters depend on core, never the reverse. `go.work` connects the local checkouts. `scripts/modules.sh` sets `GOWORK=off` to test each published module boundary independently. Local replace directives are for development and are ignored by consuming modules. See versioning before publishing.

This is an initial implementation under validation, not a production-readiness certification. See implementation-status.md for gaps relative to the requested production specification.
