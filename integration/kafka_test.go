//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
	"os"
	"testing"
	"time"
)

func TestCrossAdapterOffsets(t *testing.T) {
	broker := os.Getenv("KAFKA_BROKER")
	if broker == "" {
		t.Skip("KAFKA_BROKER unset")
	}
	for _, producer := range []string{"franz", "sarama"} {
		for _, consumer := range []string{"franz", "sarama"} {
			t.Run(producer+"-to-"+consumer, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
				defer cancel()
				topic := fmt.Sprintf("gpk-cross-%s-%s-%d", producer, consumer, time.Now().UnixNano())
				fc := f.Config{AllowPlaintext: true, Brokers: []string{broker}, Topics: []string{topic}, Group: topic}
				sc := s.Config{AllowPlaintext: true, Brokers: []string{broker}, Topics: []string{topic}, Group: topic}
				admin, e := f.NewAdmin(fc)
				if e != nil {
					t.Fatal(e)
				}
				defer admin.Close()
				if e = admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); e != nil {
					t.Fatal(e)
				}
				defer admin.DeleteTopic(context.Background(), topic)
				var p kafka.Producer
				if producer == "franz" {
					p, e = f.NewProducer(fc)
				} else {
					p, e = s.NewProducer(sc)
				}
				if e != nil {
					t.Fatal(e)
				}
				defer p.Close()
				for _, v := range []string{"committed", "uncommitted"} {
					if e = p.Publish(ctx, kafka.Message{Topic: topic, Key: []byte("key"), Value: []byte(v), Headers: map[string][]byte{"format": []byte("common")}}); e != nil {
						t.Fatal(e)
					}
				}
				newConsumer := func() (kafka.Consumer, error) {
					if consumer == "franz" {
						return f.NewConsumer(fc)
					}
					return s.NewConsumer(sc)
				}
				c, e := newConsumer()
				if e != nil {
					t.Fatal(e)
				}
				stop := errors.New("intentional handler error")
				seen := 0
				e = c.Consume(ctx, func(_ context.Context, m kafka.Message) error {
					seen++
					if string(m.Headers["format"]) != "common" || string(m.Key) != "key" {
						return errors.New("lost metadata")
					}
					if string(m.Value) == "uncommitted" {
						return stop
					}
					return nil
				})
				_ = c.Close()
				if !errors.Is(e, stop) || seen != 2 {
					t.Fatal(seen, e)
				}
				c, e = newConsumer()
				if e != nil {
					t.Fatal(e)
				}
				defer c.Close()
				seen = 0
				e = c.Consume(ctx, func(_ context.Context, m kafka.Message) error {
					seen++
					if string(m.Value) != "uncommitted" {
						return errors.New("committed record replayed")
					}
					return stop
				})
				if !errors.Is(e, stop) || seen != 1 {
					t.Fatal(seen, e)
				}
			})
		}
	}
}
