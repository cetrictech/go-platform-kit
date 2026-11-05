// Package health runs explicit dependency checks. Liveness should use an empty registry.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"
)

// Check performs one context-aware dependency check.
type Check func(context.Context) error

// Result contains operation status without exposing a native driver.
type Result struct {
	Healthy  bool          `json:"healthy"`
	Duration time.Duration `json:"duration_ns"`
}

// Report aggregates safe dependency health results.
type Report struct {
	Healthy bool              `json:"healthy"`
	Checks  map[string]Result `json:"checks"`
}

// Registry holds named dependency checks and permits concurrent checks.
type Registry struct {
	mu      sync.RWMutex
	checks  map[string]Check
	Timeout time.Duration
}

// Register registers a named hook or check before use.
func (r *Registry) Register(name string, c Check) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if name == "" || c == nil {
		return errors.New("health: name and check required")
	}
	if r.checks == nil {
		r.checks = map[string]Check{}
	}
	if _, ok := r.checks[name]; ok {
		return errors.New("health: duplicate check")
	}
	r.checks[name] = c
	return nil
}

// Run executes registered checks and aggregates safe health results.
func (r *Registry) Run(ctx context.Context) Report {
	r.mu.RLock()
	checks := make(map[string]Check, len(r.checks))
	for n, c := range r.checks {
		checks[n] = c
	}
	r.mu.RUnlock()
	out := Report{true, map[string]Result{}}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for name, check := range checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			start := time.Now()
			timeout := r.Timeout
			if timeout <= 0 {
				timeout = 2 * time.Second
			}
			c, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			e := run(c, check)
			mu.Lock()
			defer mu.Unlock()
			out.Checks[name] = Result{e == nil, time.Since(start)}
			out.Healthy = out.Healthy && e == nil
		}()
	}
	wg.Wait()
	return out
}
func run(ctx context.Context, c Check) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("health: check panic")
		}
	}()
	return c(ctx)
}

// Handler returns a native HTTP handler.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		report := r.Run(q.Context())
		w.Header().Set("Content-Type", "application/json")
		if !report.Healthy {
			w.WriteHeader(503)
		}
		_ = json.NewEncoder(w).Encode(report)
	})
}
