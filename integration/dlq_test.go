//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
	"github.com/cetrictech/go-platform-kit/retry"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRetryAndDLQBrokerDelivery(t *testing.T) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		t.Skip("KAFKA_BROKER unset")
	}
	for _, adapter := range []string{"franz", "sarama"} {
		t.Run(adapter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			topic := fmt.Sprintf("gpk-dlq-%s-%d", adapter, time.Now().UnixNano())
			cfg := f.Config{AllowPlaintext: true, Brokers: []string{broker}, Topics: []string{topic}, Group: topic}
			admin, err := f.NewAdmin(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
				t.Fatal(err)
			}
			defer admin.DeleteTopic(context.Background(), topic)
			var producer kafka.Producer
			if adapter == "franz" {
				producer, err = f.NewProducer(cfg)
			} else {
				producer, err = s.NewProducer(s.Config{AllowPlaintext: true, Brokers: cfg.Brokers})
			}
			if err != nil {
				t.Fatal(err)
			}
			defer producer.Close()
			poison := errors.New("password=never-publish-this-diagnostic")
			attempts := 0
			handler := func(context.Context, kafka.Message) error { attempts++; return poison }
			policy := retry.Policy{MaxAttempts: 3, Idempotent: true, Retryable: func(error) bool { return true }}
			message := kafka.Message{Topic: "source", Partition: 2, Offset: 9007199254740993, Value: []byte("poison"), Headers: map[string][]byte{"format": []byte("json")}}
			if err := kafka.WithFailure(handler, policy, unavailableProducer{}, topic)(ctx, message); err == nil {
				t.Fatal("DLQ failure acknowledged")
			}
			if attempts != 3 {
				t.Fatal(attempts)
			}
			attempts = 0
			if err := kafka.WithFailure(handler, policy, producer, topic)(ctx, message); err != nil {
				t.Fatal(err)
			}
			if attempts != 3 {
				t.Fatal(attempts)
			}
			consumer, err := f.NewConsumer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer consumer.Close()
			stop := errors.New("verified DLQ")
			if err := consumer.Consume(ctx, func(ctx context.Context, m kafka.Message) error {
				for _, v := range m.Headers {
					if strings.Contains(string(v), "never-publish") {
						return errors.New("diagnostic leaked")
					}
				}
				if string(m.Headers["original-topic"]) != "source" || string(m.Headers["original-offset"]) != "9007199254740993" || string(m.Headers["attempts"]) != "3" || string(m.Value) != "poison" {
					return errors.New("DLQ metadata lost")
				}
				return stop
			}); !errors.Is(err, stop) {
				t.Fatal(err)
			}
		})
	}
}
