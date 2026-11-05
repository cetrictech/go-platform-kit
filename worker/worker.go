// Package worker provides registered, bounded workers with explicit retry, metrics, cancellation and draining.
package worker

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/cetrictech/go-platform-kit/retry"
	"sync"
	"sync/atomic"
)

// Job executes one cancellable operation. It must honor ctx and must not retain caller-owned mutable inputs without synchronization.
type Job func(context.Context) error

// Config controls a pool. Report is mandatory and must not panic. Retry repeats only explicitly idempotent, classified failures.
type Config struct {
	Concurrency, Queue int
	Report             func(error)
	Retry              *retry.Policy
	Observer           instrument.Hook
}

// Stats includes finished jobs (Completed), outcome counts and current active/queued work. Panics counts recovered attempts.
type Stats struct {
	Submitted, Started, Completed, Succeeded, Failed, Panics, Active int64
	Queued                                                           int
}
type counters struct{ submitted, started, completed, succeeded, failed, panics, active atomic.Int64 }
type entry struct {
	ctx context.Context
	job Job
}

// Pool owns worker goroutines and bounded accepted work. Submit contexts control both admission and the individual job lifetime.
type Pool struct {
	jobs       chan entry
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	config     Config
	metrics    counters
	mu         sync.RWMutex
	closed     bool
	closing    chan struct{}
	once       sync.Once
	finished   chan struct{}
	registered map[string]Job
}

// New creates a pool without automatic retries. All failures are delivered to report.
func New(ctx context.Context, concurrency, queue int, report func(error)) (*Pool, error) {
	return NewWithConfig(ctx, Config{Concurrency: concurrency, Queue: queue, Report: report})
}

// NewWithConfig validates all retry/admission settings before starting any owned goroutine.
func NewWithConfig(ctx context.Context, c Config) (*Pool, error) {
	if ctx == nil || c.Concurrency < 1 || c.Queue < 0 || c.Report == nil {
		return nil, errors.New("worker: invalid configuration or missing error reporter")
	}
	if c.Retry != nil {
		copy := *c.Retry
		if copy.MaxAttempts < 1 || copy.Delay < 0 || copy.MaxDelay < 0 || copy.MaxElapsed < 0 {
			return nil, errors.New("worker: invalid retry policy")
		}
		c.Retry = &copy
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Pool{jobs: make(chan entry, c.Queue), ctx: ctx, cancel: cancel, config: c, closing: make(chan struct{}), finished: make(chan struct{}), registered: map[string]Job{}}
	for i := 0; i < c.Concurrency; i++ {
		p.wg.Add(1)
		go p.run()
	}
	go func() { p.wg.Wait(); close(p.finished) }()
	return p, nil
}
func (p *Pool) run() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case item, ok := <-p.jobs:
			if !ok {
				return
			}
			p.execute(item)
		}
	}
}
func (p *Pool) execute(item entry) {
	p.metrics.started.Add(1)
	p.metrics.active.Add(1)
	ctx, cancel := context.WithCancel(p.ctx)
	if deadline, ok := item.ctx.Deadline(); ok {
		cancel()
		ctx, cancel = context.WithDeadline(p.ctx, deadline)
	}
	stop := context.AfterFunc(item.ctx, cancel)
	if item.ctx.Err() != nil {
		cancel()
	}
	ctx = submissionContext{Context: ctx, values: context.WithoutCancel(item.ctx)}
	defer func() { stop(); cancel(); p.metrics.active.Add(-1); p.metrics.completed.Add(1) }()
	ctx, done := instrument.Start(ctx, p.config.Observer, "worker.job")
	operation := func(ctx context.Context) (err error) {
		if e := ctx.Err(); e != nil {
			return e
		}
		defer func() {
			if value := recover(); value != nil {
				p.metrics.panics.Add(1)
				err = fmt.Errorf("worker panic: %v", value)
			}
		}()
		return item.job(ctx)
	}
	var e error
	if p.config.Retry != nil {
		e = retry.Do(ctx, *p.config.Retry, operation)
	} else {
		e = operation(ctx)
	}
	done(e)
	if e != nil {
		p.metrics.failed.Add(1)
		p.config.Report(e)
	} else {
		p.metrics.succeeded.Add(1)
	}
}

// Register adds an immutable named job definition to this pool, never to a global registry.
func (p *Pool) Register(name string, job Job) error {
	if name == "" || job == nil {
		return errors.New("worker: job name and callback required")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return errors.New("worker: closed")
	}
	if _, exists := p.registered[name]; exists {
		return errors.New("worker: duplicate job name")
	}
	p.registered[name] = job
	return nil
}

// SubmitNamed enqueues a previously registered job using the same cancellation/backpressure policy as Submit.
func (p *Pool) SubmitNamed(ctx context.Context, name string) error {
	p.mu.RLock()
	job := p.registered[name]
	p.mu.RUnlock()
	if job == nil {
		return errors.New("worker: unknown job name")
	}
	return p.Submit(ctx, job)
}

// Submit blocks only until bounded admission, cancellation, or shutdown. Canceling ctx after admission also cancels this job.
// To intentionally detach a background job, pass an explicitly owned context such as context.WithoutCancel(requestContext).
func (p *Pool) Submit(ctx context.Context, job Job) error {
	if ctx == nil || job == nil {
		return errors.New("worker: context and job required")
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return errors.New("worker: closed")
	}
	select {
	case <-p.closing:
		return errors.New("worker: shutting down")
	default:
	}
	select {
	case <-p.closing:
		return errors.New("worker: shutting down")
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	case p.jobs <- entry{ctx: ctx, job: job}:
		p.metrics.submitted.Add(1)
		return nil
	}
}

// Shutdown stops admission and drains accepted work; on deadline it cancels running work and returns immediately.
// Handlers must honor cancellation. Repeated calls can wait on the same cleanup completion without creating extra wait goroutines.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.once.Do(func() { close(p.closing) })
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.jobs)
	}
	p.mu.Unlock()
	select {
	case <-p.finished:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}

// Cancel requests immediate cancellation. Shutdown can subsequently await actual worker termination.
func (p *Pool) Cancel() { p.cancel() }

// Stats returns per-pool counters with no application labels or mandatory telemetry globals.
func (p *Pool) Stats() Stats {
	return Stats{Submitted: p.metrics.submitted.Load(), Started: p.metrics.started.Load(), Completed: p.metrics.completed.Load(), Succeeded: p.metrics.succeeded.Load(), Failed: p.metrics.failed.Load(), Panics: p.metrics.panics.Load(), Active: p.metrics.active.Load(), Queued: len(p.jobs)}
}

type submissionContext struct {
	context.Context
	values context.Context
}

func (c submissionContext) Value(key any) any {
	if v := c.values.Value(key); v != nil {
		return v
	}
	return c.Context.Value(key)
}
