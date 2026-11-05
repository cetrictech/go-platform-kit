//go:build integration

package mysql

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestIntegrationTransactions(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, e := New(ctx, Config{AllowPlaintext: true, DSN: dsn})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	table := fmt.Sprintf("gpk_test_%d", time.Now().UnixNano())
	if _, e = c.Exec(ctx, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY, name VARCHAR(100))"); e != nil {
		t.Fatal(e)
	}
	defer c.Exec(context.Background(), "DROP TABLE "+table)
	placeholder := "?,?"
	insert := "INSERT INTO " + table + " (id,name) VALUES (" + placeholder + ")"
	e = c.WithinTx(ctx, TxOptions{}, func(tx *Tx) error { _, e := tx.Exec(ctx, insert, 1, "committed"); return e })
	if e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("rollback")
	e = c.WithinTx(ctx, TxOptions{}, func(tx *Tx) error {
		if _, e := tx.Exec(ctx, insert, 2, "rolled back"); e != nil {
			return e
		}
		return sentinel
	})
	if !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	var count int
	if e = c.Get(ctx, &count, "SELECT COUNT(*) FROM "+table); e != nil || count != 1 {
		t.Fatal(count, e)
	}
	var rows []struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	if e = c.Select(ctx, &rows, "SELECT id,name FROM "+table); e != nil || len(rows) != 1 || rows[0].Name != "committed" {
		t.Fatal(rows, e)
	}
	if _, e = c.Exec(ctx, insert, 1, "duplicate"); e == nil {
		t.Fatal("duplicate accepted")
	}
	ctx2, stop := context.WithCancel(ctx)
	stop()
	if c.Ping(ctx2) == nil {
		t.Fatal("cancellation ignored")
	}
}
