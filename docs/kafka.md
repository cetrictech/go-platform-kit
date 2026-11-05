# Kafka common contracts

Message, Producer, Consumer, Handler, Delivery, Topic and Admin are in the core messaging/kafka package. Select Franz or Sarama by importing its independent module. Publish and PublishBatch wait for configured broker acknowledgments, not just a local queue. A context cancellation after enqueue can leave delivery outcome unknown. Broker acknowledgment does not imply end-to-end exactly-once effects.

Consume calls the handler and commits only after success. A handler error stops consumption and leaves that message uncommitted. Handlers must be idempotent because crashes/rebalances/commit failures can redeliver messages. Do not share a consumer between concurrent Consume calls. Cancel and wait for Consume before Close.

WithFailure applies an explicit idempotency-aware retry.Policy, then optionally publishes a DLQ message with original location, attempt count and timestamp. It returns nil only after DLQ acknowledgment; a failure keeps the source uncommitted. DLQ publish and source commit are separate operations and duplicates remain possible. Original headers retain trace/correlation metadata. Error text is deliberately excluded to avoid leaking credentials. Retry policy must have MaxAttempts >= 1.

Admin provides topic create/delete/list and partition IDs. Group/offset inspection uses the Inspector extension. OAuth is available through advanced native configuration. Header maps cannot preserve duplicate header names. Offset reset, concurrency, native transactions and unsupported administration differ by adapter; see adapter documents and implementation-status.md.

Transactional outbox: build a Record and execute InsertSQL output in the same transaction as application state. SQLStore.Dispatch claims ordered rows using FOR UPDATE SKIP LOCKED, publishes with event-id, and commits delivery/retry/dead state. Its context must have a deadline. Apply docs/outbox-*.sql manually. Cleanup deletes completed rows before a cutoff. Monitor dead rows and decide replay policy explicitly. Holding row locks during publishing bounds concurrency but can hold a transaction during broker latency. It is at least once and requires consumer deduplication.

The Inspector contract adds DescribeTopic (leader/replica placement), ListGroups, DescribeGroup and Offsets. ConsumeAcknowledged requires the handler to call Ack and then return nil. Ack records commit intent; a successful broker commit is observed through Consume behavior, not the Ack call. PublishWithDelivery is available on each concrete producer for assigned broker metadata.

Both adapters expose OffsetReset (earliest/latest), Compression (none/gzip/snappy/lz4/zstd), ManualPartition, RetryMax, RetryBackoff and Acknowledgments (all/leader). All is the default. Leader acknowledgments require explicit DisableIdempotence=true. Nonzero tuning fields override driver defaults; zero retains those defaults. Workers bounds handler concurrency while preserving per-partition order.
