package franz

import (
	"crypto/tls"
	"testing"
)

func TestInvalid(t *testing.T) {
	if _, e := NewProducer(Config{}); e == nil {
		t.Fatal("invalid config")
	}
	if _, e := NewConsumer(Config{}); e == nil {
		t.Fatal("invalid consumer")
	}
}

func TestOptions(t *testing.T) {
	if _, e := options(Config{Brokers: []string{"localhost:9092"}}); e == nil {
		t.Fatal("implicit plaintext")
	}
	for _, compression := range []string{"none", "gzip", "snappy", "lz4", "zstd"} {
		if _, e := options(Config{Brokers: []string{"localhost:9092"}, TLS: &tls.Config{}, Compression: compression, ManualPartition: true, OffsetReset: "latest"}); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := options(Config{Brokers: []string{"localhost:9092"}, TLS: &tls.Config{}, Acknowledgments: "leader"}); e == nil {
		t.Fatal("incompatible acknowledgment policy")
	}
}
