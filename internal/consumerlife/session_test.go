package consumerlife

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestShutdownCancelsAndWaits(t *testing.T) {
	var life Session
	ctx, finish, e := life.Begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	var closed atomic.Int32
	done := make(chan error, 1)
	go func() { done <- life.Shutdown(context.Background(), func() error { closed.Add(1); return nil }) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("not canceled")
	}
	if closed.Load() != 0 {
		t.Fatal("native closed before handler drained")
	}
	finish()
	if e = <-done; e != nil || closed.Load() != 1 {
		t.Fatal(e)
	}
	if e = life.Shutdown(context.Background(), func() error { t.Fatal("cleanup repeated"); return nil }); e != nil {
		t.Fatal(e)
	}
	if _, _, e = life.Begin(context.Background()); e == nil {
		t.Fatal("restart after close")
	}
}
func TestDeadlineAndLaterCleanup(t *testing.T) {
	var life Session
	_, finish, _ := life.Begin(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e := life.Shutdown(ctx, func() error { return errors.New("close failure") }); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
	finish()
	if e := life.Shutdown(context.Background(), func() error { return nil }); e == nil || e.Error() != "close failure" {
		t.Fatal(e)
	}
}
