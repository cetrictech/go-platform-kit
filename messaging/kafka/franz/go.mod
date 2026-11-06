module github.com/cetrictech/go-platform-kit/messaging/kafka/franz

go 1.26.0

toolchain go1.27.3

require (
	github.com/cetrictech/go-platform-kit v0.1.0
	github.com/twmb/franz-go/pkg/kmsg v1.15.0
)

require github.com/twmb/franz-go/pkg/kadm v1.19.0

require (
	github.com/klauspost/compress v1.20.3 // indirect
	github.com/pierrec/lz4/v4 v4.1.33 // indirect
	github.com/twmb/franz-go v1.22.2
)

replace github.com/cetrictech/go-platform-kit => ../../..
