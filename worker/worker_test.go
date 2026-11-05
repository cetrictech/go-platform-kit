package worker

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/retry"
	"sync/atomic"
	"testing"
	"time"
)

func TestDrain(t *testing.T) {
	var n atomic.Int32
	p, e := New(context.Background(), 2, 10, func(e error) { t.Error(e) })
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 10; i++ {
		if e = p.Submit(context.Background(), func(context.Context) error { n.Add(1); return nil }); e != nil {
			t.Fatal(e)
		}
	}
	ctx, c := context.WithTimeout(context.Background(), time.Second)
	defer c()
	if e = p.Shutdown(ctx); e != nil || n.Load() != 10 {
		t.Fatal(n.Load(), e)
	}
	if p.Submit(ctx, func(context.Context) error { return nil }) == nil {
		t.Fatal("submit after close")
	}
}

func TestRegisteredRetryContextAndMetrics(t *testing.T) {
	var attempts atomic.Int32
	p, e := NewWithConfig(context.Background(), Config{Concurrency: 1, Queue: 2, Report: func(e error) { t.Error(e) }, Retry: &retry.Policy{MaxAttempts: 2, Idempotent: true, Retryable: func(error) bool { return true }}})
	if e != nil {
		t.Fatal(e)
	}
	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "trace")
	if e = p.Register("job", func(ctx context.Context) error {
		if ctx.Value(key{}) != "trace" {
			t.Error("submission context lost")
		}
		if attempts.Add(1) == 1 {
			return errors.New("transient")
		}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	if e = p.SubmitNamed(ctx, "job"); e != nil {
		t.Fatal(e)
	}
	if e = p.Shutdown(context.Background()); e != nil {
		t.Fatal(e)
	}
	stats := p.Stats()
	if attempts.Load() != 2 || stats.Submitted != 1 || stats.Succeeded != 1 || stats.Completed != 1 || stats.Active != 0 {
		t.Fatal(stats, attempts.Load())
	}
}
