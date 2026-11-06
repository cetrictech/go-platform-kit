package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"
)

// Cleanup adapts HTTP/gRPC servers, workers, telemetry providers, Kafka producers/consumers, SQL/Redis clients and log outputs.
// The resulting function is idempotent; a deadline can stop waiting without losing the ability to await ongoing cleanup again.
func Cleanup(resource any) (func(context.Context) error, error) {
	if resource == nil || reflect.ValueOf(resource).Kind() == reflect.Pointer && reflect.ValueOf(resource).IsNil() {
		return nil, errors.New("lifecycle: nil resource")
	}
	var operation func(context.Context) error
	switch r := resource.(type) {
	case interface{ Shutdown(context.Context) error }:
		operation = r.Shutdown
	case interface{ CloseContext(context.Context) error }:
		operation = r.CloseContext
	case interface {
		GracefulStop()
		Stop()
	}:
		var once sync.Once
		done := make(chan struct{})
		operation = func(ctx context.Context) error {
			once.Do(func() { go func() { r.GracefulStop(); close(done) }() })
			select {
			case <-done:
				return nil
			case <-ctx.Done():
				r.Stop()
				return ctx.Err()
			}
		}
	case interface {
		Flush(context.Context) error
		Close() error
	}:
		operation = func(ctx context.Context) error { return errors.Join(r.Flush(ctx), r.Close()) }
	case interface{ Close() error }:
		operation = func(context.Context) error { return r.Close() }
	case interface{ Close() }:
		operation = func(context.Context) error { r.Close(); return nil }
	default:
		return nil, fmt.Errorf("lifecycle: unsupported resource %T", resource)
	}
	var once sync.Once
	done := make(chan struct{})
	var result error
	return func(ctx context.Context) error {
		once.Do(func() { go func() { result = operation(ctx); close(done) }() })
		select {
		case <-done:
			return result
		case <-ctx.Done():
			return ctx.Err()
		}
	}, nil
}

// RegisterResource initializes an owned resource in startup order and closes it in reverse order.
// A factory that returns both a resource and an error has that partial resource cleaned up before the error is returned.
func (m *Manager) RegisterResource(name string, factory func(context.Context) (any, error)) error {
	if factory == nil {
		return errors.New("lifecycle: resource factory required")
	}
	var cleanup func(context.Context) error
	return m.Register(Hook{Name: name, Start: func(ctx context.Context) error {
		resource, e := factory(ctx)
		if resource == nil {
			if e != nil {
				return e
			}
			return errors.New("lifecycle: factory returned nil resource")
		}
		closeResource, closeErr := Cleanup(resource)
		if closeErr != nil {
			return errors.Join(e, closeErr)
		}
		if e != nil {
			timeout := m.Timeout
			if timeout <= 0 {
				timeout = 30 * time.Second
			}
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
			defer cancel()
			return errors.Join(e, closeResource(cleanupCtx))
		}
		cleanup = closeResource
		return nil
	}, Stop: func(ctx context.Context) error {
		if cleanup != nil {
			return cleanup(ctx)
		}
		return nil
	}})
}
