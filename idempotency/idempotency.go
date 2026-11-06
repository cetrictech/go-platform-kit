// Package idempotency coordinates request attempts using expiring ownership leases.
package idempotency

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/security"
	"sync"
	"time"
)

var ErrMismatch = errors.New("idempotency: request fingerprint mismatch")
var ErrBusy = errors.New("idempotency: request processing")
var ErrCompleted = errors.New("idempotency: request already completed")
var ErrOwnership = errors.New("idempotency: ownership lost")

// State identifies a processing or circuit-breaker state.
type State string

const (
	Processing State = "processing"
	Completed  State = "completed"
	Failed     State = "failed"
)

// Record contains persisted processing or event metadata.
type Record struct {
	Fingerprint string    `json:"fingerprint"`
	Token       string    `json:"token"`
	State       State     `json:"state"`
	Status      int       `json:"status"`
	Expires     time.Time `json:"expires"`
}

// Store defines atomic lease-based idempotency transitions.
type Store interface {
	Begin(context.Context, string, string, time.Duration) (Record, error)
	Complete(context.Context, string, string, int, time.Duration) error
	Fail(context.Context, string, string, time.Duration) error
	Renew(context.Context, string, string, time.Duration) error
}

// Memory is bounded. Expired processing leases may be reclaimed; operations must stop when renewal fails.
type Memory struct {
	mu      sync.Mutex
	records map[string]Record
	MaxKeys int
	Now     func() time.Time
}

// NewMemory creates a bounded process-local idempotency store.
func NewMemory(maxKeys int) (*Memory, error) {
	if maxKeys < 1 {
		return nil, errors.New("idempotency: capacity must be positive")
	}
	return &Memory{records: map[string]Record{}, MaxKeys: maxKeys, Now: time.Now}, nil
}

// Begin atomically attempts to acquire a fingerprint-bound processing lease.
func (m *Memory) Begin(ctx context.Context, key, fingerprint string, lease time.Duration) (Record, error) {
	if e := ctx.Err(); e != nil {
		return Record{}, e
	}
	if key == "" || fingerprint == "" || lease <= 0 {
		return Record{}, errors.New("idempotency: key, fingerprint and positive lease required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.Now()
	r, ok := m.records[key]
	if ok && now.Before(r.Expires) {
		if r.Fingerprint != fingerprint {
			return r, ErrMismatch
		}
		if r.State == Completed {
			return r, ErrCompleted
		}
		if r.State == Processing {
			return r, ErrBusy
		}
	}
	if !ok && len(m.records) >= m.MaxKeys {
		for k, v := range m.records {
			if !now.Before(v.Expires) {
				delete(m.records, k)
			}
		}
		if len(m.records) >= m.MaxKeys {
			return Record{}, errors.New("idempotency: capacity reached")
		}
	}
	token, e := security.Token(32)
	if e != nil {
		return Record{}, e
	}
	r = Record{Fingerprint: fingerprint, Token: token, State: Processing, Expires: now.Add(lease)}
	m.records[key] = r
	return r, nil
}
func (m *Memory) change(ctx context.Context, key, token string, state State, status int, ttl time.Duration) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if ttl <= 0 {
		return errors.New("idempotency: positive TTL required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.records[key]
	if !ok || r.Token != token || r.State != Processing || !m.Now().Before(r.Expires) {
		return ErrOwnership
	}
	r.State = state
	r.Status = status
	r.Expires = m.Now().Add(ttl)
	m.records[key] = r
	return nil
}

// Complete records successful processing only for the current ownership token.
func (m *Memory) Complete(ctx context.Context, key, token string, status int, ttl time.Duration) error {
	return m.change(ctx, key, token, Completed, status, ttl)
}

// Fail records failed processing only for the current ownership token.
func (m *Memory) Fail(ctx context.Context, key, token string, ttl time.Duration) error {
	return m.change(ctx, key, token, Failed, 0, ttl)
}

// Renew extends a current processing lease.
func (m *Memory) Renew(ctx context.Context, key, token string, ttl time.Duration) error {
	return m.change(ctx, key, token, Processing, 0, ttl)
}
