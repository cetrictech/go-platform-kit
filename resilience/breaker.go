// Package resilience provides circuit breakers and bounded concurrency.
package resilience

import (
	"context"
	"errors"
	"sync"
	"time"
)

var ErrOpen = errors.New("resilience: circuit open")

// State identifies a processing or circuit-breaker state.
type State string

const (
	Closed   State = "closed"
	Open     State = "open"
	HalfOpen State = "half_open"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	FailureThreshold, MinimumRequests, HalfOpenProbes int
	FailureRate                                       float64
	Recovery                                          time.Duration
	OnChange                                          func(State, State)
	Failure                                           func(error) bool
}

// Stats contains a snapshot of operational counters.
type Stats struct {
	State                        State
	Requests, Failures, InFlight int
	Generation                   uint64
}

// Breaker coordinates concurrency-safe circuit-breaker state.
type Breaker struct {
	mu           sync.Mutex
	c            Config
	s            Stats
	opened       time.Time
	probeSuccess int
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(c Config) (*Breaker, error) {
	if c.FailureThreshold < 1 || c.Recovery <= 0 || c.HalfOpenProbes < 1 || c.FailureRate < 0 || c.FailureRate > 1 {
		return nil, errors.New("resilience: invalid config")
	}
	return &Breaker{c: c, s: Stats{State: Closed}}, nil
}
func (b *Breaker) transition(s State) (State, State) {
	old := b.s.State
	b.s = Stats{State: s, Generation: b.s.Generation + 1}
	b.probeSuccess = 0
	if s == Open {
		b.opened = time.Now()
	}
	return old, s
}
func (b *Breaker) notify(old, next State) {
	if old != next && b.c.OnChange != nil {
		b.c.OnChange(old, next)
	}
}

// Do executes the supplied operation with this component's context and policy.
func (b *Breaker) Do(ctx context.Context, fn func(context.Context) error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	b.mu.Lock()
	old, next := b.s.State, b.s.State
	if b.s.State == Open && time.Since(b.opened) >= b.c.Recovery {
		old, next = b.transition(HalfOpen)
	}
	if b.s.State == Open || (b.s.State == HalfOpen && b.s.InFlight >= b.c.HalfOpenProbes) {
		b.mu.Unlock()
		return ErrOpen
	}
	gen := b.s.Generation
	b.s.InFlight++
	b.mu.Unlock()
	b.notify(old, next)
	var e error
	defer func() {
		if p := recover(); p != nil {
			b.record(gen, errors.New("operation panic"))
			panic(p)
		}
	}()
	e = fn(ctx)
	b.record(gen, e)
	return e
}
func (b *Breaker) record(gen uint64, e error) {
	failed := e != nil
	if b.c.Failure != nil {
		failed = b.c.Failure(e)
	}
	b.mu.Lock()
	if gen != b.s.Generation {
		b.mu.Unlock()
		return
	}
	b.s.InFlight--
	b.s.Requests++
	if failed {
		b.s.Failures++
	}
	old, next := b.s.State, b.s.State
	if b.s.State == HalfOpen {
		if failed {
			old, next = b.transition(Open)
		} else {
			b.probeSuccess++
			if b.probeSuccess >= b.c.HalfOpenProbes {
				old, next = b.transition(Closed)
			}
		}
	} else if b.s.Failures >= b.c.FailureThreshold || (b.c.FailureRate > 0 && b.s.Requests >= b.c.MinimumRequests && float64(b.s.Failures)/float64(b.s.Requests) >= b.c.FailureRate) {
		old, next = b.transition(Open)
	}
	b.mu.Unlock()
	b.notify(old, next)
}

// Stats returns a concurrency-safe statistics snapshot.
func (b *Breaker) Stats() Stats { b.mu.Lock(); defer b.mu.Unlock(); return b.s }

// Bulkhead bounds concurrent operation execution.
type Bulkhead struct{ slots chan struct{} }

// NewBulkhead constructs a positive-capacity concurrency bound.
func NewBulkhead(n int) (*Bulkhead, error) {
	if n < 1 {
		return nil, errors.New("resilience: capacity must be positive")
	}
	return &Bulkhead{make(chan struct{}, n)}, nil
}

// Do executes the supplied operation with this component's context and policy.
func (b *Bulkhead) Do(ctx context.Context, fn func(context.Context) error) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	select {
	case b.slots <- struct{}{}:
		defer func() { <-b.slots }()
		return fn(ctx)
	case <-ctx.Done():
		return ctx.Err()
	}
}
