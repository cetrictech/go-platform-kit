//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
)

// TestKafkaConcurrentConsumerRebalance observes a real join and leave, rather
// than treating construction of a replacement consumer as rebalance coverage.
func TestKafkaConcurrentConsumerRebalance(t *testing.T) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		t.Skip("KAFKA_BROKER unset")
	}
	for _, adapter := range []string{"franz", "sarama"} {
		t.Run(adapter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			topic := fmt.Sprintf("gpk-rebalance-%s-%d", adapter, time.Now().UnixNano())
			cfg := f.Config{AllowPlaintext: true, Brokers: []string{broker}, Topics: []string{topic}, Group: topic, Workers: 4, ManualPartition: true}
			admin, err := f.NewAdmin(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 4, ReplicationFactor: 1}); err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				if err := admin.DeleteTopic(cleanup, topic); err != nil {
					t.Error(err)
				}
			}()
			producer, err := f.NewProducer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer producer.Close()
			newConsumer := func() (kafka.Consumer, error) {
				if adapter == "franz" {
					return f.NewConsumer(cfg)
				}
				return s.NewConsumer(s.Config{AllowPlaintext: true, Brokers: cfg.Brokers, Topics: cfg.Topics, Group: cfg.Group, Workers: 4})
			}
			type event struct {
				owner     int
				partition int32
				value     string
			}
			var mu sync.Mutex
			seen := map[string]event{}
			observed := make(chan struct{}, 1)
			handler := func(owner int) kafka.Handler {
				return func(ctx context.Context, m kafka.Message) error {
					if err := ctx.Err(); err != nil {
						return err
					}
					mu.Lock()
					seen[string(m.Value)] = event{owner, m.Partition, string(m.Value)}
					mu.Unlock()
					select {
					case observed <- struct{}{}:
					default:
					}
					return nil
				}
			}
			consume := func(owner int) (kafka.Consumer, <-chan error) {
				c, err := newConsumer()
				if err != nil {
					t.Fatal(err)
				}
				done := make(chan error, 1)
				go func() { done <- c.Consume(ctx, handler(owner)) }()
				t.Cleanup(func() {
					cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
					defer stop()
					if err := c.(interface{ CloseContext(context.Context) error }).CloseContext(cleanup); err != nil {
						t.Error(err)
					}
				})
				return c, done
			}
			first, firstDone := consume(1)
			produced := map[string]bool{}
			publishRound := func(stage string) {
				for partition := range int32(4) {
					value := fmt.Sprintf("%s/%d", stage, partition)
					if err := producer.Publish(ctx, kafka.Message{Topic: topic, Partition: partition, Value: []byte(value)}); err != nil {
						t.Fatal(err)
					}
					produced[value] = true
				}
			}
			await := func(description string, condition func(map[string]event) bool, done <-chan error) {
				t.Helper()
				for {
					mu.Lock()
					ok := condition(seen)
					mu.Unlock()
					if ok {
						return
					}
					select {
					case <-observed:
					case err := <-done:
						t.Fatalf("%s: consumer stopped: %v", description, err)
					case <-ctx.Done():
						t.Fatalf("%s: %v", description, ctx.Err())
					}
				}
			}
			publishRound("initial")
			await("first consumer assigned all partitions", func(events map[string]event) bool {
				for partition := range int32(4) {
					if events[fmt.Sprintf("initial/%d", partition)].owner != 1 {
						return false
					}
				}
				return true
			}, firstDone)
			second, secondDone := consume(2)
			// Supply fresh messages until the broker assigns partitions to the joining
			// member. A bounded number of rounds avoids an unbounded producer loop.
			joined := false
			for round := 0; round < 100 && !joined; round++ {
				publishRound(fmt.Sprintf("join-%d", round))
				deadline := time.NewTimer(100 * time.Millisecond)
				select {
				case <-deadline.C:
				case err := <-secondDone:
					deadline.Stop()
					t.Fatalf("joining consumer stopped: %v", err)
				case <-ctx.Done():
					deadline.Stop()
					t.Fatal(ctx.Err())
				}
				mu.Lock()
				for _, event := range seen {
					if event.owner == 2 {
						joined = true
						break
					}
				}
				mu.Unlock()
			}
			if !joined {
				t.Fatal("second concurrent consumer never received a partition")
			}
			if err := first.(interface{ CloseContext(context.Context) error }).CloseContext(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-firstDone:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			publishRound("after-leave")
			await("surviving consumer assigned all partitions", func(events map[string]event) bool {
				for partition := range int32(4) {
					if events[fmt.Sprintf("after-leave/%d", partition)].owner != 2 {
						return false
					}
				}
				for value := range produced {
					if _, ok := events[value]; !ok {
						return false
					}
				}
				return true
			}, secondDone)
			if err := second.(interface{ CloseContext(context.Context) error }).CloseContext(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-secondDone:
				if err != nil && !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}
