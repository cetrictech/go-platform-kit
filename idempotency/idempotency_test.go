package idempotency

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestConcurrent(t *testing.T) {
	s, _ := NewMemory(100)
	ctx := context.Background()
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Begin(ctx, "key", "body", time.Minute)
			if e == nil {
				winners.Add(1)
				if e = s.Complete(ctx, "key", r.Token, 201, time.Hour); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatal(winners.Load())
	}
	if _, e := s.Begin(ctx, "key", "changed", time.Minute); e != ErrMismatch {
		t.Fatal(e)
	}
	if e := s.Complete(ctx, "key", "wrong", 200, time.Hour); e != ErrOwnership {
		t.Fatal(e)
	}
}

func TestExpiredLeaseOwnership(t *testing.T) {
	s, _ := NewMemory(1)
	now := time.Unix(100, 0)
	s.Now = func() time.Time { return now }
	ctx := context.Background()
	old, e := s.Begin(ctx, "key", "body", time.Second)
	if e != nil {
		t.Fatal(e)
	}
	now = now.Add(2 * time.Second)
	current, e := s.Begin(ctx, "key", "body", time.Minute)
	if e != nil || current.Token == old.Token {
		t.Fatal(current, e)
	}
	if e = s.Complete(ctx, "key", old.Token, 200, time.Minute); e != ErrOwnership {
		t.Fatal(e)
	}
	if e = s.Renew(ctx, "key", current.Token, time.Minute); e != nil {
		t.Fatal(e)
	}
}
