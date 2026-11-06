package redis

import (
	"context"
	"github.com/cetrictech/go-platform-kit/instrument"
	r "github.com/redis/go-redis/v9"
	"net"
)

type redisHook struct{ hook instrument.Hook }

func (h redisHook) DialHook(next r.DialHook) r.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, done := instrument.Start(ctx, h.hook, "redis.connect")
		c, e := next(ctx, network, addr)
		done(e)
		return c, e
	}
}
func (h redisHook) ProcessHook(next r.ProcessHook) r.ProcessHook {
	return func(ctx context.Context, c r.Cmder) error {
		ctx, done := instrument.Start(ctx, h.hook, "redis.command")
		e := next(ctx, c)
		done(e)
		return e
	}
}
func (h redisHook) ProcessPipelineHook(next r.ProcessPipelineHook) r.ProcessPipelineHook {
	return func(ctx context.Context, c []r.Cmder) error {
		ctx, done := instrument.Start(ctx, h.hook, "redis.pipeline")
		e := next(ctx, c)
		done(e)
		return e
	}
}
