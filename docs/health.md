# Health and lifecycle

Use separate Registry instances for liveness, readiness and startup. An empty registry is healthy, appropriate for process liveness. Register dependency Ping callbacks for readiness and optional startup checks. Checks run concurrently with per-check timeout contexts (default 2 s) and safe aggregate JSON. Callbacks must honor their contexts; the SDK cannot interrupt a callback that blocks forever. Diagnostic causes are not exposed through the health endpoint.

Registry.Handler returns 200 or 503. gRPC servers expose standard health; set service-specific statuses through Server.Health. Avoid coupling liveness to transient external dependencies. Restrict detailed readiness endpoints at the network boundary when dependency names are sensitive.

lifecycle.Manager registers named startup/stop hooks before Start. Stop is reverse ordered and joins errors, with a 30 s default timeout. Signal handlers require explicit NotifyContext. Hook callbacks must not call Manager methods recursively. Failed startup rolls back only successfully started hooks; a partially failed hook must clean up its own partial resources.

Each registered check permits only one active invocation. An invocation that ignores cancellation times out from the report, and subsequent runs return a safe busy reason until it exits. This bounds abandoned callback goroutines to one per check. `OnResult` receives private diagnostic errors; keep it prompt. Driver-independent Ping and Kafka administration helpers compose readiness checks.

`lifecycle.Cleanup` adapts Shutdown/CloseContext, gRPC graceful stop, Flush and Close contracts. `RegisterResource` owns a factory result and cleans up a partially returned resource on initialization failure, then closes successful resources in reverse registration order. Directly registered custom hooks still own cleanup of their own partial state.
