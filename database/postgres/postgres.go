// Package postgres provides pgxpool-backed PostgreSQL access without requiring driver imports.
package postgres

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	db "github.com/cetrictech/go-platform-kit/database"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"log/slog"
	"sync"
	"time"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Hooks                                                  db.Hooks
	QueryLogger                                            *slog.Logger
	ReadOnly                                               bool
	AllowPlaintext                                         bool
	Observer                                               instrument.Hook
	URL                                                    string
	MaxConnections, MinConnections                         int32
	MaxLifetime, MaxIdleTime, ConnectTimeout, QueryTimeout time.Duration
	TLS                                                    *tls.Config
	ApplicationName                                        string
	StatementTimeout                                       time.Duration
	AfterConnect                                           func(context.Context) error
}

// Client owns its configured infrastructure client and supports concurrent operations.
type Client struct {
	pool    *pgxpool.Pool
	timeout time.Duration
	hooks   db.Hooks
}

// Row and Rows are the driver-neutral database result contracts exposed to
// consumers. Driver types remain private to this package.
type Row = db.Row
type Rows = db.Rows

// Executor is the SQL surface shared by Client and Tx.
type Executor interface {
	Exec(context.Context, string, ...any) (Result, error)
	Query(context.Context, string, ...any) (Rows, error)
	QueryRow(context.Context, string, ...any) Row
}

// IsNoRows reports whether the driver returned the no-row condition.
func IsNoRows(err error) bool { return errors.Is(err, pgx.ErrNoRows) }

