# Redis

New accepts Addresses, MasterName for Sentinel, authentication, TLS, pool/timeouts and Serializer. One address without MasterName selects standalone; multiple addresses select cluster through go-redis UniversalClient. Ping runs before return. Close releases the client. Nil values return ErrNotFound. Observer instruments connection/command/pipeline operations with fixed operation names.

Core strings, JSON, hashes, lists, sets, sorted sets, streams and pub/sub methods use an explicit Prefix. Multi-key operations prefix every key. Redis Cluster scripts require keys in one hash slot; put a developer-controlled `{tag}` in logical keys when needed. Stream reads should use bounded block durations during shutdown. Acknowledge only after successful processing. XAutoClaim recovers pending entries; operators choose the minimum idle time.

Subscribe waits for acknowledgment, uses go-redis reconnect behavior, and closes on cancellation or handler failure. Callbacks must return promptly. Pub/sub is transient and does not guarantee delivery across reconnects.

Pipeline/Transaction/Watch are advanced native callbacks: callback keys are not automatically prefixed. Native exposes the underlying client. Eval and EvalSHA prefix supplied keys. CacheAside has no single-flight guarantee and may run loaders concurrently.

Acquire uses SET NX with a cryptographic owner token. Renew and Release are Lua compare-and-update operations. Expiration can permit a new owner while an old process continues working: these locks provide no distributed fencing. Use database fencing/unique constraints for irreversible state changes. Release/renew errors must be handled. Limiter uses an atomic fixed window; IdempotencyStore uses atomic fingerprint/ownership transitions and Redis key expiration. Neither makes Redis durable enough for every financial workflow.

Verified TLS is required by default. `AllowPlaintext: true` is an explicit opt-in for isolated development/test infrastructure; it does not disable validation of a supplied TLS configuration. A custom certificate verifier, including go-spiffe verification, is supported.

ConsumeGroup reads an existing stream group with bounded batches and block duration (defaults: 100 entries, one second), invokes handlers serially, and acknowledges only success. Handler errors/panics leave messages pending for explicit retry/reclaim. Count cannot exceed 1000. Cancellation drains no additional entries after the current callback finishes.
