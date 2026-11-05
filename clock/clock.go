// Package clock supplies injectable clocks and cancellable waiting.
package clock

import (
	"context"
	"sync"
	"time"
)

// Timer exposes timer delivery and cancellation.
type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

// Clock abstracts current time and owned timers.
type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

// Real uses the system UTC clock and timers.
type Real struct{}

// Now returns the current UTC time.
func (Real) Now() time.Time { return time.Now().UTC() }

type realTimer struct{ *time.Timer }

// C returns the timer delivery channel.
func (t realTimer) C() <-chan time.Time { return t.Timer.C }

// NewTimer creates an owned timer.
func (Real) NewTimer(d time.Duration) Timer { return realTimer{time.NewTimer(d)} }

// Wait waits using an injected clock, stopping on context cancellation.
func Wait(ctx context.Context, c Clock, d time.Duration) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if c == nil {
		c = Real{}
	}
	t := c.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C():
		return ctx.Err()
	}
}

// Manual advances only when Advance is called; timers have buffered channels.
type Manual struct {
	mu     sync.Mutex
	now    time.Time
	timers map[*manualTimer]time.Time
}
type manualTimer struct {
	owner  *Manual
	ch     chan time.Time
	active bool
}

// NewManual creates a deterministic manual clock.
func NewManual(now time.Time) *Manual {
	return &Manual{now: now.UTC(), timers: map[*manualTimer]time.Time{}}
}

// Now returns the current UTC time.
func (m *Manual) Now() time.Time { m.mu.Lock(); defer m.mu.Unlock(); return m.now }

// NewTimer creates an owned timer.
func (m *Manual) NewTimer(d time.Duration) Timer {
	m.mu.Lock()
	defer m.mu.Unlock()
	t := &manualTimer{m, make(chan time.Time, 1), true}
	if d <= 0 {
		t.active = false
		t.ch <- m.now
	} else {
		m.timers[t] = m.now.Add(d)
	}
	return t
}

// C returns the timer delivery channel.
func (t *manualTimer) C() <-chan time.Time { return t.ch }

// Stop stops owned work in documented cleanup order.
func (t *manualTimer) Stop() bool {
	m := t.owner
	m.mu.Lock()
	defer m.mu.Unlock()
	was := t.active
	t.active = false
	delete(m.timers, t)
	return was
}

// Advance advances the manual clock and fires due timers.
func (m *Manual) Advance(d time.Duration) {
	if d < 0 {
		panic("clock: cannot go backwards")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.now = m.now.Add(d)
	for t, at := range m.timers {
		if !at.After(m.now) {
			t.active = false
			t.ch <- at
			delete(m.timers, t)
		}
	}
}
