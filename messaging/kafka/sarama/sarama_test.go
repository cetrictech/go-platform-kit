package sarama

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

func TestConfig(t *testing.T) {
	for _, mechanism := range []string{"PLAIN", "SCRAM-SHA-256", "SCRAM-SHA-512"} {
		c, e := config(Config{Brokers: []string{"localhost:9092"}, TLS: &tls.Config{}, SASL: mechanism, Username: "test", Password: "test", Compression: "zstd", OffsetReset: "latest", ManualPartition: true})
		if e != nil || !c.Net.SASL.Enable {
			t.Fatal(c, e)
		}
		if mechanism != "PLAIN" {
			client := c.Net.SASL.SCRAMClientGeneratorFunc()
			if e = client.Begin("test", "test", ""); e != nil {
				t.Fatal(e)
			}
		}
	}
	if _, e := config(Config{Brokers: []string{"localhost:9092"}}); e == nil {
		t.Fatal("implicit plaintext")
	}
}
