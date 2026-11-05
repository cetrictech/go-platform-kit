package health

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestHealth(t *testing.T) {
	r := Registry{}
	if !r.Run(context.Background()).Healthy {
		t.Fatal("liveness")
	}
	_ = r.Register("db", func(context.Context) error { return errors.New("offline") })
	if r.Run(context.Background()).Healthy {
		t.Fatal("readiness")
	}
}

func TestTimeoutDoesNotCreateUnboundedChecks(t *testing.T) {
	r := Registry{Timeout: 10 * time.Millisecond}
	block := make(chan struct{})
	started := make(chan struct{})
	var calls atomic.Int32
	_ = r.Register("hung", func(context.Context) error { calls.Add(1); close(started); <-block; return nil })
	done := make(chan Report, 1)
	go func() { done <- r.Run(context.Background()) }()
	<-started
	report := <-done
	if report.Healthy || report.Checks["hung"].Reason != "timeout" {
		t.Fatal(report)
	}
	for i := 0; i < 10; i++ {
		if report := r.Run(context.Background()); report.Healthy || report.Checks["hung"].Reason != "busy" {
			t.Fatal(report)
		}
	}
	if calls.Load() != 1 {
		t.Fatal(calls.Load())
	}
	close(block)
}
