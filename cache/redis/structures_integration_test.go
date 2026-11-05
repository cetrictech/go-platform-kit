//go:build integration

package redis

import (
	"context"
	"errors"
	"fmt"
	r "github.com/redis/go-redis/v9"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestStructuresScriptsAndStreams(t *testing.T) {
	address := os.Getenv("REDIS_ADDRESS")
	if address == "" {
		t.Skip("REDIS_ADDRESS unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, e := New(ctx, Config{AllowPlaintext: true, Addresses: []string{address}, Prefix: fmt.Sprintf("gpk-structures-%d:", time.Now().UnixNano())})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	defer c.Delete(context.Background(), "hash", "list", "set", "sorted", "counter", "stream", "cached", "watch")
	if _, e = c.HSet(ctx, "hash", "n", 1); e != nil {
		t.Fatal(e)
	}
	if n, e := c.HIncrBy(ctx, "hash", "n", 2); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	if m, e := c.HGetAll(ctx, "hash"); e != nil || m["n"] != "3" {
		t.Fatal(m, e)
	}
	if ok, e := c.HExists(ctx, "hash", "n"); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if _, e = c.HDel(ctx, "hash", "n"); e != nil {
		t.Fatal(e)
	}
	if _, e = c.LPush(ctx, "list", "a"); e != nil {
		t.Fatal(e)
	}
	_, _ = c.RPush(ctx, "list", "b")
	if v, e := c.LRange(ctx, "list", 0, -1); e != nil || len(v) != 2 || v[1] != "b" {
		t.Fatal(v, e)
	}
	if e = c.LTrim(ctx, "list", 1, -1); e != nil {
		t.Fatal(e)
	}
	if v, e := c.RPop(ctx, "list"); e != nil || v != "b" {
		t.Fatal(v, e)
	}
	_, _ = c.SAdd(ctx, "set", "a", "b")
	if n, e := c.SCard(ctx, "set"); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	if ok, e := c.SIsMember(ctx, "set", "a"); e != nil || !ok {
		t.Fatal(ok, e)
	}
	_, _ = c.SRem(ctx, "set", "a")
	if v, e := c.SMembers(ctx, "set"); e != nil || len(v) != 1 {
		t.Fatal(v, e)
	}
	_, _ = c.ZAdd(ctx, "sorted", ScoredMember{Score: 2, Member: "b"}, ScoredMember{Score: 1, Member: "a"})
	if v, e := c.ZRangeByScore(ctx, "sorted", "1", "2"); e != nil || len(v) != 2 || v[0] != "a" {
		t.Fatal(v, e)
	}
	if n, e := c.ZScore(ctx, "sorted", "b"); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	_, _ = c.ZRem(ctx, "sorted", "b")
	if n, e := c.ZCard(ctx, "sorted"); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	script := `return redis.call('INCRBY',KEYS[1],ARGV[1])`
	sha, e := c.ScriptLoad(ctx, script)
	if e != nil {
		t.Fatal(e)
	}
	if v, e := c.EvalSHA(ctx, sha, []string{"counter"}, 3); e != nil || v != int64(3) {
		t.Fatal(v, e)
	}
	if e = c.Pipeline(ctx, func(p r.Pipeliner) error { p.Incr(ctx, c.key("counter")); p.Incr(ctx, c.key("counter")); return nil }); e != nil {
		t.Fatal(e)
	}
	if e = c.Transaction(ctx, func(p r.Pipeliner) error { p.Set(ctx, c.key("watch"), "before", time.Minute); return nil }); e != nil {
		t.Fatal(e)
	}
	e = c.Watch(ctx, func(tx *r.Tx) error {
		if e := c.Set(ctx, "watch", "changed", time.Minute); e != nil {
			return e
		}
		_, e := tx.TxPipelined(ctx, func(p r.Pipeliner) error { p.Set(ctx, c.key("watch"), "wrong", time.Minute); return nil })
		return e
	}, "watch")
	if !errors.Is(e, r.TxFailedErr) {
		t.Fatal("WATCH did not detect competing write", e)
	}
	var calls int
	var object map[string]int
	load := func(context.Context) (any, error) { calls++; return map[string]int{"n": 1}, nil }
	for i := 0; i < 2; i++ {
		if e = c.CacheAside(ctx, "cached", &object, time.Minute, load); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 1 || object["n"] != 1 {
		t.Fatal(calls, object)
	}
	if e = c.XGroupCreate(ctx, "stream", "workers", "0"); e != nil {
		t.Fatal(e)
	}
	id, e := c.XAdd(ctx, "stream", map[string]any{"n": "1"})
	if e != nil {
		t.Fatal(e)
	}
	sentinel := errors.New("handler failure")
	if e = c.ConsumeGroup(ctx, StreamConsumerConfig{Stream: "stream", Group: "workers", Consumer: "first", Block: 10 * time.Millisecond}, func(context.Context, StreamMessage) error { return sentinel }); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	pending, e := c.XPending(ctx, "stream", "workers")
	if e != nil || pending.Count != 1 {
		t.Fatal(pending, e)
	}
	claimed, _, e := c.XAutoClaim(ctx, "stream", "workers", "second", "0-0", 0, 10)
	if e != nil || len(claimed) != 1 || claimed[0].ID != id {
		t.Fatal(claimed, e)
	}
	if n, e := c.XAck(ctx, "stream", "workers", id); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	pending, e = c.XPending(ctx, "stream", "workers")
	if e != nil || pending.Count != 0 {
		t.Fatal(pending, e)
	}
	consumerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- c.ConsumeGroup(consumerCtx, StreamConsumerConfig{Stream: "stream", Group: "workers", Consumer: "second", Block: 10 * time.Millisecond}, func(context.Context, StreamMessage) error { return nil })
	}()
	stop()
	if e = <-done; !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	if _, e = c.XDelete(ctx, "stream", id); e != nil {
		t.Fatal(e)
	}
	if _, e = c.XGroupDestroy(ctx, "stream", "workers"); e != nil {
		t.Fatal(e)
	}
}
func TestSubscriptionsAtomicLimiterAndExpiredOwner(t *testing.T) {
	address := os.Getenv("REDIS_ADDRESS")
	if address == "" {
		t.Skip("REDIS_ADDRESS unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	c, e := New(ctx, Config{AllowPlaintext: true, Addresses: []string{address}, Prefix: fmt.Sprintf("gpk*%d:", time.Now().UnixNano())})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	defer c.Delete(context.Background(), "rate", "lease")
	for _, pattern := range []bool{false, true} {
		stop := errors.New("subscription complete")
		done := make(chan error, 1)
		subscription := "channel"
		if pattern {
			subscription = "channel*"
		}
		go func() {
			done <- c.Subscribe(ctx, pattern, func(_ context.Context, m Message) error {
				if m.Channel != "channel" || m.Payload != "data" {
					return fmt.Errorf("wrong logical channel: %v", m)
				}
				return stop
			}, subscription)
		}()
		ticker := time.NewTicker(10 * time.Millisecond)
		published := false
		for !published {
			select {
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			case <-ticker.C:
				n, e := c.Publish(ctx, "channel", "data")
				if e != nil {
					t.Fatal(e)
				}
				published = n > 0
			}
		}
		ticker.Stop()
		if e = <-done; !errors.Is(e, stop) {
			t.Fatal(e)
		}
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ok, _, _, e := c.RateLimit(ctx, "rate", 5, time.Minute)
			if e != nil {
				t.Error(e)
			}
			if ok {
				won.Add(1)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 5 {
		t.Fatal(won.Load())
	}
	old, e := c.Acquire(ctx, "lease", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.Delete(ctx, "lease"); e != nil {
		t.Fatal(e)
	}
	current, e := c.Acquire(ctx, "lease", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if e = old.Release(ctx); !errors.Is(e, ErrOwnership) {
		t.Fatal("stale owner removed successor", e)
	}
	if ok, e := current.Owned(ctx); e != nil || !ok {
		t.Fatal(ok, e)
	}
	if e = current.Release(ctx); e != nil {
		t.Fatal(e)
	}
}