// ConstraintError exposes PostgreSQL constraint details without exposing pgx.
func ConstraintError(err error) (code, constraint string, ok bool) {
	var e *pgconn.PgError
	if !errors.As(err, &e) || e == nil {
		return "", "", false
	}
	return e.Code, e.ConstraintName, true
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(ctx context.Context, c Config) (*Client, error) {
	if c.URL == "" || c.MaxConnections < 0 || c.MinConnections < 0 || c.QueryTimeout < 0 || c.ConnectTimeout < 0 || c.MaxIdleTime < 0 || c.MaxLifetime < 0 || c.StatementTimeout < 0 {
		return nil, errors.New("postgres: invalid config")
	}
	p, e := pgxpool.ParseConfig(c.URL)
	if e != nil {
		return nil, errors.New("postgres: invalid connection configuration")
	}
	if c.TLS == nil && !c.AllowPlaintext {
		if p.ConnConfig.TLSConfig == nil || p.ConnConfig.TLSConfig.InsecureSkipVerify && p.ConnConfig.TLSConfig.VerifyPeerCertificate == nil && p.ConnConfig.TLSConfig.VerifyConnection == nil {
			return nil, errors.New("postgres: verified TLS required; explicitly opt into plaintext for local development")
		}
		for _, f := range p.ConnConfig.Fallbacks {
			if f.TLSConfig == nil {
				return nil, errors.New("postgres: plaintext TLS fallback rejected")
			}
		}
	}
	if c.MaxConnections > 0 {
		p.MaxConns = c.MaxConnections
	}
	p.MinConns = c.MinConnections
	if p.MinConns > p.MaxConns {
		return nil, errors.New("postgres: minimum exceeds maximum connections")
	}
	if c.MaxLifetime > 0 {
		p.MaxConnLifetime = c.MaxLifetime
	}
	if c.MaxIdleTime > 0 {
		p.MaxConnIdleTime = c.MaxIdleTime
	}
	if c.ConnectTimeout == 0 {
		c.ConnectTimeout = 5 * time.Second
	}
	if c.ConnectTimeout > 0 {
		p.ConnConfig.ConnectTimeout = c.ConnectTimeout
	}
	if c.TLS != nil {
		if c.TLS.InsecureSkipVerify && c.TLS.VerifyPeerCertificate == nil && c.TLS.VerifyConnection == nil {
			return nil, errors.New("postgres: TLS verification required")
		}
		p.ConnConfig.TLSConfig = c.TLS.Clone()
		if p.ConnConfig.TLSConfig.ServerName == "" {
			p.ConnConfig.TLSConfig.ServerName = p.ConnConfig.Host
		}
		p.ConnConfig.Fallbacks = nil
	}
	if c.ApplicationName != "" {
		p.ConnConfig.RuntimeParams["application_name"] = c.ApplicationName
	}
	if c.StatementTimeout > 0 {
		p.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprint(c.StatementTimeout.Milliseconds())
	}
	if c.AfterConnect != nil {
		p.AfterConnect = func(ctx context.Context, _ *pgx.Conn) error { return c.AfterConnect(ctx) }
	}
	if c.ReadOnly {
		p.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
	}
	if c.Observer != nil || c.QueryLogger != nil || c.Hooks.AfterQuery != nil {
		p.ConnConfig.Tracer = queryTracer{hook: c.Observer, hooks: c.Hooks, logger: c.QueryLogger}
	}
	pool, e := pgxpool.NewWithConfig(ctx, p)
	if e != nil {
		return nil, e
	}
	if e = pool.Ping(ctx); e != nil {
		pool.Close()
		return nil, Classify(e)
	}
	return &Client{pool: pool, timeout: c.QueryTimeout, hooks: c.Hooks}, nil
}

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (c *Client) Native() *pgxpool.Pool { return c.pool }

// Ping checks connectivity using the supplied context.
func (c *Client) Ping(ctx context.Context) error { return Classify(c.pool.Ping(ctx)) }

// Close releases owned resources. See package documentation for shutdown ordering.
func (c *Client) Close() { c.pool.Close() }

// Stats contains a snapshot of operational counters.
type Stats struct {
	Total, Idle, Acquired, Maximum int32
	AcquireCount                   int64
	AcquireDuration                time.Duration
}

// Stats returns a concurrency-safe statistics snapshot.
func (c *Client) Stats() Stats {
	s := c.pool.Stat()
	return Stats{s.TotalConns(), s.IdleConns(), s.AcquiredConns(), s.MaxConns(), s.AcquireCount(), s.AcquireDuration()}
}

// Result contains operation status without exposing a native driver.
type Result struct {
	RowsAffected int64
	Command      string
}

func (c *Client) ctx(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.timeout > 0 {
		return context.WithTimeout(ctx, c.timeout)
	}
	return context.WithCancel(ctx)
}

// Exec executes a parameterized SQL command and returns its result.
func (c *Client) Exec(ctx context.Context, q string, args ...any) (Result, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	r, e := c.pool.Exec(ctx, q, bindArgs(args)...)
	return Result{r.RowsAffected(), r.String()}, Classify(e)
}

type rows struct {
	pgx.Rows
	cancel context.CancelFunc
}

func (r *rows) Columns() ([]string, error) {
	f := r.FieldDescriptions()
	a := make([]string, len(f))
	for i := range f {
		a[i] = f[i].Name
	}
	return a, nil
}
func (r *rows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.cancel()
	}
	return ok
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (r *rows) Close() error { r.Rows.Close(); r.cancel(); return r.Err() }

// Query opens a row stream; the caller must close it.
func (c *Client) Query(ctx context.Context, q string, args ...any) (db.Rows, error) {
	ctx, cancel := c.ctx(ctx)
	r, e := c.pool.Query(ctx, q, bindArgs(args)...)
	if e != nil {
		cancel()
		return nil, Classify(e)
	}
	return &rows{r, cancel}, nil
}

type row struct {
	scan func(...any) error
	mu   sync.Mutex
	used bool
}

func (r *row) Scan(a ...any) error {
	r.mu.Lock()
	if r.used {
		r.mu.Unlock()
		return errors.New("postgres: row already scanned")
	}
	r.used = true
	r.mu.Unlock()
	return r.scan(a...)
}

// QueryRow returns a single-row scanner; call Scan to execute or observe its error.
func (c *Client) QueryRow(ctx context.Context, q string, args ...any) db.Row {
	return &row{scan: func(dst ...any) error {
		ctx, cancel := c.ctx(ctx)
		defer cancel()
		return Classify(c.pool.QueryRow(ctx, q, bindArgs(args)...).Scan(dst...))
	}}
}

// Get reads one value into the requested representation.
func (c *Client) Get(ctx context.Context, dst any, q string, args ...any) error {
	r, e := c.Query(ctx, q, bindArgs(args)...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dst, false)
}

// Select scans all result rows into a slice pointer.
func (c *Client) Select(ctx context.Context, dst any, q string, args ...any) error {
	r, e := c.Query(ctx, q, bindArgs(args)...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, dst, true)
}

// TxOptions configures transaction isolation and access.
type TxOptions struct {
	Isolation string
	ReadOnly  bool
}

// Tx owns a transaction; do not share it concurrently.
type Tx struct {
	tx    pgx.Tx
	hooks db.Hooks
}

// BeginTx opens a transaction that the caller must commit or roll back.
func (c *Client) BeginTx(ctx context.Context, o TxOptions) (*Tx, error) {
	if c.hooks.BeforeBegin != nil {
		if e := c.hooks.BeforeBegin(ctx); e != nil {
			return nil, e
		}
	}
	start := time.Now()
	iso := pgx.TxIsoLevel(o.Isolation)
	switch iso {
	case "", pgx.ReadCommitted, pgx.RepeatableRead, pgx.Serializable, pgx.ReadUncommitted:
	default:
		return nil, errors.New("postgres: invalid isolation")
	}
	mode := pgx.ReadWrite
	if o.ReadOnly {
		mode = pgx.ReadOnly
	}
	t, e := c.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: iso, AccessMode: mode})
	db.ReportTransaction(ctx, c.hooks, "begin", start, e)
	if e != nil {
		return nil, Classify(e)
	}
	return &Tx{tx: t, hooks: c.hooks}, nil
}

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (t *Tx) Native() pgx.Tx { return t.tx }

