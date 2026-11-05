package postgres

import (
	"context"
	db "github.com/cetrictech/go-platform-kit/database"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/jackc/pgx/v5"
	"log/slog"
	"time"
)

type queryTracer struct {
	hook   instrument.Hook
	hooks  db.Hooks
	logger *slog.Logger
}
type traceKey struct{}
type traceState struct {
	start     time.Time
	statement string
	done      func(error)
}

func (t queryTracer) start(ctx context.Context, operation, statement string) context.Context {
	ctx, done := instrument.Start(ctx, t.hook, operation)
	return context.WithValue(ctx, traceKey{}, traceState{time.Now(), statement, done})
}
func (t queryTracer) finish(ctx context.Context, operation string, e error) {
	if s, ok := ctx.Value(traceKey{}).(traceState); ok {
		s.done(e)
		db.ReportQuery(ctx, t.hooks, t.logger, db.QueryEvent{Operation: operation, Statement: s.statement, Duration: time.Since(s.start), Err: e})
	}
}
func (t queryTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return t.start(ctx, "postgres.query", d.SQL)
}
func (t queryTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	t.finish(ctx, "postgres.query", d.Err)
}
func (t queryTracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return t.start(ctx, "postgres.batch", "")
}
func (t queryTracer) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {}
func (t queryTracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceBatchEndData) {
	t.finish(ctx, "postgres.batch", d.Err)
}
func (t queryTracer) TraceCopyFromStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceCopyFromStartData) context.Context {
	return t.start(ctx, "postgres.copy", "")
}
func (t queryTracer) TraceCopyFromEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceCopyFromEndData) {
	t.finish(ctx, "postgres.copy", d.Err)
}
