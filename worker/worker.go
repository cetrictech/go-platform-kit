// Package worker runs bounded, cancellable workers with panic reporting and draining.
package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Job performs cancellable worker processing.
type Job func(context.Context) error

// Pool owns a bounded worker queue and its cancellation lifecycle.
type Pool struct {
	jobs    chan Job
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	report  func(error)
	mu      sync.RWMutex
	closed  bool
	closing chan struct{}
	once    sync.Once
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(ctx context.Context, concurrency, queue int, report func(error)) (*Pool, error) {
	if concurrency < 1 || queue < 0 || report == nil {
		return nil, errors.New("worker: invalid configuration or missing error reporter")
	}
	ctx, cancel := context.WithCancel(ctx)
	p := &Pool{jobs: make(chan Job, queue), ctx: ctx, cancel: cancel, report: report, closing: make(chan struct{})}
	for i := 0; i < concurrency; i++ {
		p.wg.Add(1)
		go p.run()
	}
	return p, nil
}
func (p *Pool) run() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case job, ok := <-p.jobs:
			if !ok {
				return
			}
			func() {
				defer func() {
					if x := recover(); x != nil {
						p.report(fmt.Errorf("worker panic: %v", x))
					}
				}()
				if e := job(p.ctx); e != nil {
					p.report(e)
				}
			}()
		}
	}
}

// Submit enqueues a job with backpressure and cancellable waiting.
func (p *Pool) Submit(ctx context.Context, job Job) error {
	if job == nil {
		return errors.New("worker: nil job")
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return errors.New("worker: closed")
	}
	select {
	case <-p.closing:
		return errors.New("worker: shutting down")
	case <-ctx.Done():
		return ctx.Err()
	case <-p.ctx.Done():
		return p.ctx.Err()
	case p.jobs <- job:
		return nil
	}
}

// Shutdown stops accepting jobs, drains queued work, and cancels running work if its deadline expires.
func (p *Pool) Shutdown(ctx context.Context) error {
	p.once.Do(func() { close(p.closing) })
	p.mu.Lock()
	if !p.closed {
		p.closed = true
		close(p.jobs)
	}
	p.mu.Unlock()
	done := make(chan struct{})
	go func() { p.wg.Wait(); close(done) }()
	select {
	case <-done:
		p.cancel()
		return nil
	case <-ctx.Done():
		p.cancel()
		return ctx.Err()
	}
}

// Cancel cancels owned worker execution; use Shutdown to wait for draining.
func (p *Pool) Cancel() { p.cancel() }