// Commit commits the transaction and reports commit errors.
func (t *Tx) Commit(ctx context.Context) error {
	start := time.Now()
	e := t.tx.Commit(ctx)
	if !errors.Is(e, pgx.ErrTxClosed) {
		db.ReportTransaction(ctx, t.hooks, "commit", start, e)
	}
	return Classify(e)
}

// Rollback rolls back the transaction.
func (t *Tx) Rollback(ctx context.Context) error {
	start := time.Now()
	e := t.tx.Rollback(ctx)
	if !errors.Is(e, pgx.ErrTxClosed) {
		db.ReportTransaction(ctx, t.hooks, "rollback", start, e)
	}
	return Classify(e)
}

// Exec executes a parameterized SQL command and returns its result.
func (t *Tx) Exec(ctx context.Context, q string, a ...any) (Result, error) {
	r, e := t.tx.Exec(ctx, q, bindArgs(a)...)
	return Result{r.RowsAffected(), r.String()}, Classify(e)
}

// Query opens a row stream; the caller must close it.
func (t *Tx) Query(ctx context.Context, q string, a ...any) (db.Rows, error) {
	r, e := t.tx.Query(ctx, q, bindArgs(a)...)
	if e != nil {
		return nil, Classify(e)
	}
	return &rows{r, func() {}}, nil
}

// QueryRow returns a single-row scanner; call Scan to execute or observe its error.
func (t *Tx) QueryRow(ctx context.Context, q string, a ...any) db.Row {
	return &row{scan: func(dst ...any) error { return Classify(t.tx.QueryRow(ctx, q, bindArgs(a)...).Scan(dst...)) }}
}

// Get reads one value into the requested representation.
func (t *Tx) Get(ctx context.Context, d any, q string, a ...any) error {
	r, e := t.Query(ctx, q, bindArgs(a)...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, d, false)
}

// Select scans all result rows into a slice pointer.
func (t *Tx) Select(ctx context.Context, d any, q string, a ...any) error {
	r, e := t.Query(ctx, q, bindArgs(a)...)
	if e != nil {
		return e
	}
	defer r.Close()
	return db.Scan(r, d, true)
}
func within(ctx context.Context, t *Tx, fn func(*Tx) error) (err error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		e := t.Rollback(cleanup)
		if e != nil && !errors.Is(e, pgx.ErrTxClosed) {
			err = errors.Join(err, e)
		}
	}()
	if err = fn(t); err != nil {
		return err
	}
	return t.Commit(ctx)
}

// WithinTx runs a callback transactionally, rolling back on failure or panic.
func (c *Client) WithinTx(ctx context.Context, o TxOptions, fn func(*Tx) error) error {
	if fn == nil {
		return errors.New("postgres: nil transaction callback")
	}
	t, e := c.BeginTx(ctx, o)
	if e != nil {
		return e
	}
	return within(ctx, t, fn)
}

