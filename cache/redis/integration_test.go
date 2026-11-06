//go:build integration

package redis

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/idempotency"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestIntegrationAtomic(t *testing.T) {
	addr := os.Getenv("REDIS_ADDRESS")
	if addr == "" {
		t.Skip("REDIS_ADDRESS unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, e := New(ctx, Config{AllowPlaintext: true, Addresses: []string{addr}, Prefix: fmt.Sprintf("gpk:%d:", time.Now().UnixNano())})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if e = c.SetJSON(ctx, "json", map[string]int{"n": 42}, time.Minute); e != nil {
		t.Fatal(e)
	}
	var value map[string]int
	if e = c.GetJSON(ctx, "json", &value); e != nil || value["n"] != 42 {
		t.Fatal(value, e)
	}
	lock, e := c.Acquire(ctx, "lock", time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Acquire(ctx, "lock", time.Second); !errors.Is(e, ErrLockBusy) {
		t.Fatal(e)
	}
	if e = lock.Renew(ctx, time.Second); e != nil {
		t.Fatal(e)
	}
	if e = lock.Release(ctx); e != nil {
		t.Fatal(e)
	}
	if e = lock.Release(ctx); !errors.Is(e, ErrOwnership) {
		t.Fatal(e)
	}
	s := IdempotencyStore{c}
	var wg sync.WaitGroup
	var won atomic.Int32
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Begin(ctx, "idem", "fingerprint", time.Minute)
			if e == nil {
				won.Add(1)
				if e = s.Complete(ctx, "idem", r.Token, 200, time.Minute); e != nil {
					t.Error(e)
				}
			} else if !errors.Is(e, idempotency.ErrBusy) && !errors.Is(e, idempotency.ErrCompleted) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatal(won.Load())
	}
	if _, e = s.Begin(ctx, "idem", "different", time.Minute); !errors.Is(e, idempotency.ErrMismatch) {
		t.Fatal(e)
	}
	_, _ = c.Delete(ctx, "json", "idem", "lock")
}

func BenchmarkRedisRoundTrip(b *testing.B) {
	addr := os.Getenv("REDIS_ADDRESS")
	if addr == "" {
		b.Skip("REDIS_ADDRESS unset")
	}
	c, e := New(context.Background(), Config{AllowPlaintext: true, Addresses: []string{addr}, Prefix: "gpk:benchmark:"})
	if e != nil {
		b.Fatal(e)
	}
	defer c.Close()
	defer c.Delete(context.Background(), "key")
	if e = c.Set(context.Background(), "key", "value", time.Minute); e != nil {
		b.Fatal(e)
	}
	b.ResetTimer()
	for b.Loop() {
		if _, e = c.Get(context.Background(), "key"); e != nil {
			b.Fatal(e)
		}
	}
}
