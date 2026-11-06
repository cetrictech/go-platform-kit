# Versioning and compatibility

The SDK is unreleased. `release/modules.json` records each module's proposed version and its declared core compatibility. The initial candidates are v0.1.0; `integration/` is private test infrastructure and is not releasable. No module is declared v1-stable. Read [public API contracts](public-api-contracts.md), [support policy](support-policy.md), [deprecations](deprecations.md) and [the compatibility matrix](compatibility-matrix.md) before changing a contract.

## Independent module versions and tags

The root uses tags such as `v0.1.0`. Nested modules use directory-prefixed tags, for example `database/postgres/v0.1.0`, `database/mysql/v0.1.0`, `cache/redis/v0.1.0`, `messaging/kafka/franz/v0.1.0`, `messaging/kafka/sarama/v0.1.0`, `grpcx/v0.1.0`, `spiffe/v0.1.0`, `observability/v0.1.0`, `security/jwt/v0.1.0` and `storage/s3/v0.1.0`. A root tag releases no nested module. An adapter release does not require unrelated adapters to receive the same version.

Every release must have a canonical semantic version, immutable module-specific tag and matching go.mod path. A patch preserves public source and behavioral contracts; a minor adds backward-compatible behavior; a major permits a reviewed breaking change. During v0, breaking changes require a new minor plus migration/deprecation evidence, never a patch. Increase even the patch version for a published fix; never reuse, move or rewrite an existing release tag. Prerelease candidates are explicitly identified and do not satisfy the stable-consumer requirement merely by compiling.

For v0 and v1, import paths have no `/v0` or `/v1` suffix. A stable v2+ breaking release requires Go semantic import versioning: the module declaration and imports end in `/v2` (then `/v3`, etc.), and the version major matches. For example, a future PostgreSQL adapter v2 declares `github.com/cetrictech/go-platform-kit/database/postgres/v2` and uses tag `database/postgres/v2.0.0` when released from its existing module directory. A future core v2 declares `github.com/cetrictech/go-platform-kit/v2` and uses `v2.0.0`. If modules move to a major-version subdirectory, the checker/release manifest must reflect Go's directory/tag rules. An independently versioned adapter adopting a new core major must review exported type identities and source compatibility; core and adapter major numbers need not match automatically.

## Compatibility checks

`scripts/check-api.sh` compares each releasable module against its latest prior stable module-specific Git tag with a pinned Go API comparison tool. CI must fetch full tag history. Missing history, invalid tags/paths or comparator errors fail the check; a failed tool invocation is not an empty diff. Since no release tags exist yet, the explicit immutable `bootstrap_ref` in the manifest provides a first-development comparison baseline. That fallback is reported and is not evidence of previous-release compatibility.

Source API comparison does not detect all error, JSON, default, cancellation, ownership, allocation or delivery changes. The release gate also requires wire/error contract tests, independent module and clean-consumer checks, race/static/security checks, benchmark regression checks and real-infrastructure fault tests. Each public API must have its concurrency, cancellation and ownership rules covered by [the contract](public-api-contracts.md). Unsupported driver exposure requires explicit review rather than silently broadening a core interface.

Development replace directives support repository work; consumers do not inherit them. `scripts/check-independent.sh` copies and checks modules outside the workspace with `GOWORK=off`, including independent import/build/test resolution. Before publishing adapters, their declared core version must exist in the public/private module source used by actual consumers. The core requirement in each module must agree with the manifest/matrix; `spiffe` and `security/jwt` intentionally have no core dependency.

## Release admission

Run `scripts/release-check.sh` on the exact proposed revision. Failed tests, API/wire checks, security checks, missing evidence, independent-module checks or performance gates stop the candidate. Preserve raw logs, tool versions, dependency graph, fixture/server versions, benchmark comparison, API baseline, migration records and reviewed exceptions in release artifacts. A skipped infrastructure test is not a passing release gate.

Any v1-or-later candidate additionally requires the reviewed real-project evidence in `release/consumers.json` described in [support policy](support-policy.md). The initial empty evidence file deliberately prevents v1 admission. Repository examples and import smoke tests do not replace independent consumer compatibility and integration results.

A successful local gate neither tags nor publishes. Publish only after maintainer authorization and successful required CI for that exact commit; configure hosting branch/tag protection so the release identity cannot bypass required checks. Release core first when adapters require a new core version, then independently release the affected adapters. Verify downloads and import graphs from real consuming projects after staging the release. No release or tag is created by implementing these safeguards.
