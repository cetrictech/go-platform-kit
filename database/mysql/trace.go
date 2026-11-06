package mysql

import (
	"context"
	db "github.com/cetrictech/go-platform-kit/database"
	"github.com/cetrictech/go-platform-kit/instrument"
	"log/slog"
	"sync"
	"time"
)

type observedRows struct {
	db.Rows
	done func(error)
	once sync.Once
}

func (r *observedRows) finish(e error) { r.once.Do(func() { r.done(e) }) }
func (r *observedRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.finish(r.Rows.Err())
	}
	return ok
}
func (r *observedRows) Scan(a ...any) error {
	e := r.Rows.Scan(a...)
	if e != nil {
		r.finish(e)
	}
	return e
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (r *observedRows) Close() error { e := r.Rows.Close(); r.finish(e); return e }

type observedRow struct {
	db.Row
	done func(error)
}

func (r observedRow) Scan(a ...any) error { e := r.Row.Scan(a...); r.done(e); return e }

func startQuery(ctx context.Context, hook instrument.Hook, h db.Hooks, l *slog.Logger, timeout time.Duration, operation, query string) (context.Context, func(error)) {
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		ctx, cancel = context.WithCancel(ctx)
	}
	ctx, done := instrument.Start(ctx, hook, operation)
	start := time.Now()
	var once sync.Once
	return ctx, func(e error) {
		once.Do(func() {
			done(e)
			db.ReportQuery(ctx, h, l, db.QueryEvent{Operation: operation, Statement: query, Duration: time.Since(start), Err: e})
			cancel()
		})
	}
}
