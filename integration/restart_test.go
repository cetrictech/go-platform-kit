//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

	redis "github.com/cetrictech/go-platform-kit/cache/redis"
	"github.com/cetrictech/go-platform-kit/database/mysql"
	"github.com/cetrictech/go-platform-kit/database/postgres"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
)

const (
	localPostgres = "postgres://postgres:sdk-test-only@127.0.0.1:15432/sdk?sslmode=disable"
	localMySQL    = "root:sdk-test-only@tcp(127.0.0.1:13306)/sdk?parseTime=true"
	localRedis    = "127.0.0.1:16379"
	localKafka    = "127.0.0.1:19092"
)

// Dependency process restarts are opt-in: ordinary integration never restarts
// infrastructure provided by an application. The fault gate sets fixed endpoints.
func requireLocalFaultFixtures(t *testing.T) {
	t.Helper()
	if os.Getenv("SDK_FAULT_TESTS") != "1" {
		t.Skip("run scripts/fault-integration.sh for destructive local-fixture faults")
	}
	for key, want := range map[string]string{"POSTGRES_URL": localPostgres, "MYSQL_DSN": localMySQL, "REDIS_ADDRESS": localRedis, "KAFKA_BROKER": localKafka} {
		if os.Getenv(key) != want {
			t.Fatalf("fault tests require the exact disposable local fixture for %s", key)
		}
	}
}

func restartLocalService(t *testing.T, service string) {
	t.Helper()
	switch service {
	case "postgres", "mysql", "redis", "redpanda":
	default:
		t.Fatal("unrecognized local service")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	// compose.yaml declares the fixed go-platform-kit-tests project and loopback
	// listeners. No caller-controlled compose path or service is accepted.
	command := exec.CommandContext(ctx, "docker", "compose", "-f", "compose.yaml", "restart", "--timeout", "5", service)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("restart %s: %v\n%s", service, err, output)
	}
}

func awaitDependency(t *testing.T, ctx context.Context, operation func(context.Context) error) {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var last error
	for {
		attempt, cancel := context.WithTimeout(ctx, time.Second)
		last = operation(attempt)
		cancel()
		if last == nil {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("dependency did not recover: %v (last operation: %v)", ctx.Err(), last)
		}
	}
}

func TestDependencyProcessRestartRecovery(t *testing.T) {
	requireLocalFaultFixtures(t)
	t.Run("postgres", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		client, err := postgres.New(ctx, postgres.Config{URL: localPostgres, AllowPlaintext: true, MaxConnections: 2, ConnectTimeout: time.Second, QueryTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		var before int64
		if err := client.QueryRow(ctx, "SELECT (extract(epoch FROM pg_postmaster_start_time())*1000000)::bigint").Scan(&before); err != nil {
			t.Fatal(err)
		}
		restartLocalService(t, "postgres")
		awaitDependency(t, ctx, func(ctx context.Context) error {
			var after int64
			if err := client.QueryRow(ctx, "SELECT (extract(epoch FROM pg_postmaster_start_time())*1000000)::bigint").Scan(&after); err != nil {
				return err
			}
			if after == before {
				return errors.New("original server process still observed")
			}
			return nil
		})
		if stats := client.Stats(); stats.Acquired != 0 || stats.Total > 2 {
			t.Fatalf("pool did not release/bound connections: %+v", stats)
		}
	})
	t.Run("mysql", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		client, err := mysql.New(ctx, mysql.Config{DSN: localMySQL + "&readTimeout=1s&writeTimeout=1s", AllowPlaintext: true, MaxOpenConnections: 2, MaxIdleConnections: 1, ConnectTimeout: time.Second, QueryTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if err := client.QueryRow(ctx, "SELECT 1").Scan(new(int)); err != nil {
			t.Fatal(err)
		}
		restartLocalService(t, "mysql")
		awaitDependency(t, ctx, func(ctx context.Context) error {
			var n int
			if err := client.QueryRow(ctx, "SELECT 1").Scan(&n); err != nil {
				return err
			}
			if n != 1 {
				return errors.New("unexpected result")
			}
			return nil
		})
		if stats := client.Stats(); stats.InUse != 0 || stats.OpenConnections > 2 {
			t.Fatalf("pool did not release/bound connections: %+v", stats)
		}
	})
	t.Run("redis", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		client, err := redis.New(ctx, redis.Config{Addresses: []string{localRedis}, AllowPlaintext: true, PoolSize: 2, MaxRetries: -1, DialTimeout: time.Second, ReadTimeout: time.Second, WriteTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		defer client.Close()
		if err := client.Ping(ctx); err != nil {
			t.Fatal(err)
		}
		restartLocalService(t, "redis")
		awaitDependency(t, ctx, client.Ping)
		key := fmt.Sprintf("gpk-restart-%d", time.Now().UnixNano())
		if err := client.Set(ctx, key, "recovered", time.Minute); err != nil {
			t.Fatal(err)
		}
		defer client.Delete(context.Background(), key)
		if value, err := client.Get(ctx, key); err != nil || value != "recovered" {
			t.Fatal(value, err)
		}
		if stats := client.Stats(); stats.TotalConns > 2 {
			t.Fatalf("pool exceeded configured bound: %+v", stats)
		}
	})
	for _, adapter := range []string{"franz", "sarama"} {
		t.Run("kafka-"+adapter, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			topic := fmt.Sprintf("gpk-restart-%s-%d", adapter, time.Now().UnixNano())
			cfg := f.Config{AllowPlaintext: true, Brokers: []string{localKafka}, Topics: []string{topic}, Group: topic}
			admin, err := f.NewAdmin(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
				defer stop()
				if err := admin.DeleteTopic(cleanup, topic); err != nil {
					t.Error(err)
				}
			}()
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
			if err := producer.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("before-restart")}); err != nil {
				t.Fatal(err)
			}
			restartLocalService(t, "redpanda")
			awaitDependency(t, ctx, func(ctx context.Context) error {
				return producer.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("after-restart")})
			})
			consumer, err := f.NewConsumer(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer consumer.Close()
			found := errors.New("recovered delivery")
			err = consumer.Consume(ctx, func(ctx context.Context, m kafka.Message) error {
				if string(m.Value) == "after-restart" {
					return found
				}
				return nil
			})
			if !errors.Is(err, found) {
				t.Fatal(err)
			}
		})
	}
}
