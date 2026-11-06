package consumerlife

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestRepeatedExpiredShutdownKeepsOneCleanup(t *testing.T) {
	var session Session
	_, finish, err := session.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	before := runtime.NumGoroutine()
	var cleanups atomic.Int32
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for range 1000 {
		if err := session.Shutdown(canceled, func() error { cleanups.Add(1); return nil }); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if cleanups.Load() != 0 {
		t.Fatal("native cleanup ran before active loop finished")
	}
	// At most the single session cleanup waiter may outlive expired callers.
	if delta := runtime.NumGoroutine() - before; delta > 1 {
		t.Fatalf("expired shutdown calls created %d waiters", delta)
	}
	finish()
	ctx, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if err := session.Shutdown(ctx, func() error { t.Fatal("cleanup callback replaced"); return nil }); err != nil {
		t.Fatal(err)
	}
	if cleanups.Load() != 1 {
		t.Fatalf("native cleanup count=%d", cleanups.Load())
	}
}
