module github.com/cetrictech/go-platform-kit/integration

go 1.26.0

toolchain go1.27.3

require (
	github.com/cetrictech/go-platform-kit v0.1.0
	github.com/cetrictech/go-platform-kit/messaging/kafka/franz v0.1.0
	github.com/cetrictech/go-platform-kit/messaging/kafka/sarama v0.1.0
	github.com/spiffe/go-spiffe/v2 v2.9.0
	google.golang.org/grpc v1.85.0
)

require (
	github.com/Microsoft/go-winio v0.6.2 // indirect
	github.com/go-jose/go-jose/v4 v4.1.5 // indirect
	github.com/golang-jwt/jwt/v5 v5.3.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/pgx/v5 v5.12.0 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	golang.org/x/sync v0.24.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260928230214-8a89bd6388cc // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
)

require (
	github.com/IBM/sarama v1.61.1 // indirect
	github.com/cetrictech/go-platform-kit/database/postgres v0.1.0
	github.com/cetrictech/go-platform-kit/grpcx v0.1.0
	github.com/cetrictech/go-platform-kit/security/jwt v0.1.0
	github.com/cetrictech/go-platform-kit/spiffe v0.1.0
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/eapache/go-resiliency v1.7.0 // indirect
	github.com/hashicorp/go-uuid v1.0.3 // indirect
	github.com/jcmturner/aescts/v2 v2.0.0 // indirect
	github.com/jcmturner/dnsutils/v2 v2.0.0 // indirect
	github.com/jcmturner/gofork v1.7.6 // indirect
	github.com/jcmturner/gokrb5/v8 v8.4.4 // indirect
	github.com/jcmturner/rpc/v2 v2.0.3 // indirect
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/pierrec/lz4/v4 v4.1.33 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20250401214520-65e299d6c5c9 // indirect
	github.com/twmb/franz-go v1.22.2 // indirect
	github.com/twmb/franz-go/pkg/kadm v1.19.0 // indirect
	github.com/twmb/franz-go/pkg/kmsg v1.15.0 // indirect
	github.com/xdg-go/scram v1.4.0 // indirect
	github.com/xdg-go/stringprep v1.0.4 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/cetrictech/go-platform-kit => ..

replace github.com/cetrictech/go-platform-kit/messaging/kafka/franz => ../messaging/kafka/franz

replace github.com/cetrictech/go-platform-kit/messaging/kafka/sarama => ../messaging/kafka/sarama

replace github.com/cetrictech/go-platform-kit/database/postgres => ../database/postgres

replace github.com/cetrictech/go-platform-kit/spiffe => ../spiffe

replace github.com/cetrictech/go-platform-kit/grpcx => ../grpcx

replace github.com/cetrictech/go-platform-kit/security/jwt => ../security/jwt
