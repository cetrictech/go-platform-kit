// Package health implements independent liveness, readiness and startup registries with bounded dependency checking.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"net/http"
	"sync"
	"time"
)

// Check must honor ctx. A misbehaving callback is isolated to at most one active invocation per registered check.
type Check func(context.Context) error

// Result contains only safe status metadata; raw dependency errors are never exposed by Handler.
type Result struct {
	Healthy  bool          `json:"healthy"`
	Duration time.Duration `json:"duration_ns"`
	Reason   string        `json:"reason,omitempty"`
}

// Report aggregates configured dependency checks. Use an empty registry for dependency-independent liveness.
type Report struct {
	Healthy bool              `json:"healthy"`
	Checks  map[string]Result `json:"checks"`
}
type checkState struct {
	check   Check
	running chan struct{}
}

// Registry supports concurrent Run and Register. Configure Timeout/OnResult before serving concurrent requests.
type Registry struct {
	mu       sync.RWMutex
	checks   map[string]*checkState
	Timeout  time.Duration
	OnResult func(context.Context, string, Result, error)
}

// Register rejects duplicate/empty dependency names.
func (r *Registry) Register(name string, c Check) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" || c == nil {
		return errors.New("health: name and check required")
	}
	if r.checks == nil {
		r.checks = map[string]*checkState{}
	}
	if _, exists := r.checks[name]; exists {
		return errors.New("health: duplicate check")
	}
	r.checks[name] = &checkState{c, make(chan struct{}, 1)}
	return nil
}

// Run executes checks in parallel, returning by each check's timeout even if a callback ignores cancellation.
// A timed-out callback retains its one invocation slot until it exits; later checks report busy instead of creating runaway goroutines.
func (r *Registry) Run(ctx context.Context) Report {
	r.mu.RLock()
	checks := make(map[string]*checkState, len(r.checks))
	for n, c := range r.checks {
		checks[n] = c
	}
	timeout, onResult := r.Timeout, r.OnResult
	r.mu.RUnlock()
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	out := Report{Healthy: true, Checks: map[string]Result{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, check := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			c, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			reason, e := check.run(c)
			result := Result{Healthy: e == nil, Duration: time.Since(start), Reason: reason}
			if onResult != nil {
				onResult(ctx, name, result, e)
			}
			mu.Lock()
			defer mu.Unlock()
			out.Checks[name] = result
			out.Healthy = out.Healthy && result.Healthy
		}()
	}
	wg.Wait()
	return out
}
func (s *checkState) run(ctx context.Context) (string, error) {
	if e := ctx.Err(); e != nil {
		return "canceled", e
	}
	select {
	case s.running <- struct{}{}:
	default:
		return "busy", errors.New("health: prior check still running")
	}
	type outcome struct {
		err    error
		reason string
	}
	done := make(chan outcome, 1)
	go func() { defer func() { <-s.running }(); reason, err := run(ctx, s.check); done <- outcome{err, reason} }()
	select {
	case result := <-done:
		if ctx.Err() != nil {
			return "timeout", ctx.Err()
		}
		return result.reason, result.err
	case <-ctx.Done():
		reason := "canceled"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			reason = "timeout"
		}
		return reason, ctx.Err()
	}
}
func run(ctx context.Context, check Check) (reason string, err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("health: check panic")
			reason = "panic"
		}
	}()
	err = check(ctx)
	if err != nil {
		reason = "failed"
	}
	return
}

// Handler returns HTTP 200/503 with bounded, diagnostic-free JSON status.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		report := r.Run(q.Context())
		w.Header().Set("Content-Type", "application/json")
		if !report.Healthy {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
}

// Pinger is satisfied by the SQL and Redis SDK clients.
type Pinger interface{ Ping(context.Context) error }

// Ping adapts a dependency client without importing its optional driver.
func Ping(p Pinger) Check {
	return func(ctx context.Context) error {
		if p == nil {
			return errors.New("health: ping client required")
		}
		return p.Ping(ctx)
	}
}

// Kafka verifies actual broker metadata availability through either administration adapter.
func Kafka(admin kafka.Admin) Check {
	return func(ctx context.Context) error {
		if admin == nil {
			return errors.New("health: Kafka admin required")
		}
		_, e := admin.ListTopics(ctx)
		return e
	}
}
