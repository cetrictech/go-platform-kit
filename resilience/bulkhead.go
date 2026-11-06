package resilience

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/instrument"
	"sync/atomic"
)

var ErrBulkheadFull = errors.New("resilience: bulkhead queue full")

// BulkheadConfig bounds both executing operations and admitted waiters. Zero Queue means no waiting.
type BulkheadConfig struct {
	Concurrency, Queue int
	Observer           instrument.Hook
}

// BulkheadStats exposes cumulative completion/rejection and current execution/queue counters.
type BulkheadStats struct{ Active, Queued, Completed, Rejected int64 }

// Bulkhead bounds concurrency and memory attributable to queued admissions.
type Bulkhead struct {
	slots, queue                         chan struct{}
	observer                             instrument.Hook
	active, waiting, completed, rejected atomic.Int64
}

// NewBulkhead preserves cancellable waiting with a finite default queue of 1024 operations.
func NewBulkhead(n int) (*Bulkhead, error) {
	return NewBulkheadWithConfig(BulkheadConfig{Concurrency: n, Queue: 1024})
}

// NewBulkheadWithConfig validates and creates an independently usable admission policy.
func NewBulkheadWithConfig(c BulkheadConfig) (*Bulkhead, error) {
	if c.Concurrency < 1 || c.Queue < 0 {
		return nil, errors.New("resilience: invalid bulkhead bounds")
	}
	return &Bulkhead{slots: make(chan struct{}, c.Concurrency), queue: make(chan struct{}, c.Queue), observer: c.Observer}, nil
}

// Do executes an operation after bounded admission. Context cancellation interrupts queued waiting.
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) (result error) {
	if fn == nil {
		return errors.New("resilience: nil operation")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	ctx, done := instrument.Start(ctx, b.observer, "resilience.bulkhead")
	defer func() { done(result) }()
	select {
	case b.slots <- struct{}{}:
	default:
		select {
		case b.queue <- struct{}{}:
			b.waiting.Add(1)
		case <-ctx.Done():
			return ctx.Err()
		default:
			b.rejected.Add(1)
			return ErrBulkheadFull
		}
		select {
		case b.slots <- struct{}{}:
			<-b.queue
			b.waiting.Add(-1)
		case <-ctx.Done():
			<-b.queue
			b.waiting.Add(-1)
			return ctx.Err()
		}
	}
	b.active.Add(1)
	defer func() { <-b.slots; b.active.Add(-1); b.completed.Add(1) }()
	if e := ctx.Err(); e != nil {
		return e
	}
	return fn(ctx)
}

// Stats is a concurrency-safe observational snapshot; current counters may change immediately after return.
func (b *Bulkhead) Stats() BulkheadStats {
	return BulkheadStats{b.active.Load(), b.waiting.Load(), b.completed.Load(), b.rejected.Load()}
}
