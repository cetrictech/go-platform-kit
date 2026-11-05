//go:build integration

package integration

import (
	"context"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestKafkaPartitionWorkersAndDraining(t *testing.T) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		t.Skip("KAFKA_BROKER unset")
	}
	for _, adapter := range []string{"franz", "sarama"} {
		t.Run(adapter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
			defer cancel()
			topic := fmt.Sprintf("gpk-workers-%s-%d", adapter, time.Now().UnixNano())
			cfg := f.Config{AllowPlaintext: true, Brokers: []string{broker}, Topics: []string{topic}, Group: topic, Workers: 2, ManualPartition: true}
			admin, err := f.NewAdmin(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 3, ReplicationFactor: 1}); err != nil {
				t.Fatal(err)
			}
			defer admin.DeleteTopic(context.Background(), topic)
			p, err := f.NewProducer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			for partition := range int32(3) {
				for i := range 3 {
					if err := p.Publish(ctx, kafka.Message{Topic: topic, Partition: partition, Value: []byte(fmt.Sprint(i))}); err != nil {
						t.Fatal(err)
					}
				}
			}
			var c kafka.Consumer
			if adapter == "franz" {
				c, err = f.NewConsumer(cfg)
			} else {
				c, err = s.NewConsumer(s.Config{AllowPlaintext: true, Brokers: cfg.Brokers, Topics: cfg.Topics, Group: topic, Workers: 2})
			}
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			entered := make(chan struct{}, 9)
			release := make(chan struct{})
			defer func() {
				select {
				case <-release:
				default:
					close(release)
				}
			}()
			done := make(chan error, 1)
			var active, maximum, seen atomic.Int32
			var mu sync.Mutex
			next := map[int32]int{}
			handled := make(chan struct{}, 1)
			go func() {
				done <- c.Consume(ctx, func(ctx context.Context, m kafka.Message) error {
					current := active.Add(1)
					defer active.Add(-1)
					for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
					}
					entered <- struct{}{}
					select {
					case <-release:
					case <-ctx.Done():
						return ctx.Err()
					}
					mu.Lock()
					expected := next[m.Partition]
					next[m.Partition]++
					mu.Unlock()
					if string(m.Value) != fmt.Sprint(expected) {
						return fmt.Errorf("partition %d order: got %s want %d", m.Partition, m.Value, expected)
					}
					if seen.Add(1) == 9 {
						handled <- struct{}{}
					}
					return nil
				})
			}()
			for range 2 {
				select {
				case <-entered:
				case err := <-done:
					t.Fatal(err)
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			if maximum.Load() != 2 {
				t.Fatal("workers did not run concurrently")
			}
			close(release)
			select {
			case <-handled:
			case err := <-done:
				t.Fatal(err)
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			closer := c.(interface{ CloseContext(context.Context) error })
			if err := closer.CloseContext(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case <-done:
			case <-ctx.Done():
				t.Fatal("consumer did not drain")
			}
			if maximum.Load() != 2 || active.Load() != 0 {
				t.Fatal(maximum.Load(), active.Load())
			}
		})
	}
}
