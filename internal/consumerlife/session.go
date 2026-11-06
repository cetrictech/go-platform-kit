// Package consumerlife coordinates explicit consumer cancellation and native cleanup.
package consumerlife

import (
	"context"
	"errors"
	"sync"
)

// Session permits one consuming loop and one concurrent, idempotent shutdown.
type Session struct {
	mu        sync.Mutex
	cancel    context.CancelFunc
	active    chan struct{}
	closed    bool
	closeOnce sync.Once
	closeDone chan struct{}
	closeErr  error
}

// Begin registers a cancelable consuming loop. Finish must be deferred after successful acquisition.
func (s *Session) Begin(ctx context.Context) (context.Context, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.active != nil {
		return ctx, nil, errors.New("kafka: consumer closed or already consuming")
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.cancel = cancel
	s.active = done
	var once sync.Once
	return ctx, func() {
		once.Do(func() {
			cancel()
			s.mu.Lock()
			s.cancel = nil
			s.active = nil
			close(done)
			s.mu.Unlock()
		})
	}, nil
}

// Shutdown cancels and waits for the loop before native cleanup. On deadline, cleanup continues and later calls can await it.
func (s *Session) Shutdown(ctx context.Context, cleanup func() error) error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		if s.cancel != nil {
			s.cancel()
		}
		active := s.active
		s.closeDone = make(chan struct{})
		s.mu.Unlock()
		go func() {
			if active != nil {
				<-active
			}
			e := cleanup()
			s.mu.Lock()
			s.closeErr = e
			close(s.closeDone)
			s.mu.Unlock()
		}()
	})
	s.mu.Lock()
	done := s.closeDone
	s.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.closeErr
	}
}
