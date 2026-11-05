# go-platform-kit

A modular Go infrastructure SDK with native HTTP utilities, typed configuration, structured logging, application errors, SQL/Redis adapters, two Kafka adapters, secure gRPC, SPIFFE workload identities and OpenTelemetry.

**Status: initial development implementation; release qualification is incomplete.** Read [implementation status](docs/implementation-status.md) and [validation results](docs/validation.md) before adoption.

Requires Go 1.26 or later; development validation uses Go 1.27.3. The root module is `github.com/cetrictech/go-platform-kit`. Optional adapters are separate modules, so importing logging does not add database, Kafka or gRPC drivers.

```go
import (
    "context"
    "github.com/cetrictech/go-platform-kit/logger"
)

func run(ctx context.Context) {
    log := logger.New(logger.Config{JSON: true, Service: "my-service"})
    defer log.Close()
    log.InfoContext(ctx, "service started")
}
```

```sh
make test          # all modules, no infrastructure needed
make race
make services      # isolated real integration services
make integration
make services-down
```

See [module APIs](docs/modules.md), [architecture](docs/architecture.md), [configuration](docs/configuration.md), [HTTP](docs/http.md), [databases](docs/postgres.md), [Redis](docs/redis.md), [Kafka](docs/kafka.md), [gRPC](docs/grpc.md), [SPIFFE](docs/spiffe.md), [security](docs/security.md), and [versioning and installation](docs/versioning.md). No tags or releases have been created.
