//go:build integration

package mysql

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	db "github.com/cetrictech/go-platform-kit/database"
	"github.com/cetrictech/go-platform-kit/pagination"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
)

func TestEndToEndSQLFeatures(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var mu sync.Mutex
	var events []string
	var logs bytes.Buffer
	hooks := db.Hooks{AfterQuery: func(_ context.Context, e db.QueryEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, "query")
	}, AfterTransaction: func(_ context.Context, e db.TransactionEvent) {
		mu.Lock()
		defer mu.Unlock()
		events = append(events, e.Operation)
	}}
	c, e := New(ctx, Config{AllowPlaintext: true, DSN: dsn, Hooks: hooks, QueryLogger: slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	table := fmt.Sprintf("gpk_full_%d", time.Now().UnixNano())
	if _, e = c.Exec(ctx, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, rank_value BIGINT NULL, payload JSON)"); e != nil {
		t.Fatal(e)
	}
	defer c.Exec(context.Background(), "DROP TABLE "+table)
	for _, q := range []string{"INSERT INTO " + table + " VALUES (1,NULL,'{\"n\":1}')", "INSERT INTO " + table + " VALUES (2,5,'{\"n\":2}')", "INSERT INTO " + table + " VALUES (3,5,'{\"n\":3}')", "INSERT INTO " + table + " VALUES (9007199254740993,6,'{\"n\":4}')"} {
		if _, e = c.Exec(ctx, q); e != nil {
			t.Fatal(e)
		}
	}
	type item struct {
		ID      int64
		Rank    *int64         `db:"rank_value"`
		Payload map[string]int `db:"payload,json"`
	}
	var data []item
	if e = c.Select(ctx, &data, "SELECT id,rank_value,payload FROM "+table+" ORDER BY id"); e != nil || len(data) != 4 || data[0].Rank != nil || data[3].Payload["n"] != 4 {
		t.Fatal(data, e)
	}
	statement, e := c.Prepare(ctx, "SELECT id FROM "+table+" WHERE id=?")
	if e != nil {
		t.Fatal(e)
	}
	var id int64
	if e = statement.Get(ctx, &id, int64(9007199254740993)); e != nil || id != 9007199254740993 {
		t.Fatal(id, e)
	}
	if e = statement.Close(); e != nil {
		t.Fatal(e)
	}
	ring, e := pagination.NewKeyRing("a", map[string][]byte{"a": bytes.Repeat([]byte{1}, 32)})
	if e != nil {
		t.Fatal(e)
	}
	codec := pagination.Codec{Provider: ring, TTL: time.Hour}
	query := pagination.Query{Dialect: "mysql", Template: "SELECT id,rank_value,payload FROM " + table + " WHERE {{where}} ORDER BY {{order}} LIMIT {{limit}}", Identity: "full-query", Filter: "all", Columns: map[string]string{"rank": "rank_value", "id": "id"}, Sort: []pagination.Sort{{Name: "rank", NullsFirst: true}, {Name: "id", Unique: true}}, KeyTypes: []pagination.KeyType{pagination.KeyInt64, pagination.KeyInt64}}
	keys := func(v item) ([]any, error) {
		var rank any
		if v.Rank != nil {
			rank = *v.Rank
		}
		return []any{rank, v.ID}, nil
	}
	count := func(ctx context.Context) (int64, error) {
		var n int64
		e := c.Get(ctx, &n, "SELECT COUNT(*) FROM "+table)
		return n, e
	}
	first, e := pagination.FetchCursor(ctx, c.Select, query, pagination.CursorRequest{Limit: 2}, codec, keys, count)
	if e != nil || len(first.Data) != 2 || first.Data[0].ID != 1 || first.Meta.Next == "" || first.Meta.Total == nil || *first.Meta.Total != 4 {
		t.Fatal(first, e)
	}
	second, e := pagination.FetchCursor(ctx, c.Select, query, pagination.CursorRequest{Cursor: first.Meta.Next, Limit: 2}, codec, keys, nil)
	if e != nil || len(second.Data) != 2 || second.Data[1].ID != 9007199254740993 || second.Meta.Previous == "" || second.Meta.HasNext {
		t.Fatal(second, e)
	}
	back, e := pagination.FetchCursor(ctx, c.Select, query, pagination.CursorRequest{Cursor: second.Meta.Previous, Limit: 2}, codec, keys, nil)
	if e != nil || len(back.Data) != 2 || back.Data[0].ID != 1 || back.Data[1].ID != 2 {
		t.Fatal(back, e)
	}
	offsetSQL, args, e := pagination.OffsetSQL("mysql", "SELECT id FROM "+table+" ORDER BY id LIMIT {{limit}} OFFSET {{offset}}", nil, pagination.Offset{Page: 2, Limit: 2})
	if e != nil {
		t.Fatal(e)
	}
	var ids []int64
	if e = c.Select(ctx, &ids, offsetSQL, args...); e != nil || len(ids) != 2 || ids[0] != 3 {
		t.Fatal(ids, e)
	}
	sentinel := errors.New("rollback")
	e = c.WithinTx(ctx, TxOptions{}, func(tx *Tx) error {
		_, e := tx.Exec(ctx, "INSERT INTO "+table+" VALUES (10,10,'{}')")
		if e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	mu.Lock()
	sawBegin, sawRollback := false, false
	for _, e := range events {
		sawBegin = sawBegin || e == "begin"
		sawRollback = sawRollback || e == "rollback"
	}
	mu.Unlock()
	if !sawBegin || !sawRollback {
		t.Fatal(events)
	}
	if bytes.Contains(logs.Bytes(), []byte("INSERT INTO")) || bytes.Contains(logs.Bytes(), []byte("SELECT id")) {
		t.Fatal("raw SQL leaked to query logs")
	}
}
