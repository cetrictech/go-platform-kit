package postgres

import (
	"context"
	"database/sql"
	"errors"
	db "github.com/cetrictech/go-platform-kit/database"
	"sync"
)

// Statement owns a database/sql prepared statement and a pgxpool bridge. It supports concurrent operations.
// Close it before closing the Client. PostgreSQL positional placeholders are used in prepared SQL.
type Statement struct {
	statement *sql.Stmt
	bridge    *sql.DB
	once      sync.Once
	err       error
	client    *Client
}

// Prepare validates and prepares SQL on a pgx-backed connection. Normal queries also retain pgx's native statement cache.
func (c *Client) Prepare(ctx context.Context, query string) (*Statement, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	bridge := c.SQLDB()
	s, e := bridge.PrepareContext(ctx, query)
	if e != nil {
		bridge.Close()
		return nil, Classify(e)
	}
	return &Statement{statement: s, bridge: bridge, client: c}, nil
}

// Exec executes bound arguments. Command is empty because database/sql does not expose PostgreSQL command tags.
func (s *Statement) Exec(ctx context.Context, args ...any) (Result, error) {
	ctx, cancel := s.client.ctx(ctx)
	defer cancel()
	r, e := s.statement.ExecContext(ctx, args...)
	if e != nil {
		return Result{}, Classify(e)
	}
	n, e := r.RowsAffected()
	return Result{RowsAffected: n}, e
}

// Query opens a stream whose deadline ends on Close or exhaustion.
func (s *Statement) Query(ctx context.Context, args ...any) (db.Rows, error) {
	ctx, cancel := s.client.ctx(ctx)
	r, e := s.statement.QueryContext(ctx, args...)
	if e != nil {
		cancel()
		return nil, Classify(e)
	}
	return &statementRows{Rows: r, cancel: cancel}, nil
}

type statementRows struct {
	*sql.Rows
	cancel context.CancelFunc
	once   sync.Once
}

func (r *statementRows) Close() error { e := r.Rows.Close(); r.once.Do(r.cancel); return e }
func (r *statementRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.once.Do(r.cancel)
	}
	return ok
}

// QueryRow defers execution until Scan so resources cannot be acquired by an unconsumed row handle.
func (s *Statement) QueryRow(ctx context.Context, args ...any) db.Row {
	return &row{scan: func(dest ...any) error {
		ctx, cancel := s.client.ctx(ctx)
		defer cancel()
		return Classify(s.statement.QueryRowContext(ctx, args...).Scan(dest...))
	}}
}

// Get scans exactly one record, returning sql.ErrNoRows when absent.
func (s *Statement) Get(ctx context.Context, dest any, args ...any) error {
	r, e := s.Query(ctx, args...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dest, false)
}

// Select replaces the destination slice with all rows on success.
func (s *Statement) Select(ctx context.Context, dest any, args ...any) error {
	r, e := s.Query(ctx, args...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dest, true)
}

// Native is an explicit escape hatch; the statement remains owned by this handle.
func (s *Statement) Native() *sql.Stmt { return s.statement }

// Close releases the prepared handle and its bridge exactly once.
func (s *Statement) Close() error {
	s.once.Do(func() { s.err = errors.Join(s.statement.Close(), s.bridge.Close()) })
	return s.err
}
