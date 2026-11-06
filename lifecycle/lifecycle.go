// Package lifecycle manages explicit startup and reverse-order shutdown.
package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"time"
)

// Hook describes an explicit lifecycle action or instrumentation callback.
type Hook struct {
	Name  string
	Start func(context.Context) error
	Stop  func(context.Context) error
}

// Manager serializes lifecycle operations. Hooks must respect their context.
type Manager struct {
	mu      sync.Mutex
	hooks   []Hook
	started []Hook
	running bool
	Timeout time.Duration
}

// Register registers a named hook or check before use.
func (m *Manager) Register(h Hook) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running || len(m.started) > 0 {
		return errors.New("lifecycle: already started")
	}
	if h.Name == "" {
		return errors.New("lifecycle: empty hook name")
	}
	for _, x := range m.hooks {
		if x.Name == h.Name {
			return errors.New("lifecycle: duplicate hook")
		}
	}
	m.hooks = append(m.hooks, h)
	return nil
}

// Start starts registered work with the supplied context.
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return errors.New("lifecycle: already running")
	}
	for _, h := range m.hooks {
		if e := ctx.Err(); e != nil {
			return errors.Join(e, m.stop(context.Background()))
		}
		if h.Start != nil {
			if e := h.Start(ctx); e != nil {
				return errors.Join(fmt.Errorf("start %s: %w", h.Name, e), m.stop(context.Background()))
			}
		}
		m.started = append(m.started, h)
	}
	m.running = true
	return nil
}

// Stop stops owned work in documented cleanup order.
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stop(ctx)
}
func (m *Manager) stop(ctx context.Context) error {
	timeout := m.Timeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	var errs []error
	for i := len(m.started) - 1; i >= 0; i-- {
		h := m.started[i]
		if h.Stop != nil {
			if e := h.Stop(ctx); e != nil {
				errs = append(errs, fmt.Errorf("stop %s: %w", h.Name, e))
			}
		}
	}
	m.started = nil
	m.running = false
	return errors.Join(errs...)
}

// NotifyContext explicitly opts into OS signal handling. The caller must invoke stop.
func NotifyContext(ctx context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, signals...)
}
