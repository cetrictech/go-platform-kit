# Exact module dependencies

Resolved independently with `GOWORK=off go list -mod=readonly -m all`, Go 1.27.3. Each go.mod pins selected versions; go.sum records integrity hashes. Stable releases were checked during implementation. Integration is a test-only module.

## Core

```text
github.com/cetrictech/go-platform-kit
github.com/google/uuid v1.6.0
golang.org/x/crypto v0.58.0
golang.org/x/net v0.60.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
gopkg.in/natefinch/lumberjack.v2 v2.2.1
```

## database/postgres

```text
github.com/cetrictech/go-platform-kit/database/postgres
github.com/cetrictech/go-platform-kit v0.1.0 => ../..
github.com/davecgh/go-spew v1.1.1
github.com/google/uuid v1.6.0
github.com/jackc/pgpassfile v1.0.0
github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761
github.com/jackc/pgx/v5 v5.12.0
github.com/jackc/puddle/v2 v2.2.3
github.com/kr/pretty v0.3.0
github.com/pmezard/go-difflib v1.0.0
github.com/stretchr/objx v0.5.3
github.com/stretchr/testify v1.12.1
go.yaml.in/yaml/v3 v3.0.5
golang.org/x/crypto v0.58.0
golang.org/x/mod v0.41.0
golang.org/x/net v0.60.0
golang.org/x/sync v0.24.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
golang.org/x/tools v0.51.0
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
gopkg.in/natefinch/lumberjack.v2 v2.2.1
gopkg.in/yaml.v3 v3.0.1
```

## database/mysql

```text
github.com/cetrictech/go-platform-kit/database/mysql
filippo.io/edwards25519 v1.2.0
github.com/cetrictech/go-platform-kit v0.1.0 => ../..
github.com/go-sql-driver/mysql v1.10.1
github.com/google/uuid v1.6.0
golang.org/x/crypto v0.58.0
gopkg.in/natefinch/lumberjack.v2 v2.2.1
```

## cache/redis

```text
github.com/cetrictech/go-platform-kit/cache/redis
github.com/bsm/ginkgo/v2 v2.12.0
github.com/bsm/gomega v1.27.10
github.com/cespare/xxhash/v2 v2.3.0
github.com/cetrictech/go-platform-kit v0.1.0 => ../..
github.com/google/uuid v1.6.0
github.com/klauspost/cpuid/v2 v2.4.0
github.com/redis/go-redis/v9 v9.23.0
github.com/stretchr/testify v1.12.1
github.com/zeebo/xxh3 v1.1.0
go.uber.org/atomic v1.12.0
go.yaml.in/yaml/v3 v3.0.5
golang.org/x/crypto v0.58.0
golang.org/x/net v0.60.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
gopkg.in/natefinch/lumberjack.v2 v2.2.1
```

## messaging/kafka/franz

```text
github.com/cetrictech/go-platform-kit/messaging/kafka/franz
github.com/cetrictech/go-platform-kit v0.1.0 => ../../..
github.com/google/uuid v1.6.0
github.com/klauspost/compress v1.20.3
github.com/pierrec/lz4/v4 v4.1.33
github.com/twmb/franz-go v1.22.2
github.com/twmb/franz-go/pkg/kadm v1.19.0
github.com/twmb/franz-go/pkg/kmsg v1.15.0
golang.org/x/crypto v0.58.0
gopkg.in/natefinch/lumberjack.v2 v2.2.1
```

## messaging/kafka/sarama

```text
github.com/cetrictech/go-platform-kit/messaging/kafka/sarama
github.com/IBM/sarama v1.61.1
github.com/cetrictech/go-platform-kit v0.1.0 => ../../..
github.com/davecgh/go-spew v1.1.1
github.com/eapache/go-resiliency v1.7.0
github.com/google/uuid v1.6.0
github.com/gorilla/securecookie v1.1.1
github.com/gorilla/sessions v1.2.1
github.com/hashicorp/go-uuid v1.0.3
github.com/jcmturner/aescts/v2 v2.0.0
github.com/jcmturner/dnsutils/v2 v2.0.0
github.com/jcmturner/gofork v1.7.6
github.com/jcmturner/goidentity/v6 v6.0.1
github.com/jcmturner/gokrb5/v8 v8.4.4
github.com/jcmturner/rpc/v2 v2.0.3
github.com/klauspost/compress v1.20.3
github.com/pierrec/lz4/v4 v4.1.33
github.com/pmezard/go-difflib v1.0.0
github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
github.com/stretchr/objx v0.5.0
github.com/stretchr/testify v1.12.1
github.com/xdg-go/scram v1.4.0
github.com/xdg-go/stringprep v1.0.4
github.com/yuin/goldmark v1.4.13
go.uber.org/goleak v1.3.0
go.yaml.in/yaml/v3 v3.0.5
golang.org/x/crypto v0.58.0
golang.org/x/mod v0.41.0
golang.org/x/net v0.62.0
golang.org/x/sync v0.24.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
golang.org/x/tools v0.51.0
golang.org/x/xerrors v0.0.0-20190717185122-a985d3407aa7
gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405
gopkg.in/natefinch/lumberjack.v2 v2.2.1
gopkg.in/yaml.v2 v2.2.2
gopkg.in/yaml.v3 v3.0.1
```

