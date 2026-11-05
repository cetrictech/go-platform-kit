package retry

import (
	"context"
	"errors"
	"testing"
)

func TestAttempts(t *testing.T) {
	n := 0
	e := Do(context.Background(), Policy{MaxAttempts: 3, Idempotent: true, Retryable: func(error) bool { return true }}, func(context.Context) error {
		n++
		if n < 3 {
			return errors.New("retry")
		}
		return nil
	})
	if e != nil || n != 3 {
		t.Fatal(n, e)
	}
	n = 0
	_ = Do(context.Background(), Policy{MaxAttempts: 3}, func(context.Context) error { n++; return errors.New("stop") })
	if n != 1 {
		t.Fatal(n)
	}
}
