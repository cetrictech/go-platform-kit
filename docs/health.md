# Health and lifecycle

Use separate Registry instances for liveness, readiness and startup. An empty registry is healthy, appropriate for process liveness. Register dependency Ping callbacks for readiness and optional startup checks. Checks run concurrently with per-check timeout contexts (default 2 s) and safe aggregate JSON. Callbacks must honor their contexts; the SDK cannot interrupt a callback that blocks forever. Diagnostic causes are not exposed through the health endpoint.

Registry.Handler returns 200 or 503. gRPC servers expose standard health; set service-specific statuses through Server.Health. Avoid coupling liveness to transient external dependencies. Restrict detailed readiness endpoints at the network boundary when dependency names are sensitive.

lifecycle.Manager registers named startup/stop hooks before Start. Stop is reverse ordered and joins errors, with a 30 s default timeout. Signal handlers require explicit NotifyContext. Hook callbacks must not call Manager methods recursively. Failed startup rolls back only successfully started hooks; a partially failed hook must clean up its own partial resources.
