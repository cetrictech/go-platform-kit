package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestAtomic(t *testing.T) {
	now := time.Unix(0, 0)
	l, _ := New(Config{Rate: 1, Burst: 1, MaxKeys: 1, IdleTTL: time.Minute, Now: func() time.Time { return now }})
	d, e := l.Allow(context.Background(), "x")
	if e != nil || !d.Allowed {
		t.Fatal(d, e)
	}
	d, _ = l.Allow(context.Background(), "x")
	if d.Allowed || d.RetryAfter != time.Second {
		t.Fatal(d)
	}
	now = now.Add(time.Second)
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = l.Allow(context.Background(), "x") }()
	}
	wg.Wait()
}

func TestTinyRate(t *testing.T) {
	l, e := New(Config{Rate: 1e-300, Burst: 1, MaxKeys: 1, IdleTTL: time.Hour})
	if e != nil {
		t.Fatal(e)
	}
	_, _ = l.Allow(context.Background(), "key")
	d, e := l.Allow(context.Background(), "key")
	if e != nil || d.Allowed || d.RetryAfter <= 0 {
		t.Fatal(d, e)
	}
}

func TestEvictionCannotResetBudget(t *testing.T) {
	now := time.Unix(100, 0)
	l, e := New(Config{Rate: 0.01, Burst: 1, MaxKeys: 1, IdleTTL: time.Millisecond, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	_, _ = l.Allow(context.Background(), "a")
	now = now.Add(time.Second)
	if _, e = l.Allow(context.Background(), "b"); e == nil {
		t.Fatal("unfilled bucket evicted")
	}
	d, e := l.Allow(context.Background(), "a")
	if e != nil || d.Allowed {
		t.Fatal(d, e)
	}
}
