package database

import (
	"context"
	"log/slog"
	"time"
)

// QueryEvent contains query diagnostics without parameter values. Statement is available only to explicit hooks.
// Never use Statement as a metric label or log SQL containing literal secrets.
type QueryEvent struct {
	Operation, Statement string
	Duration             time.Duration
	Err                  error
}

// TransactionEvent reports a begin, commit, rollback, savepoint or release attempt.
type TransactionEvent struct {
	Operation string
	Duration  time.Duration
	Err       error
}

// Hooks run synchronously and must be concurrency safe. BeforeBegin can reject a transaction before acquiring resources.
type Hooks struct {
	BeforeBegin      func(context.Context) error
	AfterQuery       func(context.Context, QueryEvent)
	AfterTransaction func(context.Context, TransactionEvent)
}

// ReportQuery emits only bounded operation/success/duration attributes to the configured logger. SQL and arguments are never logged.
func ReportQuery(ctx context.Context, h Hooks, l *slog.Logger, e QueryEvent) {
	if h.AfterQuery != nil {
		h.AfterQuery(ctx, e)
	}
	if l != nil {
		l.DebugContext(ctx, "database query", "operation", e.Operation, "duration", e.Duration, "success", e.Err == nil)
	}
}

// ReportTransaction invokes transaction instrumentation without changing the database outcome.
func ReportTransaction(ctx context.Context, h Hooks, operation string, start time.Time, err error) {
	if h.AfterTransaction != nil {
		h.AfterTransaction(ctx, TransactionEvent{operation, time.Since(start), err})
	}
}
