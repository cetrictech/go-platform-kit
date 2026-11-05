# Idempotency and leases

Store exposes Begin, Complete, Fail and Renew. A record contains fingerprint, random ownership token, processing/completed/failed state, expiration and HTTP status. Begin is atomic: matching completed requests return ErrCompleted; active ones ErrBusy; fingerprint differences ErrMismatch. Completion/renewal requires the current processing owner. Memory is bounded; Redis uses atomic scripts and server TTL expiration.

Expired leases can be acquired by a new owner while an old worker is still executing. Stop processing on lost ownership and use authoritative unique constraints/fencing for irreversible operations. Completed retention controls deduplication duration. Failure permits retry with the same fingerprint. Redis persistence/failover semantics constrain durability; there is no universal exactly-once promise.

HTTP middleware requires server-defined Scope, Store, positive Lease/Retention/RequestTimeout and MaxBodyBytes. RequestTimeout must be shorter than Lease. Scope should bind authorization/tenant identity and should never be derived solely from an untrusted header. Fingerprints include method, URI, content type and bounded body. The middleware persists only status/fingerprint/ownership metadata. Completed duplicates return 409 with Idempotency-Status rather than replaying sensitive response bodies. Supply OnError to monitor failures after response commitment. Handlers must respect cancellation. Renew manually for non-HTTP long operations.

A durable PostgreSQL idempotency store and encrypted replay-body storage are not implemented in this initial release; applications requiring those guarantees must add authoritative persistence before deployment.
