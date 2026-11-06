# Performance regression gate

Critical core paths have an automated, fail-closed comparison in `scripts/check-benchmarks.py`. The suite covers cached SQL scanning, JSON encoding, error construction, structured logging, Kafka handler processing, cursor encoding and response serialization. Real PostgreSQL/Redis/Kafka round-trip benchmarks remain separate: their server, network and container latency cannot serve as a deterministic SDK-overhead gate.

Collect repeated samples with:

```sh
./scripts/benchmark-run.sh /tmp/sdk-current-bench
python3 scripts/check-benchmarks.py \
  --input /tmp/sdk-current-bench/benchmarks.log \
  --metadata /tmp/sdk-current-bench/metadata.json \
  --baseline benchmarks/core-darwin-arm64-go1.27.3.json
```

The runner fixes GOMAXPROCS and benchmark CPU count to 2, disables ambient GOFLAGS, uses GOWORK=off, runs ten samples of each case for 200ms each, and records the actual CPU, OS, architecture, Go version, source commit and dirty-tree state. Run on an idle machine. Changing runtime, CPU, sampling duration or instrumentation makes existing evidence ineligible: comparisons fail rather than silently treating unrelated machines as equivalent. Release evidence must refer to a clean source tree at the exact release commit. Add `--release` to the comparison command to require clean-tree markers in both baseline and candidate evidence. A missing or dirty marker fails.

The committed Darwin/arm64 snapshot contains actual local samples collected during development. Its `source_dirty: true` marker is intentional: it is a reviewed-format local candidate and does not prove a clean release commit. Do not use it to authorize a release or compare Linux CI. The trusted runner accepts `SDK_BENCH_ROOT=/path/to/base-checkout` to collect a previous commit without using an older or absent benchmark script. CI compares base and candidate commits on the same runner, with identical environment metadata and benchmark parameters; maintainers review and adopt dedicated release-runner evidence before publication. Review thresholds, raw samples, source identity and runner eligibility when adopting or updating a release baseline.

Each package-qualified benchmark/subbenchmark identity is preserved, with Go's CPU-count suffix normalized. Missing cases or allocation metrics, malformed/nonfinite values, failed benchmark processes, added unreviewed cases, insufficient samples, weakened baseline policy and incompatible environment metadata all fail the gate. The baseline retains raw values; recomputing summaries detects edited or inconsistent medians.

Comparisons require at least seven samples and use repeated medians. Median absolute deviation may not exceed 15% of the median. Noisy samples fail and must be recollected; they do not waive the gate. Default allowed increases are 20% for ns/op and 10% for B/op. No increase in median allocations/op is allowed, including growth from zero. These bounds are engineering guardrails, not a claim of statistical significance or a throughput SLA. Investigate regressions with profiles and repeated controlled runs before changing policy.

Record a new candidate explicitly; normal comparison cannot overwrite a baseline:

```sh
python3 scripts/check-benchmarks.py --record \
  --input /tmp/sdk-current-bench/benchmarks.log \
  --metadata /tmp/sdk-current-bench/metadata.json \
  --candidate /tmp/sdk-candidate-baseline.json
```

`--candidate` must identify a new file. Commit a reviewed baseline only after checking its environment, source identity, dispersion and raw samples. A baseline update requires the same compatibility/release review as other changes; a red performance gate is not resolved by silently replacing numbers.

The gate's own negative tests run with `python3 -m unittest discover -s scripts -p 'test_check_benchmarks.py'`. They establish that regressions in each metric, missing evidence, environment mismatch, noisy samples, allocation growth and weakened/tampered baselines fail.

Wire behavior is guarded separately by reviewed versioned fixtures in `errors/testdata/classification-v0.json`, `response/testdata/wire-v0.json` and `grpcx/testdata/status-v0.json`. Tests exercise the actual APIs and reject code/category/status/field changes and private diagnostic exposure. Updating a fixture requires versioning and migration review; it is not an automatic approval of changed behavior.
