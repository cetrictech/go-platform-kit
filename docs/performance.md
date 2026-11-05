# Performance

`make bench` runs reproducible local benchmarks with allocation reporting, independently for every module. For stable comparisons, record CPU, OS, Go version, GOMAXPROCS, commit and benchstat results; repeat with `-count=10` on an idle host. Current microbenchmarks cover JSON logging, serialization and signed cursor encoding. Infrastructure latency must be measured separately using provisioned services.

Struct scanning uses safe reflection and currently rebuilds a field map per row. This favors straightforward correctness over optimal allocation behavior; profile before adding schema caches. Atomic JSON responses buffer up to the configured output bound, while encoding itself may allocate proportional to its input. Avoid large envelope payloads and use native streaming when appropriate. Kafka batching and pool sizing should be load-tested against actual brokers/databases.

Not all requested database/Redis/Kafka network benchmarks are included yet; do not infer throughput guarantees from core microbenchmarks.

Additional microbenchmarks cover error construction, HTTP envelope encoding and shared SQL struct scanning. Tagged Redis and Kafka producer round-trip benchmarks require REDIS_ADDRESS / KAFKA_BROKER and measure service latency separately. Kafka consumer throughput and broad deployment load benchmarks remain qualification work.

A profiling pass on the Apple M3 Pro/Go 1.27.3 validation host found per-attribute construction of a strings.Replacer in secret-field normalization. Replacing that construction reduced the JSON logging microbenchmark from approximately 27,103 B/op and 22 allocations/op to 72 B/op and 6 allocations/op, with the repeated post-change runs around 1.4 microseconds/op. This is a local observation, not a throughput guarantee. Recorded core and Docker round-trip results are in validation/benchmarks.log and validation/network-benchmarks.log. Network results include Docker transport and broker/server work and must not be interpreted as SDK overhead alone.

Real-network follow-up benchmarks: `POSTGRES_URL='postgres://postgres:sdk-test-only@127.0.0.1:15432/sdk?sslmode=disable' KAFKA_BROKER=127.0.0.1:19092 GOWORK=off go -C integration test -tags=integration -run '^$' -bench . -benchtime=100x -benchmem`. These include local container/network latency and are distinct from root scanning/codec/consumer-processing overhead benchmarks. Adapter integration benchmark files also provide Redis and Kafka round-trip benchmarks with their corresponding environment variables.
