package clock

import (
	"context"
	"testing"
	"time"
)

func TestManual(t *testing.T) {
	m := NewManual(time.Unix(0, 0))
	a := m.NewTimer(time.Second)
	m.Advance(time.Second)
	select {
	case at := <-a.C():
		if at != m.Now() {
			t.Fatal(at)
		}
	default:
		t.Fatal("did not fire")
	}
	ctx, c := context.WithCancel(context.Background())
	c()
	if Wait(ctx, m, time.Hour) != context.Canceled {
		t.Fatal("cancel")
	}
}
