package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestCancellationReleasesQueuedJobsAndBoundsAdmission(t *testing.T) {
	var executed, reported atomic.Int64
	pool, err := New(context.Background(), 1, 2, func(err error) {
		if !errors.Is(err, context.Canceled) {
			t.Errorf("unexpected failure: %v", err)
		}
		reported.Add(1)
	})
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	if err := pool.Submit(context.Background(), func(ctx context.Context) error { close(started); <-ctx.Done(); return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	<-started
	queued := func(context.Context) error { executed.Add(1); return nil }
	if err := pool.Register("large-payload-job", queued); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := pool.SubmitNamed(context.Background(), "large-payload-job"); err != nil {
			t.Fatal(err)
		}
	}
	bounded, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = pool.Submit(bounded, queued)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("full queue accepted unbounded work: %v", err)
	}
	pool.Cancel()
	cleanup, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := pool.Shutdown(cleanup); err != nil {
		t.Fatal(err)
	}
	stats := pool.Stats()
	if stats.Queued != 0 || stats.Active != 0 || stats.Completed != 3 || stats.Failed != 3 || stats.Submitted != 3 {
		t.Fatalf("canceled work retained or unaccounted: %+v", stats)
	}
	if executed.Load() != 0 || reported.Load() != 3 {
		t.Fatalf("queued callbacks executed=%d cancellations reported=%d", executed.Load(), reported.Load())
	}
	if len(pool.registered) != 0 {
		t.Fatal("closed pool retained registered job closures")
	}
	if err := pool.Submit(context.Background(), queued); err == nil {
		t.Fatal("closed pool admitted work")
	}
}

func TestParentCancellationClosesAdmissionWithoutShutdownWaiters(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	pool, err := New(parent, 1, 1, func(error) {})
	if err != nil {
		t.Fatal(err)
	}
	entered := make(chan struct{})
	if err := pool.Submit(context.Background(), func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() }); err != nil {
		t.Fatal(err)
	}
	<-entered
	if err := pool.Submit(context.Background(), func(context.Context) error { t.Error("canceled queued callback executed"); return nil }); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case <-pool.finished:
	case <-time.After(time.Second):
		t.Fatal("parent cancellation left owned work running")
	}
	if err := pool.Register("after-cancel", func(context.Context) error { return nil }); err == nil {
		t.Fatal("canceled pool accepted a registered closure")
	}
	if stats := pool.Stats(); stats.Queued != 0 || stats.Completed != 2 || stats.Failed != 2 {
		t.Fatalf("parent cancellation retained work: %+v", stats)
	}
	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
}
