//go:build integration

package integration

import (
	"context"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestKafkaCredentialRotationAndNetworkRecovery(t *testing.T) {
	secure := testTLS(t)
	for _, adapter := range []string{"franz", "sarama"} {
		t.Run(adapter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
			defer cancel()
			user := fmt.Sprintf("rotate-%s-%d", adapter, time.Now().UnixNano())
			update := func(method, password string) {
				endpoint := "http://127.0.0.1:29644/v1/security/users"
				if method != "POST" {
					endpoint += "/" + user
				}
				body := fmt.Sprintf(`{"username":%q,"password":%q,"algorithm":"SCRAM-SHA-512"}`, user, password)
				req, err := http.NewRequestWithContext(ctx, method, endpoint, strings.NewReader(body))
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Content-Type", "application/json")
				res, err := http.DefaultClient.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				res.Body.Close()
				if res.StatusCode >= 300 {
					t.Fatalf("credential update: HTTP %d", res.StatusCode)
				}
			}
			update("POST", "before")
			defer update("DELETE", "")
			var password atomic.Value
			password.Store("before")
			var calls atomic.Int32
			provider := func(ctx context.Context) (kafka.Credentials, error) {
				calls.Add(1)
				return kafka.Credentials{Username: user, Password: password.Load().(string)}, ctx.Err()
			}
			network := new(faultNetwork)
			defer network.setOffline(true)
			topic := user
			admin, err := f.NewAdmin(f.Config{Brokers: []string{"127.0.0.1:29092"}, TLS: secure, SASL: "SCRAM-SHA-512", Credentials: provider})
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
				t.Fatal(err)
			}
			defer admin.DeleteTopic(context.Background(), topic)
			var p kafka.Producer
			if adapter == "franz" {
				p, err = f.NewProducer(f.Config{Brokers: []string{"127.0.0.1:29092"}, TLS: secure, SASL: "SCRAM-SHA-512", Credentials: provider, DialContext: network.DialContext})
			} else {
				p, err = s.NewProducer(s.Config{Brokers: []string{"127.0.0.1:29092"}, TLS: secure, SASL: "SCRAM-SHA-512", Credentials: provider, DialContext: network.DialContext})
			}
			if err != nil {
				t.Fatal(err)
			}
			defer p.Close()
			if err := p.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("before")}); err != nil {
				t.Fatal(err)
			}
			initialCalls := calls.Load()
			network.setOffline(true)
			short, stop := context.WithTimeout(ctx, 200*time.Millisecond)
			err = p.Publish(short, kafka.Message{Topic: topic, Value: []byte("interrupted")})
			stop()
			if err == nil {
				t.Fatal("outage was hidden")
			}
			update("PUT", "after")
			password.Store("after")
			network.setOffline(false)
			if err := p.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("after")}); err != nil {
				t.Fatal(err)
			}
			if calls.Load() <= initialCalls {
				t.Fatal("credentials were not refreshed on reconnect")
			}
		})
	}
}
