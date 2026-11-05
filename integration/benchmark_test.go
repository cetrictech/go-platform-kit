//go:build integration

package integration

import (
	"context"
	"fmt"
	"github.com/cetrictech/go-platform-kit/database/postgres"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	"os"
	"testing"
	"time"
)

func BenchmarkPostgresQuery(b *testing.B) {
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		b.Skip("POSTGRES_URL unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c, err := postgres.New(ctx, postgres.Config{URL: url, AllowPlaintext: true})
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var value int64
		if err := c.QueryRow(ctx, "SELECT $1::bigint", int64(9007199254740993)).Scan(&value); err != nil || value != 9007199254740993 {
			b.Fatal(value, err)
		}
	}
}
func BenchmarkKafkaAcknowledgedPublish(b *testing.B) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		b.Skip("KAFKA_BROKER unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	cfg := f.Config{AllowPlaintext: true, Brokers: []string{broker}}
	admin, err := f.NewAdmin(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer admin.Close()
	topic := fmt.Sprintf("gpk-bench-%d", time.Now().UnixNano())
	if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
		b.Fatal(err)
	}
	defer admin.DeleteTopic(context.Background(), topic)
	p, err := f.NewProducer(cfg)
	if err != nil {
		b.Fatal(err)
	}
	defer p.Close()
	message := kafka.Message{Topic: topic, Value: make([]byte, 1024)}
	b.ReportAllocs()
	b.SetBytes(1024)
	b.ResetTimer()
	for range b.N {
		if err := p.Publish(ctx, message); err != nil {
			b.Fatal(err)
		}
	}
}
