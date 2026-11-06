//go:build integration

package redis

import (
	"context"
	r "github.com/redis/go-redis/v9"
	"os"
	"testing"
	"time"
)

func TestPoolHasHardConnectionBound(t *testing.T) {
	address := os.Getenv("REDIS_ADDRESS")
	if address == "" {
		t.Skip("REDIS_ADDRESS unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := New(ctx, Config{Addresses: []string{address}, AllowPlaintext: true, PoolSize: 2, MaxRetries: -1, ReadTimeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	native := client.Native().(*r.Client)
	held := []*r.Conn{native.Conn(), native.Conn()}
	for _, conn := range held {
		defer conn.Close()
		if err := conn.Ping(ctx).Err(); err != nil {
			t.Fatal(err)
		}
	}
	full, cancelFull := context.WithTimeout(ctx, 100*time.Millisecond)
	err = client.Ping(full)
	cancelFull()
	if err == nil {
		t.Fatal("third command allocated beyond the configured connection bound")
	}
	full, cancelFull = context.WithTimeout(ctx, 100*time.Millisecond)
	err = client.Pipeline(full, func(p r.Pipeliner) error { p.Ping(full); return nil })
	cancelFull()
	if err == nil {
		t.Fatal("pipeline bypassed the configured connection bound")
	}
	if stats := client.Stats(); stats.TotalConns > 2 {
		t.Fatalf("pool escaped bound: %+v", stats)
	}
	// Releasing a lease restores progress without opening an extra connection.
	if err := held[0].Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Pipeline(ctx, func(p r.Pipeliner) error { p.Ping(ctx); return nil }); err != nil {
		t.Fatal(err)
	}
	if stats := client.Stats(); stats.TotalConns > 2 {
		t.Fatalf("recovered pool escaped bound: %+v", stats)
	}
}