## grpcx

```text
github.com/cetrictech/go-platform-kit/grpcx
cel.dev/expr v0.25.3
cloud.google.com/go/compute/metadata v0.9.0
github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.35.0
github.com/cespare/xxhash/v2 v2.3.0
github.com/cetrictech/go-platform-kit v0.1.0 => ..
github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2
github.com/envoyproxy/go-control-plane v0.14.0
github.com/envoyproxy/go-control-plane/envoy v1.39.1-0.20260819172001-e6e3fd93e4be
github.com/envoyproxy/go-control-plane/ratelimit v0.1.0
github.com/envoyproxy/protoc-gen-validate v1.3.3
github.com/go-jose/go-jose/v4 v4.1.4
github.com/go-logr/logr v1.4.4
github.com/go-logr/stdr v1.2.2
github.com/golang/glog v1.2.5
github.com/golang/protobuf v1.5.4
github.com/google/go-cmp v0.7.0
github.com/google/uuid v1.6.0
github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
github.com/spiffe/go-spiffe/v2 v2.8.1
go.opentelemetry.io/auto/sdk v1.2.1
go.opentelemetry.io/contrib/detectors/gcp v1.45.0
go.opentelemetry.io/otel v1.45.0
go.opentelemetry.io/otel/metric v1.45.0
go.opentelemetry.io/otel/sdk v1.45.0
go.opentelemetry.io/otel/sdk/metric v1.45.0
go.opentelemetry.io/otel/trace v1.45.0
golang.org/x/crypto v0.58.0
golang.org/x/mod v0.41.0
golang.org/x/net v0.62.0
golang.org/x/oauth2 v0.36.0
golang.org/x/sync v0.24.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
golang.org/x/tools v0.51.0
gonum.org/v1/gonum v0.17.0
google.golang.org/genproto/googleapis/api v0.0.0-20260817212433-ac3dfec99bb1
google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc
google.golang.org/grpc v1.85.0
google.golang.org/protobuf v1.36.12
gopkg.in/natefinch/lumberjack.v2 v2.2.1
```

## spiffe

```text
github.com/cetrictech/go-platform-kit/spiffe
cel.dev/expr v0.25.3
cloud.google.com/go/compute/metadata v0.9.0
github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.35.0
github.com/Microsoft/go-winio v0.6.2
github.com/cespare/xxhash/v2 v2.3.0
github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2
github.com/envoyproxy/go-control-plane v0.14.0
github.com/envoyproxy/go-control-plane/envoy v1.39.1-0.20260819172001-e6e3fd93e4be
github.com/envoyproxy/go-control-plane/ratelimit v0.1.0
github.com/envoyproxy/protoc-gen-validate v1.3.3
github.com/go-jose/go-jose/v4 v4.1.5
github.com/go-logr/logr v1.4.4
github.com/go-logr/stdr v1.2.2
github.com/golang/glog v1.2.5
github.com/golang/protobuf v1.5.4
github.com/google/go-cmp v0.7.0
github.com/google/uuid v1.6.0
github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
github.com/sirupsen/logrus v1.9.3
github.com/spiffe/go-spiffe/v2 v2.9.0
github.com/stretchr/testify v1.12.1
go.opentelemetry.io/auto/sdk v1.2.1
go.opentelemetry.io/contrib/detectors/gcp v1.45.0
go.opentelemetry.io/otel v1.45.0
go.opentelemetry.io/otel/metric v1.45.0
go.opentelemetry.io/otel/sdk v1.45.0
go.opentelemetry.io/otel/sdk/metric v1.45.0
go.opentelemetry.io/otel/trace v1.45.0
go.yaml.in/yaml/v3 v3.0.5
golang.org/x/crypto v0.58.0
golang.org/x/mod v0.41.0
golang.org/x/net v0.62.0
golang.org/x/oauth2 v0.36.0
golang.org/x/sync v0.24.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
golang.org/x/tools v0.51.0
gonum.org/v1/gonum v0.17.0
google.golang.org/genproto/googleapis/api v0.0.0-20260817212433-ac3dfec99bb1
google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc
google.golang.org/grpc v1.85.0
google.golang.org/grpc/examples v0.0.0-20250407062114-b368379ef8f6
google.golang.org/protobuf v1.36.12
```

## observability

