package postgres

import (
	"context"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/jackc/pgx/v5"
)

type queryTracer struct{ hook instrument.Hook }
type traceKey struct{}

func (t queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	ctx, done := instrument.Start(ctx, t.hook, "postgres.query")
	return context.WithValue(ctx, traceKey{}, done)
}
func (t queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if done, ok := ctx.Value(traceKey{}).(func(error)); ok {
		done(d.Err)
	}
}
