# Franz-go adapter

NewProducer accepts broker addresses, TLS, SASL PLAIN/SCRAM-256/SCRAM-512, client ID, linger, batch size and buffered record count. SASL requires TLS in the common configuration. NativeOptions provides explicit advanced franz-go options for partitioners, compression, OAuth, idempotence/transaction policy and instrumentation. Default franz-go idempotent production is preserved. Native options can change guarantees; review them as application security configuration.

PublishAsync uses the driver's bounded producer queue and calls a delivery callback with broker metadata. Callbacks must return promptly. Flush takes a context. Close shuts down the owned client; flush first with an application deadline. Native exposes the client.

NewConsumer requires Group and Topics. Consume disables auto commit and processes up to 100 records per poll with bounded per-partition concurrency, with rebalance blocking during a batch and synchronous record commits. Bound handler duration below broker poll/rebalance limits. Pause/Resume operate on topics. NativeOptions is an advanced escape hatch; overriding offset/commit options changes the documented common behavior. Native supports transactional production, but the SDK does not claim end-to-end exactly once.

Verified TLS is required by default. `AllowPlaintext: true` is an explicit opt-in for isolated development/test infrastructure; it does not disable validation of a supplied TLS configuration. A custom certificate verifier, including go-spiffe verification, is supported.

Workers defaults to 1 and accepts up to 256. A poll batch is grouped by partition and processed with bounded active workers; rebalances remain blocked until that batch drains. Callbacks must honor cancellation and finish within broker poll limits. Set ManualPartition=true to honor Message.Partition; otherwise the native key partitioner selects placement. PublishWithDelivery returns broker metadata. The common configuration supports compression, acknowledgment and retry controls without native imports.