```text
github.com/cetrictech/go-platform-kit/observability
cel.dev/expr v0.25.3
cloud.google.com/go/compute/metadata v0.9.0
github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.35.0
github.com/apapsch/go-jsonmerge/v2 v2.0.0
github.com/cenkalti/backoff/v5 v5.0.3
github.com/cespare/xxhash/v2 v2.3.0
github.com/cetrictech/go-platform-kit v0.1.0 => ..
github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2
github.com/davecgh/go-spew v1.1.1
github.com/envoyproxy/go-control-plane v0.14.0
github.com/envoyproxy/go-control-plane/envoy v1.39.1-0.20260819172001-e6e3fd93e4be
github.com/envoyproxy/go-control-plane/ratelimit v0.1.0
github.com/envoyproxy/protoc-gen-validate v1.3.3
github.com/felixge/httpsnoop v1.1.0
github.com/go-jose/go-jose/v4 v4.1.4
github.com/go-logr/logr v1.4.4
github.com/go-logr/stdr v1.2.2
github.com/go-openapi/analysis v0.25.5
github.com/go-openapi/errors v0.22.8
github.com/go-openapi/jsonpointer v1.0.0
github.com/go-openapi/jsonreference v1.0.0
github.com/go-openapi/loads v0.25.0
github.com/go-openapi/runtime v0.33.0
github.com/go-openapi/runtime/server-middleware v0.30.0
github.com/go-openapi/spec v0.22.9
github.com/go-openapi/strfmt v0.27.0
github.com/go-openapi/swag v0.28.0
github.com/go-openapi/swag/cmdutils v0.28.0
github.com/go-openapi/swag/conv v0.28.0
github.com/go-openapi/swag/fileutils v0.28.0
github.com/go-openapi/swag/jsonutils v0.28.0
github.com/go-openapi/swag/loading v0.28.0
github.com/go-openapi/swag/mangling v0.28.0
github.com/go-openapi/swag/netutils v0.28.0
github.com/go-openapi/swag/pools v0.28.0
github.com/go-openapi/swag/stringutils v0.28.0
github.com/go-openapi/swag/typeutils v0.28.0
github.com/go-openapi/swag/yamlutils v0.28.0
github.com/go-openapi/validate v0.26.1
github.com/go-viper/mapstructure/v2 v2.5.0
github.com/golang/glog v1.2.5
github.com/golang/protobuf v1.5.4
github.com/google/go-cmp v0.7.0
github.com/google/uuid v1.6.0
github.com/grpc-ecosystem/grpc-gateway/v2 v2.30.0
github.com/kr/pretty v0.3.1
github.com/oapi-codegen/runtime v1.6.0
github.com/oklog/ulid/v2 v2.1.1
github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
github.com/pmezard/go-difflib v1.0.0
github.com/rogpeppe/fastuuid v1.2.0
github.com/rogpeppe/go-internal v1.14.1
github.com/spiffe/go-spiffe/v2 v2.8.1
github.com/stretchr/testify v1.12.1
go.opentelemetry.io/auto/sdk v1.2.1
go.opentelemetry.io/contrib/detectors/gcp v1.45.0
go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.70.0
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.70.0
go.opentelemetry.io/otel v1.47.0
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.47.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.47.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.47.0
go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.45.0
go.opentelemetry.io/otel/log v1.47.0
go.opentelemetry.io/otel/metric v1.47.0
go.opentelemetry.io/otel/metric/x v0.69.0
go.opentelemetry.io/otel/sdk v1.47.0
go.opentelemetry.io/otel/sdk/metric v1.47.0
go.opentelemetry.io/otel/trace v1.47.0
go.opentelemetry.io/proto/otlp v1.11.0
go.uber.org/goleak v1.3.0
go.yaml.in/yaml/v3 v3.0.5
golang.org/x/crypto v0.58.0
golang.org/x/mod v0.41.0
golang.org/x/net v0.62.0
golang.org/x/oauth2 v0.36.0
golang.org/x/sync v0.24.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
golang.org/x/tools v0.51.0
gonum.org/v1/gonum v0.17.0
google.golang.org/genproto/googleapis/api v0.0.0-20260928230214-8a89bd6388cc
google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc
google.golang.org/grpc v1.85.0
google.golang.org/protobuf v1.36.12
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
gopkg.in/natefinch/lumberjack.v2 v2.2.1
gopkg.in/yaml.v3 v3.0.1
```

## security/jwt

```text
github.com/cetrictech/go-platform-kit/security/jwt
github.com/golang-jwt/jwt/v5 v5.3.1
```

## integration

