// Package mysql provides database/sql-backed MySQL access with explicit transactions.
package mysql

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	db "github.com/cetrictech/go-platform-kit/database"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/instrument"
	driver "github.com/go-sql-driver/mysql"
	"log/slog"
	"time"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Hooks          db.Hooks
	QueryLogger    *slog.Logger
	QueryTimeout   time.Duration
	AllowPlaintext bool
	Observer       instrument.Hook
	DSN            string
	// MaxOpenConnections bounds concurrent connections; zero defaults to 32.
	MaxOpenConnections int
	// MaxIdleConnections must not exceed the effective open-connection bound.
	MaxIdleConnections                       int
	MaxLifetime, MaxIdleTime, ConnectTimeout time.Duration
	TLS                                      *tls.Config
}

// Client owns its configured infrastructure client and supports concurrent operations.
type Client struct {
	db       *sql.DB
	observer instrument.Hook
	hooks    db.Hooks
	logger   *slog.Logger
	timeout  time.Duration
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(ctx context.Context, c Config) (*Client, error) {
	if c.DSN == "" || c.MaxOpenConnections < 0 || c.MaxIdleConnections < 0 || c.ConnectTimeout < 0 || c.MaxLifetime < 0 || c.MaxIdleTime < 0 || c.QueryTimeout < 0 {
		return nil, errors.New("mysql: invalid config")
	}
	if c.MaxOpenConnections == 0 {
		c.MaxOpenConnections = 32
	}
	if c.MaxIdleConnections > c.MaxOpenConnections {
		return nil, errors.New("mysql: idle connections exceed open connection bound")
	}
	cfg, e := driver.ParseDSN(c.DSN)
	if e != nil {
		return nil, errors.New("mysql: invalid connection configuration")
	}
	cfg.ParseTime = true
	if c.ConnectTimeout == 0 && cfg.Timeout == 0 {
		cfg.Timeout = 5 * time.Second
	}
	if c.ConnectTimeout > 0 {
		cfg.Timeout = c.ConnectTimeout
	}
	if c.TLS != nil {
		if c.TLS.InsecureSkipVerify && c.TLS.VerifyPeerCertificate == nil && c.TLS.VerifyConnection == nil {
			return nil, errors.New("mysql: TLS verification required")
		}
		cfg.TLS = c.TLS.Clone()
	}
	if !c.AllowPlaintext {
		if cfg.AllowFallbackToPlaintext {
			return nil, errors.New("mysql: plaintext TLS fallback rejected")
		}
		if cfg.TLS == nil || cfg.TLS.InsecureSkipVerify && cfg.TLS.VerifyPeerCertificate == nil && cfg.TLS.VerifyConnection == nil {
			return nil, errors.New("mysql: verified TLS required; explicitly opt into plaintext for local development")
		}
	}
	connector, e := driver.NewConnector(cfg)
	if e != nil {
		return nil, e
	}
	s := sql.OpenDB(connector)
	s.SetMaxOpenConns(c.MaxOpenConnections)
	if c.MaxIdleConnections > 0 {
		s.SetMaxIdleConns(c.MaxIdleConnections)
	}
	s.SetConnMaxLifetime(c.MaxLifetime)
	s.SetConnMaxIdleTime(c.MaxIdleTime)
	if e = s.PingContext(ctx); e != nil {
		s.Close()
		return nil, Classify(e)
	}
	return &Client{db: s, observer: c.Observer, hooks: c.Hooks, logger: c.QueryLogger, timeout: c.QueryTimeout}, nil
}

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (c *Client) Native() *sql.DB { return c.db }

// Ping checks connectivity using the supplied context.
func (c *Client) Ping(ctx context.Context) error { return Classify(c.db.PingContext(ctx)) }

// Close releases owned resources. See package documentation for shutdown ordering.
func (c *Client) Close() error { return c.db.Close() }

// Stats returns a concurrency-safe statistics snapshot.
func (c *Client) Stats() sql.DBStats { return c.db.Stats() }

// Exec executes a parameterized SQL command and returns its result.
func (c *Client) Exec(ctx context.Context, q string, a ...any) (sql.Result, error) {
	ctx, done := startQuery(ctx, c.observer, c.hooks, c.logger, c.timeout, "mysql.exec", q)
	r, e := c.db.ExecContext(ctx, q, a...)
	done(e)
	return r, Classify(e)
}

// Query opens a row stream; the caller must close it.
func (c *Client) Query(ctx context.Context, q string, a ...any) (db.Rows, error) {
	ctx, done := startQuery(ctx, c.observer, c.hooks, c.logger, c.timeout, "mysql.query", q)
	r, e := c.db.QueryContext(ctx, q, a...)
	if e != nil {
		done(e)
		return nil, Classify(e)
	}
	return &observedRows{Rows: r, done: done}, nil
}

// QueryRow returns a single-row scanner; call Scan to execute or observe its error.
func (c *Client) QueryRow(ctx context.Context, q string, a ...any) db.Row {
	ctx, done := startQuery(ctx, c.observer, c.hooks, c.logger, c.timeout, "mysql.query_row", q)
	return observedRow{c.db.QueryRowContext(ctx, q, a...), done}
}

// Get reads one value into the requested representation.
func (c *Client) Get(ctx context.Context, dst any, q string, a ...any) error {
	r, e := c.Query(ctx, q, a...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dst, false)
}

// Select scans all result rows into a slice pointer.
func (c *Client) Select(ctx context.Context, dst any, q string, a ...any) error {
	r, e := c.Query(ctx, q, a...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dst, true)
}

// Prepare creates an owned prepared statement that must be closed.
func (c *Client) Prepare(ctx context.Context, q string) (*Statement, error) {
	ctx, done := startQuery(ctx, c.observer, c.hooks, c.logger, c.timeout, "mysql.prepare", q)
	stmt, e := c.db.PrepareContext(ctx, q)
	done(e)
	if e != nil {
		return nil, Classify(e)
	}
	return &Statement{statement: stmt, client: c, query: q}, nil
}

// TxOptions configures transaction isolation and access.
type TxOptions struct {
	Isolation sql.IsolationLevel
	ReadOnly  bool
}

// Tx owns a transaction; do not share it concurrently.
type Tx struct {
	tx       *sql.Tx
	observer instrument.Hook
	hooks    db.Hooks
	logger   *slog.Logger
	timeout  time.Duration
}

// BeginTx opens a transaction that the caller must commit or roll back.
func (c *Client) BeginTx(ctx context.Context, o TxOptions) (*Tx, error) {
	if c.hooks.BeforeBegin != nil {
		if e := c.hooks.BeforeBegin(ctx); e != nil {
			return nil, e
		}
	}
	start := time.Now()
	t, e := c.db.BeginTx(ctx, &sql.TxOptions{Isolation: o.Isolation, ReadOnly: o.ReadOnly})
	db.ReportTransaction(ctx, c.hooks, "begin", start, e)
	if e != nil {
		return nil, Classify(e)
	}
	return &Tx{tx: t, observer: c.observer, hooks: c.hooks, logger: c.logger, timeout: c.timeout}, nil
}

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (t *Tx) Native() *sql.Tx { return t.tx }

// Commit commits the transaction and reports commit errors.
func (t *Tx) Commit() error { return t.CommitContext(context.Background()) }

// Rollback rolls back the transaction.
func (t *Tx) Rollback() error { return t.RollbackContext(context.Background()) }

// Exec executes a parameterized SQL command and returns its result.
func (t *Tx) Exec(ctx context.Context, q string, a ...any) (sql.Result, error) {
	ctx, done := startQuery(ctx, t.observer, t.hooks, t.logger, t.timeout, "mysql.exec", q)
	r, e := t.tx.ExecContext(ctx, q, a...)
	done(e)
	return r, Classify(e)
}

// Query opens a row stream; the caller must close it.
func (t *Tx) Query(ctx context.Context, q string, a ...any) (db.Rows, error) {
	ctx, done := startQuery(ctx, t.observer, t.hooks, t.logger, t.timeout, "mysql.query", q)
	r, e := t.tx.QueryContext(ctx, q, a...)
	if e != nil {
		done(e)
		return nil, Classify(e)
	}
	return &observedRows{Rows: r, done: done}, nil
}

// QueryRow returns a single-row scanner; call Scan to execute or observe its error.
func (t *Tx) QueryRow(ctx context.Context, q string, a ...any) db.Row {
	ctx, done := startQuery(ctx, t.observer, t.hooks, t.logger, t.timeout, "mysql.query_row", q)
	return observedRow{t.tx.QueryRowContext(ctx, q, a...), done}
}

// Get reads one value into the requested representation.
func (t *Tx) Get(ctx context.Context, d any, q string, a ...any) error {
	r, e := t.Query(ctx, q, a...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, d, false)
}

// Select scans all result rows into a slice pointer.
func (t *Tx) Select(ctx context.Context, d any, q string, a ...any) error {
	r, e := t.Query(ctx, q, a...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, d, true)
}

// WithinTx runs a callback transactionally, rolling back on failure or panic.
func (c *Client) WithinTx(ctx context.Context, o TxOptions, fn func(*Tx) error) (err error) {
	if fn == nil {
		return errors.New("mysql: nil transaction callback")
	}
	t, e := c.BeginTx(ctx, o)
	if e != nil {
		return e
	}
	defer func() {
		e := t.RollbackContext(context.WithoutCancel(ctx))
		if e != nil && !errors.Is(e, sql.ErrTxDone) {
			err = errors.Join(err, e)
		}
	}()
	if e = fn(t); e != nil {
		return e
	}
	return t.CommitContext(ctx)
}

// Operation contains a parameterized SQL command and its arguments.
type Operation struct {
	SQL  string
	Args []any
}

// Batch executes commands sequentially in one transaction, rolling back on any error.
func (c *Client) Batch(ctx context.Context, ops []Operation) ([]sql.Result, error) {
	var out []sql.Result
	e := c.WithinTx(ctx, TxOptions{}, func(t *Tx) error {
		for _, o := range ops {
			r, e := t.Exec(ctx, o.SQL, o.Args...)
			if e != nil {
				return e
			}
			out = append(out, r)
		}
		return nil
	})
	if e != nil {
		return nil, e
	}
	return out, nil
}

// Classify maps supported native error codes while preserving the original cause.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	var m *driver.MySQLError
	if errors.As(err, &m) {
		cat, status, code, retry := app.Database, 500, "DATABASE_ERROR", false
		switch m.Number {
		case 1062:
			cat, status, code = app.Conflict, 409, "DUPLICATE_KEY"
		case 1213, 1205:
			code, retry = "TRANSACTION_RETRY", true
		}
		return app.Wrap(err, "We could not complete your request.", app.WithCategory(cat), app.WithCode(code), app.WithHTTPStatus(status), app.WithRetryable(retry))
	}
	return err
}