// WithinTx opens a nested savepoint, using pgx logical transactions.
func (t *Tx) WithinTx(ctx context.Context, fn func(*Tx) error) error {
	if fn == nil {
		return errors.New("postgres: nil transaction callback")
	}
	start := time.Now()
	n, e := t.tx.Begin(ctx)
	db.ReportTransaction(ctx, t.hooks, "savepoint", start, e)
	if e != nil {
		return e
	}
	return within(ctx, &Tx{tx: n, hooks: t.hooks}, fn)
}

// Operation contains a parameterized SQL command and its arguments.
type Operation struct {
	SQL  string
	Args []any
}

// Batch sends commands in one protocol batch; all commands execute in an implicit transaction.
func (c *Client) Batch(ctx context.Context, ops []Operation) (out []Result, err error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	b := &pgx.Batch{}
	for _, o := range ops {
		b.Queue(o.SQL, bindArgs(o.Args)...)
	}
	r := c.pool.SendBatch(ctx, b)
	defer func() { err = errors.Join(err, r.Close()) }()
	for range ops {
		v, e := r.Exec()
		if e != nil {
			return nil, Classify(e)
		}
		out = append(out, Result{v.RowsAffected(), v.String()})
	}
	return out, nil
}

// CopyFrom performs PostgreSQL COPY ingestion with safely quoted identifiers.
func (c *Client) CopyFrom(ctx context.Context, table []string, columns []string, values [][]any) (int64, error) {
	ctx, cancel := c.ctx(ctx)
	defer cancel()
	n, e := c.pool.CopyFrom(ctx, pgx.Identifier(table), columns, pgx.CopyFromRows(values))
	return n, Classify(e)
}

// Listen owns a dedicated pool connection until ctx ends. The callback must return promptly.
func (c *Client) Listen(ctx context.Context, channel string, handler func(string) error) error {
	if channel == "" || handler == nil {
		return errors.New("postgres: invalid listener")
	}
	conn, e := c.pool.Acquire(ctx)
	if e != nil {
		return e
	}
	defer conn.Release()
	q := "LISTEN " + pgx.Identifier{channel}.Sanitize()
	if _, e = conn.Exec(ctx, q); e != nil {
		return e
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if _, e := conn.Exec(cleanup, "UNLISTEN *"); e != nil {
			_ = conn.Conn().Close(cleanup)
		}
	}()
	for {
		n, e := conn.Conn().WaitForNotification(ctx)
		if e != nil {
			return e
		}
		if e = handler(n.Payload); e != nil {
			return e
		}
	}
}

// Notify sends a bound PostgreSQL notification payload.
func (c *Client) Notify(ctx context.Context, channel, payload string) error {
	_, e := c.Exec(ctx, "SELECT pg_notify($1,$2)", channel, payload)
	return e
}

// AdvisoryLock holds a session-level lock on a dedicated connection. Release is mandatory.
func (c *Client) AdvisoryLock(ctx context.Context, key int64) (func(context.Context) error, error) {
	conn, e := c.pool.Acquire(ctx)
	if e != nil {
		return nil, e
	}
	var ok bool
	e = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&ok)
	if e != nil || !ok {
		conn.Release()
		if e == nil {
			e = errors.New("postgres: advisory lock busy")
		}
		return nil, e
	}
	return func(ctx context.Context) error {
		defer conn.Release()
		_, e := conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", key)
		if e != nil {
			_ = conn.Conn().Close(ctx)
		}
		return e
	}, nil
}

// Classify maps supported native error codes while preserving the original cause.
func Classify(err error) error {
	if err == nil {
		return nil
	}
	var p *pgconn.PgError
	if errors.As(err, &p) {
		cat, status, code, retry := app.Database, 500, "DATABASE_ERROR", false
		switch p.Code {
		case "23505":
			cat, status, code = app.Conflict, 409, "DUPLICATE_KEY"
		case "23503", "23502", "23514":
			cat, status, code = app.Validation, 422, "CONSTRAINT_VIOLATION"
		case "40001", "40P01":
			code, retry = "TRANSACTION_RETRY", true
		}
		return app.Wrap(err, "We could not complete your request.", app.WithCategory(cat), app.WithCode(code), app.WithHTTPStatus(status), app.WithRetryable(retry), app.WithDebug("SQLSTATE "+p.Code))
	}
	return err
}

// SQLDB exposes a database/sql bridge for advanced SQL integrations. Close the bridge before closing this Client.
func (c *Client) SQLDB() *sql.DB { return stdlib.OpenDBFromPool(c.pool) }
