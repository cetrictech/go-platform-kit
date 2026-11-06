//go:build integration

package mysql

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"
)

func TestDefaultPoolHasHardConnectionBound(t *testing.T) {
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		t.Skip("MYSQL_DSN unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client, err := New(ctx, Config{DSN: dsn, AllowPlaintext: true})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if limit := client.Stats().MaxOpenConnections; limit != 32 {
		t.Fatalf("default pool is unbounded or changed contract: %d", limit)
	}
	var held []*sql.Conn
	defer func() {
		for _, connection := range held {
			if err := connection.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	for range 32 {
		connection, err := client.Native().Conn(ctx)
		if err != nil {
			t.Fatal(err)
		}
		held = append(held, connection)
	}
	blocked, stop := context.WithTimeout(ctx, 100*time.Millisecond)
	extra, err := client.Native().Conn(blocked)
	stop()
	if extra != nil {
		extra.Close()
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("pool allocated beyond the default bound: %v", err)
	}
	if stats := client.Stats(); stats.OpenConnections != 32 || stats.InUse != 32 {
		t.Fatalf("pool did not respect admission: %+v", stats)
	}
}
