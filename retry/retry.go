// Package retry retries explicitly classified operations with bounded backoff.
package retry

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/clock"
	"math/rand/v2"
	"time"
)

// Policy bounds retries and requires explicit idempotency and error classification.
type Policy struct {
	MaxAttempts                     int
	Delay, MaxDelay, MaxElapsed     time.Duration
	Exponential, Jitter, Idempotent bool
	Retryable                       func(error) bool
	OnRetry                         func(int, error, time.Duration)
	Clock                           clock.Clock
}

// Do executes the supplied operation with this component's context and policy.
func Do(ctx context.Context, p Policy, operation func(context.Context) error) error {
	if operation == nil {
		return errors.New("retry: nil operation")
	}
	if p.MaxAttempts < 1 || p.Delay < 0 || p.MaxDelay < 0 || p.MaxElapsed < 0 {
		return errors.New("retry: invalid policy")
	}
	c := p.Clock
	if c == nil {
		c = clock.Real{}
	}
	if p.MaxElapsed > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.MaxElapsed)
		defer cancel()
	}
	start := c.Now()
	delay := p.Delay
	for i := 1; i <= p.MaxAttempts; i++ {
		if e := ctx.Err(); e != nil {
			return e
		}
		e := operation(ctx)
		if e == nil {
			return nil
		}
		if i == p.MaxAttempts || !p.Idempotent || p.Retryable == nil || !p.Retryable(e) {
			return e
		}
		d := delay
		if p.MaxDelay > 0 && d > p.MaxDelay {
			d = p.MaxDelay
		}
		if p.Jitter && d > 0 {
			d = time.Duration(rand.Int64N(int64(d)))
		}
		if p.MaxElapsed > 0 && c.Now().Sub(start)+d >= p.MaxElapsed {
			return fmt.Errorf("retry elapsed: %w", e)
		}
		if p.OnRetry != nil {
			p.OnRetry(i, e, d)
		}
		if e = clock.Wait(ctx, c, d); e != nil {
			return e
		}
		if p.Exponential {
			if delay >= time.Duration(1<<62) {
				delay = time.Duration(1<<63 - 1)
			} else {
				delay *= 2
			}
		}
	}
	return nil
}
