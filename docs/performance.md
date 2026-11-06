# Performance

`make bench` runs local Go benchmarks with allocation reporting independently for every module. Core benchmarks cover logging, JSON serialization, error construction, cached SQL struct scanning, Kafka handler processing, signed cursor encoding and HTTP response serialization. Infrastructure round-trip benchmarks are separate because they include network, broker/server and container latency.

Use the [performance regression gate](performance-regressions.md) for controlled comparisons. Its runner collects ten 200ms samples per core case, fixes GOMAXPROCS to 2, and records CPU, OS, architecture, Go version, source commit and working-tree state. The gate compares repeated medians and allocation counts, checks dispersion, and rejects missing cases or ineligible environments. CI collects base and candidate commits on the same runner; the committed local Darwin snapshot is reference evidence, not a Linux CI baseline or release authorization.

```sh
./scripts/benchmark-run.sh /tmp/sdk-current-bench
python3 scripts/check-benchmarks.py \
  --input /tmp/sdk-current-bench/benchmarks.log \
  --metadata /tmp/sdk-current-bench/metadata.json \
  --baseline benchmarks/core-darwin-arm64-go1.27.3.json
```

Use an idle machine. Release evidence additionally requires `--release`, a clean source tree at the exact release commit, and a compatible reviewed baseline. Investigate ns/op, bytes/op or allocations/op regressions with CPU and allocation profiles before updating any baseline. A changed or noisy environment is a failed qualification check, not permission to substitute unrelated numbers.

The SQL scanner uses safe reflection with a concurrency-safe metadata cache; it handles exported embedded pointer structs and JSON fields. Allocation costs still depend on destination types and row schemas. Atomic JSON responses buffer up to the configured bound, while serialization may allocate in proportion to the value being encoded. Avoid large envelope payloads and use native streaming when appropriate. Kafka batching and connection pools require separate capacity tests against the intended deployment.

The original profiling pass on Apple M3 Pro/Go 1.27.3 identified repeated strings.Replacer construction in secret-field normalization. Removing it reduced the measured logging allocation count. That historical comparison is recorded in validation logs; current repeated evidence and source/environment metadata are in `benchmarks/core-darwin-arm64-go1.27.3.json`. These measurements are not throughput guarantees.

Real services are required for network benchmarks:

```sh
make services
REDIS_ADDRESS=127.0.0.1:16379 GOWORK=off go -C cache/redis test \
  -tags=integration -run '^$' -bench . -benchtime=100x -benchmem
KAFKA_BROKER=127.0.0.1:19092 GOWORK=off go -C messaging/kafka/franz test \
  -tags=integration -run '^$' -bench . -benchtime=100x -benchmem
KAFKA_BROKER=127.0.0.1:19092 GOWORK=off go -C messaging/kafka/sarama test \
  -tags=integration -run '^$' -bench . -benchtime=100x -benchmem
POSTGRES_URL='postgres://postgres:sdk-test-only@127.0.0.1:15432/sdk?sslmode=disable' \
KAFKA_BROKER=127.0.0.1:19092 GOWORK=off go -C integration test \
  -tags=integration -run '^$' -bench . -benchtime=100x -benchmem
make services-down
```

The core Kafka consumer benchmark measures handler-wrapper overhead. Real broker throughput, multi-consumer group load, sustained memory pressure and production capacity require deployment-specific load tests beyond that microbenchmark. Current network results are recorded in [validation/network-benchmarks.log](validation/network-benchmarks.log).
