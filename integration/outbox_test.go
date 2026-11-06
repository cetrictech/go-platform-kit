//go:build integration

package integration

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/database/postgres"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	f "github.com/cetrictech/go-platform-kit/messaging/kafka/franz"
	"github.com/cetrictech/go-platform-kit/messaging/outbox"
	"os"
	"testing"
	"time"
)

type unavailableProducer struct{ kafka.Producer }

func (unavailableProducer) Publish(context.Context, kafka.Message) error {
	return errors.New("injected broker outage")
}
func TestTransactionalOutboxBrokerDelivery(t *testing.T) {
	url, broker := os.Getenv("POSTGRES_URL"), os.Getenv("KAFKA_BROKER")
	if url == "" || broker == "" {
		t.Skip("POSTGRES_URL/KAFKA_BROKER unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := postgres.New(ctx, postgres.Config{URL: url, AllowPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	bridge := db.SQLDB()
	defer bridge.Close()
	// Explicit test migration. The SDK never creates production schema automatically.
	_, err = bridge.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS sdk_outbox (id VARCHAR(36) PRIMARY KEY,payload BYTEA NOT NULL,attempts INTEGER NOT NULL DEFAULT 0,next_attempt TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL,delivered_at TIMESTAMPTZ,dead_at TIMESTAMPTZ)`)
	if err != nil {
		t.Fatal(err)
	}
	topic := fmt.Sprintf("gpk-outbox-%d", time.Now().UnixNano())
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
	p, err := f.NewProducer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	record, err := outbox.New(kafka.Message{Topic: topic, Value: []byte("committed business event")})
	if err != nil {
		t.Fatal(err)
	}
	query, args, err := outbox.InsertSQL("postgres", record)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := bridge.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := bridge.QueryRowContext(ctx, "SELECT count(*) FROM sdk_outbox WHERE id=$1", record.ID).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	tx, err = bridge.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	defer bridge.ExecContext(context.Background(), "DELETE FROM sdk_outbox WHERE id=$1", record.ID)
	store := outbox.SQLStore{DB: bridge, Dialect: "postgres", Producer: unavailableProducer{}, BatchSize: 100, MaxAttempts: 3, RetryDelay: time.Millisecond}
	if n, err := store.Dispatch(ctx); err == nil || n != 0 {
		t.Fatal(n, err)
	}
	var attempts int
	if err := bridge.QueryRowContext(ctx, "SELECT attempts FROM sdk_outbox WHERE id=$1", record.ID).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal(attempts, err)
	}
	if _, err := bridge.ExecContext(ctx, "UPDATE sdk_outbox SET next_attempt=CURRENT_TIMESTAMP WHERE id=$1", record.ID); err != nil {
		t.Fatal(err)
	}
	store.Producer = p
	if n, err := store.Dispatch(ctx); err != nil || n < 1 {
		t.Fatal(n, err)
	}
	c, err := f.NewConsumer(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	stop := errors.New("received")
	if err := c.Consume(ctx, func(ctx context.Context, m kafka.Message) error {
		if string(m.Headers["event-id"]) != record.ID || string(m.Value) != "committed business event" {
			return errors.New("event metadata mismatch")
		}
		return stop
	}); !errors.Is(err, stop) {
		t.Fatal(err)
	}
	if n, err := store.Dispatch(ctx); err != nil || n != 0 {
		t.Fatal(n, err)
	}
}
