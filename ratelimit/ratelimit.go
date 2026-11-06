// Package ratelimit implements concurrency-safe in-memory token buckets.
package ratelimit

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Decision contains admission and retry-after metadata.
type Decision struct {
	Allowed    bool
	Remaining  int
	RetryAfter time.Duration
	Limit      int
}

// Limiter evaluates a context-aware rate policy.
type Limiter interface {
	Allow(context.Context, string) (Decision, error)
}
type bucket struct {
	tokens     float64
	last, seen time.Time
}

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Rate           float64
	Burst, MaxKeys int
	IdleTTL        time.Duration
	Now            func() time.Time
}

// Memory provides bounded process-local coordination.
type Memory struct {
	mu   sync.Mutex
	c    Config
	keys map[string]bucket
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(c Config) (*Memory, error) {
	if c.Rate <= 0 || math.IsInf(c.Rate, 0) || math.IsNaN(c.Rate) || c.Burst < 1 || c.MaxKeys < 1 || c.IdleTTL <= 0 {
		return nil, errors.New("ratelimit: invalid configuration")
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	return &Memory{c: c, keys: map[string]bucket{}}, nil
}

// Allow evaluates the configured rate policy for a key.
func (m *Memory) Allow(ctx context.Context, key string) (Decision, error) {
	if e := ctx.Err(); e != nil {
		return Decision{}, e
	}
	if key == "" {
		return Decision{}, errors.New("ratelimit: empty key")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.c.Now()
	b, ok := m.keys[key]
	if !ok {
		if len(m.keys) >= m.c.MaxKeys {
			for k, v := range m.keys {
				if now.Sub(v.seen) >= m.c.IdleTTL && v.tokens+max(0, now.Sub(v.last).Seconds())*m.c.Rate >= float64(m.c.Burst) {
					delete(m.keys, k)
				}
			}
		}
		if len(m.keys) >= m.c.MaxKeys {
			return Decision{}, errors.New("ratelimit: key capacity reached")
		}
		b = bucket{tokens: float64(m.c.Burst), last: now}
	}
	elapsed := max(0, now.Sub(b.last).Seconds())
	b.tokens = min(float64(m.c.Burst), b.tokens+elapsed*m.c.Rate)
	b.last = now
	b.seen = now
	d := Decision{Limit: m.c.Burst}
	if b.tokens >= 1 {
		b.tokens--
		d.Allowed = true
	} else {
		wait := math.Ceil((1 - b.tokens) / m.c.Rate * float64(time.Second))
		if wait >= float64(math.MaxInt64) {
			d.RetryAfter = time.Duration(math.MaxInt64)
		} else {
			d.RetryAfter = time.Duration(wait)
		}
	}
	d.Remaining = int(b.tokens)
	m.keys[key] = b
	return d, nil
}

// Middleware returns context-aware HTTP admission or coordination middleware.
func Middleware(l Limiter, key func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			d, e := l.Allow(r.Context(), key(r))
			if e != nil {
				http.Error(w, "Rate limiter unavailable", http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("RateLimit-Limit", strconv.Itoa(d.Limit))
			w.Header().Set("RateLimit-Remaining", strconv.Itoa(d.Remaining))
			if !d.Allowed {
				w.Header().Set("Retry-After", strconv.Itoa(max(1, int(math.Ceil(d.RetryAfter.Seconds())))))
				http.Error(w, "Too many requests", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
