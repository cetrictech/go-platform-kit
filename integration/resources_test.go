//go:build integration

package integration

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	redis "github.com/cetrictech/go-platform-kit/cache/redis"
	"github.com/cetrictech/go-platform-kit/database/mysql"
	"github.com/cetrictech/go-platform-kit/database/postgres"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	s "github.com/cetrictech/go-platform-kit/messaging/kafka/sarama"
)

func adapterGoroutines() (int, string) {
	buffer := make([]byte, 1<<20)
	n := runtime.Stack(buffer, true)
	dump := string(buffer[:n])
	count := 0
	for _, stack := range strings.Split(dump, "\n\n") {
		for _, name := range []string{"github.com/twmb/franz-go/", "github.com/IBM/sarama.", "github.com/redis/go-redis/", "github.com/jackc/pgx/", "database/sql.(*DB).connectionOpener"} {
			if strings.Contains(stack, name) {
				count++
				break
			}
		}
	}
	return count, dump
}

// TestInfrastructureResourceBounds repeats actual connection lifecycles and
// exercises pool admission under concurrent load. Native pool counters and a
// stabilized goroutine baseline make leaks visible without mocking cleanup.
func TestInfrastructureResourceBounds(t *testing.T) {
	for _, key := range []string{"POSTGRES_URL", "MYSQL_DSN", "REDIS_ADDRESS", "KAFKA_BROKER"} {
		if os.Getenv(key) == "" {
			t.Skip(key + " unset")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	topic := fmt.Sprintf("gpk-resources-%d", time.Now().UnixNano())
	cfg := f.Config{AllowPlaintext: true, Brokers: []string{os.Getenv("KAFKA_BROKER")}}
	admin, err := f.NewAdmin(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.CreateTopic(ctx, kafka.Topic{Name: topic, Partitions: 1, ReplicationFactor: 1}); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := admin.DeleteTopic(cleanup, topic); err != nil {
			t.Error(err)
		}
		admin.Close()
	}()
	cycle := func() {
		pg, err := postgres.New(ctx, postgres.Config{URL: os.Getenv("POSTGRES_URL"), AllowPlaintext: true, MaxConnections: 2, QueryTimeout: time.Second})
		if err != nil {
			t.Fatal(err)
		}
		my, err := mysql.New(ctx, mysql.Config{DSN: os.Getenv("MYSQL_DSN"), AllowPlaintext: true, MaxOpenConnections: 2, MaxIdleConnections: 1, QueryTimeout: time.Second})
		if err != nil {
			pg.Close()
			t.Fatal(err)
		}
		rd, err := redis.New(ctx, redis.Config{Addresses: []string{os.Getenv("REDIS_ADDRESS")}, AllowPlaintext: true, PoolSize: 2})
		if err != nil {
			pg.Close()
			my.Close()
			t.Fatal(err)
		}
		var workers sync.WaitGroup
		for range 12 {
			workers.Go(func() {
				// Close before consuming rows to test that abandoned streams release leases.
				rows, err := pg.Query(ctx, "SELECT generate_series(1,10)")
				if err != nil {
					t.Error(err)
				} else {
					if err := rows.Close(); err != nil {
						t.Error(err)
					}
				}
				rows, err = my.Query(ctx, "SELECT 1 UNION ALL SELECT 2")
				if err != nil {
					t.Error(err)
				} else {
					if err := rows.Close(); err != nil {
						t.Error(err)
					}
				}
				if err := rd.Ping(ctx); err != nil {
					t.Error(err)
				}
			})
		}
		workers.Wait()
		if stats := pg.Stats(); stats.Acquired != 0 || stats.Total > 2 {
			t.Errorf("postgres pool bound/lease: %+v", stats)
		}
		if stats := my.Stats(); stats.InUse != 0 || stats.OpenConnections > 2 {
			t.Errorf("mysql pool bound/lease: %+v", stats)
		}
		if stats := rd.Stats(); stats.TotalConns > 2 {
			t.Errorf("redis pool bound: %+v", stats)
		}
		pg.Close()
		if err := my.Close(); err != nil {
			t.Error(err)
		}
		if err := rd.Close(); err != nil {
			t.Error(err)
		}
		if stats := pg.Stats(); stats.Total != 0 {
			t.Errorf("postgres connections remain after close: %+v", stats)
		}
		if stats := my.Stats(); stats.OpenConnections != 0 {
			t.Errorf("mysql connections remain after close: %+v", stats)
		}
		if stats := rd.Stats(); stats.TotalConns != 0 {
			t.Errorf("redis connections remain after close: %+v", stats)
		}
		for _, adapter := range []string{"franz", "sarama"} {
			network := new(faultNetwork)
			var producer kafka.Producer
			if adapter == "franz" {
				producer, err = f.NewProducer(f.Config{AllowPlaintext: true, Brokers: cfg.Brokers, DialContext: network.DialContext, MaxBufferedRecords: 8})
			} else {
				producer, err = s.NewProducer(s.Config{AllowPlaintext: true, Brokers: cfg.Brokers, DialContext: network.DialContext})
			}
			if err != nil {
				network.setOffline(true)
				t.Fatal(err)
			}
			if err := producer.Publish(ctx, kafka.Message{Topic: topic, Value: []byte("resource lifecycle")}); err != nil {
				producer.Close()
				network.setOffline(true)
				t.Fatal(err)
			}
			if err := producer.Close(); err != nil {
				t.Error(err)
			}
			// Sarama settles native broker socket teardown asynchronously after
			// draining delivery channels. Observe the ownership postcondition
			// within a finite bound instead of requiring scheduler coincidence.
			settle := time.NewTimer(2 * time.Second)
			ticker := time.NewTicker(10 * time.Millisecond)
			closed := false
			for !closed {
				network.mu.Lock()
				open := len(network.connections)
				network.mu.Unlock()
				if open == 0 {
					break
				}
				select {
				case <-ticker.C:
				case <-settle.C:
					t.Errorf("%s retained %d network connections after close", adapter, open)
					closed = true
				case <-ctx.Done():
					t.Error(ctx.Err())
					closed = true
				}
			}
			settle.Stop()
			ticker.Stop()
			network.setOffline(true)
		}
	}
	cycle() // Warm driver/global runtime initialization before measuring owned work.
	baseline, _ := adapterGoroutines()
	for range 8 {
		cycle()
	}
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, dump := adapterGoroutines()
		if current <= baseline {
			return
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("adapter goroutines grew across repeated Close: before=%d after=%d\n%s", baseline, current, dump)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}
