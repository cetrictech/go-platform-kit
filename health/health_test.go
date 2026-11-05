package health

import (
	"context"
	"errors"
	"testing"
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
