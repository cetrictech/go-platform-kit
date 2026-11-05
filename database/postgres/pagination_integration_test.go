//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"github.com/cetrictech/go-platform-kit/messaging/outbox"
	"github.com/cetrictech/go-platform-kit/pagination"
	"os"
	"testing"
	"time"
)

type ackProducer struct {
	events []kafka.Message
	fail   bool
}

func (p *ackProducer) Publish(_ context.Context, m kafka.Message) error {
	if p.fail {
		return errors.New("broker unavailable")
	}
	p.events = append(p.events, m)
	return nil
}
func (p *ackProducer) PublishBatch(ctx context.Context, m []kafka.Message) error {
	for _, v := range m {
		_ = p.Publish(ctx, v)
	}
	return nil
}
func (*ackProducer) Flush(context.Context) error { return nil }
func (*ackProducer) Close() error                { return nil }
func TestIntegrationKeysetBatchOutbox(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, e := New(ctx, Config{AllowPlaintext: true, URL: dsn})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	table := fmt.Sprintf("gpk_page_%d", time.Now().UnixNano())
	if _, e = c.Exec(ctx, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, rank_value BIGINT NULL)"); e != nil {
		t.Fatal(e)
	}
	defer c.Exec(context.Background(), "DROP TABLE "+table)
	ops := []Operation{{SQL: "INSERT INTO " + table + " (id,rank_value) VALUES (1,NULL)"}, {SQL: "INSERT INTO " + table + " (id,rank_value) VALUES (2,5)"}, {SQL: "INSERT INTO " + table + " (id,rank_value) VALUES (3,5)"}, {SQL: "INSERT INTO " + table + " (id,rank_value) VALUES (4,6)"}}
	if r, e := c.Batch(ctx, ops); e != nil || len(r) != 4 {
		t.Fatal(r, e)
	}
	sorts := []pagination.Sort{{Name: "rank", NullsFirst: true}, {Name: "id", Unique: true}}
	allow := map[string]string{"rank": "rank_value", "id": "id"}
	p, e := pagination.Keyset("postgres", allow, sorts, []any{5, 2}, false, 0)
	if e != nil {
		t.Fatal(e)
	}
	var ids []int
	if e = c.Select(ctx, &ids, "SELECT id FROM "+table+" WHERE "+p.Where+" ORDER BY "+p.OrderBy, p.Args...); e != nil || len(ids) != 2 || ids[0] != 3 || ids[1] != 4 {
		t.Fatal(ids, e)
	}
	p, e = pagination.Keyset("postgres", allow, sorts, []any{5, 3}, true, 0)
	if e != nil {
		t.Fatal(e)
	}
	ids = nil
	if e = c.Select(ctx, &ids, "SELECT id FROM "+table+" WHERE "+p.Where+" ORDER BY "+p.OrderBy, p.Args...); e != nil || len(ids) != 2 || ids[0] != 2 || ids[1] != 1 {
		t.Fatal(ids, e)
	}
	ddl := "CREATE TABLE IF NOT EXISTS sdk_outbox (id VARCHAR(36) PRIMARY KEY,payload BYTEA NOT NULL,attempts INTEGER NOT NULL,next_attempt TIMESTAMPTZ NOT NULL,created_at TIMESTAMPTZ NOT NULL,delivered_at TIMESTAMPTZ,dead_at TIMESTAMPTZ)"
	if _, e = c.Exec(ctx, ddl); e != nil {
		t.Fatal(e)
	}
	r, e := outbox.New(kafka.Message{Topic: "test-outbox", Value: []byte("event")})
	if e != nil {
		t.Fatal(e)
	}
	q, args, e := outbox.InsertSQL("postgres", r)
	if e != nil {
		t.Fatal(e)
	}
	if e = c.WithinTx(ctx, TxOptions{}, func(tx *Tx) error { _, e := tx.Exec(ctx, q, args...); return e }); e != nil {
		t.Fatal(e)
	}
	defer c.Exec(context.Background(), "DELETE FROM sdk_outbox WHERE id=$1", r.ID)
	producer := &ackProducer{}
	sqlDB := c.SQLDB()
	defer sqlDB.Close()
	store := outbox.SQLStore{DB: sqlDB, Dialect: "postgres", Producer: producer, BatchSize: 10, MaxAttempts: 3, RetryDelay: time.Second}
	if n, e := store.Dispatch(ctx); e != nil || n != 1 || len(producer.events) != 1 {
		t.Fatal(n, e)
	}
	if n, e := store.Dispatch(ctx); e != nil || n != 0 {
		t.Fatal(n, e)
	}
	failed, e := outbox.New(kafka.Message{Topic: "test-outbox", Value: []byte("failure")})
	if e != nil {
		t.Fatal(e)
	}
	q, args, e = outbox.InsertSQL("postgres", failed)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Exec(ctx, q, args...); e != nil {
		t.Fatal(e)
	}
	defer c.Exec(context.Background(), "DELETE FROM sdk_outbox WHERE id=$1", failed.ID)
	producer.fail = true
	store.MaxAttempts = 1
	if n, e := store.Dispatch(ctx); e == nil || n != 0 {
		t.Fatal(n, e)
	}
	var attempts int
	if e = c.Get(ctx, &attempts, "SELECT attempts FROM sdk_outbox WHERE id=$1", failed.ID); e != nil || attempts != 1 {
		t.Fatal(attempts, e)
	}
	if n, e := store.Dispatch(ctx); e != nil || n != 0 {
		t.Fatal(n, e)
	}

}
