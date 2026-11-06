# Support policy

No SDK release series is published yet. The first candidate is v0.1.0; no module is designated v1-stable or production-certified. The support commitments below apply when releases are published and are checked by the release process.

## Go and SDK versions

The minimum supported language/toolchain line is Go 1.26; Go 1.26.x and 1.27.x form the current supported CI matrix. Validation uses patched Go 1.26.10 and 1.27.3. Use the latest security patch on a supported line. The go directive in every releasable module is 1.26.0; toolchain suggestions do not silently change that minimum. New language/API usage must pass both matrix entries with `GOTOOLCHAIN=local`.

Maintain the current stable Go line and its immediately preceding line, subject to each line's upstream security support. When a new stable line ships, add it to CI and run all gates before changing this document and manifest; do not drop a supported line in a patch release. Raising the minimum Go version is a documented minor change during v0 and a major-version decision for stable modules. An unsupported Go version may work, but is not a supported compatibility target.

For v0, support the latest minor series; backport critical security fixes to the immediately preceding minor for at least 90 days after its successor when a safe backport exists. Public source and wire incompatibilities still require a new minor and migration notes. After v1, support the latest two minor series within each actively maintained major, and keep the previous major on critical-security support for at least 12 months after a replacement major. Record explicit end-of-support dates in release notes before retiring a series. No unpublished series receives a fabricated support/end-of-support date.

## Infrastructure and driver support

The initial exercised lines are PostgreSQL 18, MySQL 8.4, Redis 8 and Redpanda v25.3.1, plus real local gRPC, HTTPS JWKS/OTLP and protocol-compatible Workload API endpoints. [The matrix](compatibility-matrix.md) links the SDK/native versions and documented limits. Require vendor-supported security patches in production. Floating fixture image lines provide convenient local testing, while release evidence must capture the actual resolved image/digest or server version; a tag name alone is not immutable evidence.

Claims for another server major, Apache Kafka version, Redis-compatible service, cloud proxy, collector or SPIRE deployment require real tests and an explicit matrix entry. Driver wire compatibility and a mock test are insufficient. Credential rotation, TLS verification and failure/recovery tests are mandatory for the adapters that expose them. Production capacity, HA topology and application authorization remain consuming-project qualifications.

## v1 admission and consumer evidence

`scripts/release-check.sh` blocks any v1-or-later candidate unless `release/consumers.json` contains reviewed passing evidence from at least two distinct external consuming projects, dated within 90 days and covering both supported Go lines. Repository examples, generated import smoke tests and the SDK integration module do not count as independent consumers. At least one consumer must exercise the module through real infrastructure when that module is an infrastructure adapter. All modules selected for the stable release must appear with their exact candidate versions.

The evidence file uses `schema_version: 1` and a `consumers` array. Each entry supplies `project_url`, immutable 40-hex `revision`, RFC3339 `tested_at`, exact `go_version` such as `go1.27.3`, a `modules` map from full import path to tested version, `checks` containing `compatibility: true` and `integration: true`, an HTTPS `artifact_url` linking reproducible CI output, and a nonempty maintainer `reviewer`. Reviewers must verify that referenced artifacts establish the claimed results; JSON booleans are attestations, not independently verified test runs. The initial empty array deliberately fails the v1 gate.

Stable admission also requires an API/wire baseline, public ownership/concurrency/cancellation documentation, deprecation/migration review, passing real-service fault and resource tests, race/security/performance checks, and independent module validation. A passing script does not publish anything or replace maintainer review. No v1 tag should be created or pushed before the release gate and external consumer review pass.