```text
github.com/cetrictech/go-platform-kit/integration
cel.dev/expr v0.25.3
cloud.google.com/go/compute/metadata v0.9.0
github.com/GoogleCloudPlatform/opentelemetry-operations-go/detectors/gcp v1.35.0
github.com/IBM/sarama v1.61.1
github.com/Microsoft/go-winio v0.6.2
github.com/cespare/xxhash/v2 v2.3.0
github.com/cetrictech/go-platform-kit v0.1.0 => ..
github.com/cetrictech/go-platform-kit/database/postgres v0.1.0 => ../database/postgres
github.com/cetrictech/go-platform-kit/grpcx v0.1.0 => ../grpcx
github.com/cetrictech/go-platform-kit/messaging/kafka/franz v0.1.0 => ../messaging/kafka/franz
github.com/cetrictech/go-platform-kit/messaging/kafka/sarama v0.1.0 => ../messaging/kafka/sarama
github.com/cetrictech/go-platform-kit/security/jwt v0.1.0 => ../security/jwt
github.com/cetrictech/go-platform-kit/spiffe v0.1.0 => ../spiffe
github.com/cncf/xds/go v0.0.0-20260202195803-dba9d589def2
github.com/davecgh/go-spew v1.1.1
github.com/eapache/go-resiliency v1.7.0
github.com/envoyproxy/go-control-plane v0.14.0
github.com/envoyproxy/go-control-plane/envoy v1.39.1-0.20260819172001-e6e3fd93e4be
github.com/envoyproxy/go-control-plane/ratelimit v0.1.0
github.com/envoyproxy/protoc-gen-validate v1.3.3
github.com/go-jose/go-jose/v4 v4.1.5
github.com/go-logr/logr v1.4.4
github.com/go-logr/stdr v1.2.2
github.com/golang-jwt/jwt/v5 v5.3.1
github.com/golang/glog v1.2.5
github.com/golang/protobuf v1.5.4
github.com/google/go-cmp v0.7.0
github.com/google/uuid v1.6.0
github.com/gorilla/securecookie v1.1.1
github.com/gorilla/sessions v1.2.1
github.com/hashicorp/go-uuid v1.0.3
github.com/jackc/pgpassfile v1.0.0
github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761
github.com/jackc/pgx/v5 v5.12.0
github.com/jackc/puddle/v2 v2.2.3
github.com/jcmturner/aescts/v2 v2.0.0
github.com/jcmturner/dnsutils/v2 v2.0.0
github.com/jcmturner/gofork v1.7.6
github.com/jcmturner/goidentity/v6 v6.0.1
github.com/jcmturner/gokrb5/v8 v8.4.4
github.com/jcmturner/rpc/v2 v2.0.3
github.com/klauspost/compress v1.20.3
github.com/kr/pretty v0.3.0
github.com/pierrec/lz4/v4 v4.1.33
github.com/planetscale/vtprotobuf v0.6.1-0.20240319094008-0393e58bdf10
github.com/pmezard/go-difflib v1.0.0
github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9
github.com/sirupsen/logrus v1.9.3
github.com/spiffe/go-spiffe/v2 v2.9.0
github.com/stretchr/objx v0.5.0
github.com/stretchr/testify v1.12.1
github.com/twmb/franz-go v1.22.2
github.com/twmb/franz-go/pkg/kadm v1.19.0
github.com/twmb/franz-go/pkg/kmsg v1.15.0
github.com/xdg-go/scram v1.4.0
github.com/xdg-go/stringprep v1.0.4
github.com/yuin/goldmark v1.4.13
go.opentelemetry.io/auto/sdk v1.2.1
go.opentelemetry.io/contrib/detectors/gcp v1.45.0
go.opentelemetry.io/otel v1.45.0
go.opentelemetry.io/otel/metric v1.45.0
go.opentelemetry.io/otel/sdk v1.45.0
go.opentelemetry.io/otel/sdk/metric v1.45.0
go.opentelemetry.io/otel/trace v1.45.0
go.uber.org/goleak v1.3.0
go.yaml.in/yaml/v3 v3.0.5
golang.org/x/crypto v0.58.0
golang.org/x/mod v0.41.0
golang.org/x/net v0.62.0
golang.org/x/oauth2 v0.36.0
golang.org/x/sync v0.24.0
golang.org/x/sys v0.49.0
golang.org/x/term v0.47.0
golang.org/x/text v0.43.0
golang.org/x/tools v0.51.0
golang.org/x/xerrors v0.0.0-20190717185122-a985d3407aa7
gonum.org/v1/gonum v0.17.0
google.golang.org/genproto/googleapis/api v0.0.0-20260817212433-ac3dfec99bb1
google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc
google.golang.org/grpc v1.85.0
google.golang.org/grpc/examples v0.0.0-20250407062114-b368379ef8f6
google.golang.org/protobuf v1.36.12
gopkg.in/check.v1 v1.0.0-20201130134442-10cb98267c6c
gopkg.in/natefinch/lumberjack.v2 v2.2.1
gopkg.in/yaml.v2 v2.2.2
gopkg.in/yaml.v3 v3.0.1
```
