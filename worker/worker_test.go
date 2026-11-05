package worker

import (
	"context"
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
