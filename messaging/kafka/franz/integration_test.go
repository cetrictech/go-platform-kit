//go:build integration

package franz

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"os"
	"testing"
	"time"
)

func TestIntegrationDelivery(t *testing.T) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		t.Skip("KAFKA_BROKER unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	topic := fmt.Sprintf("gpk-franz-%d", time.Now().UnixNano())
	cfg := Config{AllowPlaintext: true, Brokers: []string{broker}, Topics: []string{topic}, Group: topic}
	a, e := NewAdmin(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer a.Close()
	if e = a.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); e != nil {
		t.Fatal(e)
	}
	defer a.DeleteTopic(context.Background(), topic)
	description, err := a.DescribeTopic(ctx, topic)
	if err != nil || len(description.Partitions) != 1 {
		t.Fatal(description, err)
	}
	p, e := NewProducer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer p.Close()
	c, e := NewConsumer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	done := make(chan error, 1)
	received := make(chan struct{}, 1)
	sentinel := errors.New("stop without commit")
	go func() {
		done <- c.Consume(ctx, func(_ context.Context, m kafka.Message) error {
			if string(m.Value) != "hello" {
				return fmt.Errorf("unexpected payload")
			}
			received <- struct{}{}
			return sentinel
		})
	}()
	time.Sleep(2 * time.Second)
	if e = p.Publish(ctx, kafka.Message{Topic: topic, Key: []byte("key"), Value: []byte("hello")}); e != nil {
		t.Fatal(e)
	}
	if e = p.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	select {
	case <-received:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	select {
	case e = <-done:
		if !errors.Is(e, sentinel) {
			t.Fatal(e)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	groups, err := a.ListGroups(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range groups {
		if g.Name == topic {
			found = true
		}
	}
	if !found {
		t.Fatal("consumer group missing")
	}
	if _, err = a.DescribeGroup(ctx, topic); err != nil {
		t.Fatal(err)
	}
	if _, err = a.Offsets(ctx, topic); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkPublishRoundTrip(b *testing.B) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		b.Skip("KAFKA_BROKER unset")
	}
	cfg := Config{AllowPlaintext: true, Brokers: []string{broker}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	a, e := NewAdmin(cfg)
	if e != nil {
		b.Fatal(e)
	}
	defer a.Close()
	topic := fmt.Sprintf("gpk-benchmark-%d", time.Now().UnixNano())
	if e = a.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); e != nil {
		b.Fatal(e)
	}
	defer a.DeleteTopic(context.Background(), topic)
	p, e := NewProducer(cfg)
	if e != nil {
		b.Fatal(e)
	}
	defer p.Close()
	b.ResetTimer()
	for b.Loop() {
		if e = p.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("payload")}); e != nil {
			b.Fatal(e)
		}
	}
}
