# Versioning and compatibility

Proposed initial development version: v0.1.0. No release is published automatically. The code is pre-1.0 and still requires qualification; do not label it production ready solely from unit results.

The root uses tags such as v0.1.0. Each nested module requires its own directory-prefixed tag: database/postgres/v0.1.0, database/mysql/v0.1.0, cache/redis/v0.1.0, messaging/kafka/franz/v0.1.0, messaging/kafka/sarama/v0.1.0, grpcx/v0.1.0, spiffe/v0.1.0, observability/v0.1.0. A root tag does not release nested modules.

| Core | Adapter versions | Minimum Go | Validation toolchain |
| --- | --- | --- | --- |
| proposed v0.1.0 | proposed v0.1.0 | 1.26.0 | 1.27.3 |

Development uses local replace directives; downstream users ignore them and resolve the declared core version from its root tag. Release core before adapter tags. Require the matching core minor release for this initial series. Before releasing, independently tidy/test each module with GOWORK=off, publish core, then verify adapters from a clean consumer without local replacements. See dependencies.md for exact selected versions.

Pre-1.0 breaking changes require minor increments and migration notes. After 1.0, public breaking API changes require the next major module path. Deprecate for at least two minor releases before removal when practical. Review stable dependency updates monthly and security advisories promptly. Never select prereleases implicitly. Re-run race/integration/vulnerability/module-isolation checks on dependency updates.

After explicit release authorization and successful validation:

```sh
git tag v0.1.0
for m in database/postgres database/mysql cache/redis messaging/kafka/franz messaging/kafka/sarama grpcx spiffe observability; do
  git tag "$m/v0.1.0"
done
# Review tags before explicitly pushing them.
```

Consume after tags have been published:

```sh
go get github.com/cetrictech/go-platform-kit@v0.1.0
go get github.com/cetrictech/go-platform-kit/database/postgres@v0.1.0
```
