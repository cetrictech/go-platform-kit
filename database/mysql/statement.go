package mysql

import (
	"context"
	"database/sql"
	"errors"
	db "github.com/cetrictech/go-platform-kit/database"
	"sync"
)

// Statement owns a prepared SQL statement and borrows the Client's pool and instrumentation.
type Statement struct {
	statement *sql.Stmt
	client    *Client
	query     string
}

// Exec executes prepared bound arguments with configured timeout and diagnostics.
func (s *Statement) Exec(ctx context.Context, args ...any) (sql.Result, error) {
	ctx, done := startQuery(ctx, s.client.observer, s.client.hooks, s.client.logger, s.client.timeout, "mysql.prepared.exec", s.query)
	r, e := s.statement.ExecContext(ctx, args...)
	done(e)
	return r, Classify(e)
}

// Query opens a prepared row stream. Close releases stream ownership and instrumentation.
func (s *Statement) Query(ctx context.Context, args ...any) (db.Rows, error) {
	ctx, done := startQuery(ctx, s.client.observer, s.client.hooks, s.client.logger, s.client.timeout, "mysql.prepared.query", s.query)
	r, e := s.statement.QueryContext(ctx, args...)
	if e != nil {
		done(e)
		return nil, Classify(e)
	}
	return &observedRows{Rows: r, done: done}, nil
}

// QueryRow executes only on Scan, avoiding resource acquisition for an unused handle.
func (s *Statement) QueryRow(ctx context.Context, args ...any) db.Row {
	return &statementRow{scan: func(dest ...any) error {
		ctx, done := startQuery(ctx, s.client.observer, s.client.hooks, s.client.logger, s.client.timeout, "mysql.prepared.query_row", s.query)
		e := s.statement.QueryRowContext(ctx, args...).Scan(dest...)
		done(e)
		return Classify(e)
	}}
}

type statementRow struct {
	scan func(...any) error
	mu   sync.Mutex
	used bool
}

func (r *statementRow) Scan(dest ...any) error {
	r.mu.Lock()
	if r.used {
		r.mu.Unlock()
		return errors.New("mysql: row already scanned")
	}
	r.used = true
	r.mu.Unlock()
	return r.scan(dest...)
}

// Get scans one record, returning sql.ErrNoRows for an empty result.
func (s *Statement) Get(ctx context.Context, dest any, args ...any) error {
	r, e := s.Query(ctx, args...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dest, false)
}

// Select replaces the destination slice on successful result consumption.
func (s *Statement) Select(ctx context.Context, dest any, args ...any) error {
	r, e := s.Query(ctx, args...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dest, true)
}

// Close releases this statement. The client remains caller-owned.
func (s *Statement) Close() error { return s.statement.Close() }

// Native exposes the standard SQL handle as an advanced escape hatch.
func (s *Statement) Native() *sql.Stmt { return s.statement }
