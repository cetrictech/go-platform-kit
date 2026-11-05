package resilience

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestOpen(t *testing.T) {
	b, e := New(Config{FailureThreshold: 1, Recovery: time.Hour, HalfOpenProbes: 1})
	if e != nil {
		t.Fatal(e)
	}
	_ = b.Do(context.Background(), func(context.Context) error { return errors.New("failure") })
	if b.Do(context.Background(), func(context.Context) error { return nil }) != ErrOpen {
		t.Fatal("not open")
	}
	h, _ := NewBulkhead(1)
	ctx, c := context.WithCancel(context.Background())
	c()
	if h.Do(ctx, func(context.Context) error { t.Fatal("called"); return nil }) != context.Canceled {
		t.Fatal("cancel")
	}
}

func TestHalfOpen(t *testing.T) {
	b, _ := New(Config{FailureThreshold: 1, HalfOpenProbes: 1, Recovery: time.Hour})
	ctx := context.Background()
	_ = b.Do(ctx, func(context.Context) error { return errors.New("failure") })
	b.mu.Lock()
	b.opened = time.Now().Add(-2 * time.Hour)
	b.mu.Unlock()
	if e := b.Do(ctx, func(context.Context) error { return nil }); e != nil || b.Stats().State != Closed {
		t.Fatal(b.Stats(), e)
	}
}
