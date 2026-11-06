package franz

import (
	"context"
	"errors"
	"time"
)

func (c *Consumer) watchCommits(ctx context.Context) (context.Context, func(*error)) {
	ctx, cancel := context.WithCancel(ctx)
	failure := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
		case e := <-c.commitErrors:
			failure <- e
			cancel()
		}
	}()
	return ctx, func(result *error) {
		cancel()
		<-done
		select {
		case e := <-failure:
			*result = errors.Join(*result, e)
		default:
		}
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer stop()
		*result = errors.Join(*result, c.client.CommitMarkedOffsets(cleanup))
		c.client.AllowRebalance()
	}
}

// CloseContext cancels and drains consumption before closing the native client. A deadline bounds the wait, not arbitrary caller callbacks.
func (c *Consumer) CloseContext(ctx context.Context) error {
	return c.life.Shutdown(ctx, func() error { c.client.CloseAllowingRebalance(); return nil })
}
