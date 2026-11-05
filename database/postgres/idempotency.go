package postgres

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/idempotency"
	"github.com/cetrictech/go-platform-kit/security"
	"github.com/jackc/pgx/v5"
	"time"
)

// IdempotencySchema must be applied explicitly by the application's migration system. No constructor runs DDL.
const IdempotencySchema = `CREATE TABLE IF NOT EXISTS sdk_idempotency (
 key TEXT PRIMARY KEY, fingerprint TEXT NOT NULL, token TEXT NOT NULL,
 state TEXT NOT NULL CHECK(state IN ('processing','completed','failed')),
 status INTEGER NOT NULL DEFAULT 0, expires_at TIMESTAMPTZ NOT NULL);
 CREATE INDEX IF NOT EXISTS sdk_idempotency_expiration ON sdk_idempotency(expires_at);`

// IdempotencyStore implements atomic durable ownership leases using the database server clock.
// Expired owners cannot complete; external effects still require authoritative unique business keys or fencing.
type IdempotencyStore struct{ client *Client }

// NewIdempotencyStore borrows an existing client. Close the client only after all operations have drained.
func NewIdempotencyStore(c *Client) (*IdempotencyStore, error) {
	if c == nil || c.pool == nil {
		return nil, errors.New("postgres: idempotency client required")
	}
	return &IdempotencyStore{c}, nil
}

// Begin acquires a new/expired/failed lease using one conditional UPSERT. Active/completed conflicts retain their fingerprint.
func (s *IdempotencyStore) Begin(ctx context.Context, key, fingerprint string, lease time.Duration) (idempotency.Record, error) {
	if key == "" || len(key) > 4096 || fingerprint == "" || len(fingerprint) > 4096 || lease < time.Millisecond {
		return idempotency.Record{}, errors.New("postgres: invalid idempotency lease")
	}
	token, e := security.Token(32)
	if e != nil {
		return idempotency.Record{}, e
	}
	var r idempotency.Record
	e = s.client.pool.QueryRow(ctx, `INSERT INTO sdk_idempotency(key,fingerprint,token,state,status,expires_at)
 VALUES($1,$2,$3,'processing',0,clock_timestamp()+$4*interval '1 millisecond')
 ON CONFLICT(key) DO UPDATE SET fingerprint=EXCLUDED.fingerprint,token=EXCLUDED.token,state='processing',status=0,expires_at=EXCLUDED.expires_at
 WHERE sdk_idempotency.expires_at<=clock_timestamp() OR (sdk_idempotency.state='failed' AND sdk_idempotency.fingerprint=EXCLUDED.fingerprint)
 RETURNING fingerprint,token,state,status,expires_at`, key, fingerprint, token, lease.Milliseconds()).Scan(&r.Fingerprint, &r.Token, &r.State, &r.Status, &r.Expires)
	if e == nil {
		return r, nil
	}
	if !errors.Is(e, pgx.ErrNoRows) {
		return r, Classify(e)
	}
	e = s.client.pool.QueryRow(ctx, `SELECT fingerprint,token,state,status,expires_at FROM sdk_idempotency WHERE key=$1`, key).Scan(&r.Fingerprint, &r.Token, &r.State, &r.Status, &r.Expires)
	if e != nil {
		return r, Classify(e)
	}
	if r.Fingerprint != fingerprint {
		return r, idempotency.ErrMismatch
	}
	if r.State == idempotency.Completed {
		return r, idempotency.ErrCompleted
	}
	return r, idempotency.ErrBusy
}
func (s *IdempotencyStore) change(ctx context.Context, key, token string, state idempotency.State, status int, ttl time.Duration) error {
	if key == "" || token == "" || ttl < time.Millisecond {
		return errors.New("postgres: invalid idempotency transition")
	}
	r, e := s.client.pool.Exec(ctx, `UPDATE sdk_idempotency SET state=$3,status=$4,expires_at=clock_timestamp()+$5*interval '1 millisecond'
 WHERE key=$1 AND token=$2 AND state='processing' AND expires_at>clock_timestamp()`, key, token, string(state), status, ttl.Milliseconds())
	if e != nil {
		return Classify(e)
	}
	if r.RowsAffected() != 1 {
		return idempotency.ErrOwnership
	}
	return nil
}

// Complete persists status metadata only for the current unexpired owner.
func (s *IdempotencyStore) Complete(ctx context.Context, key, token string, status int, ttl time.Duration) error {
	if status < 100 || status > 599 {
		return errors.New("postgres: invalid completion status")
	}
	return s.change(ctx, key, token, idempotency.Completed, status, ttl)
}

// Fail allows a matching fingerprint to retry with a new ownership token.
func (s *IdempotencyStore) Fail(ctx context.Context, key, token string, ttl time.Duration) error {
	return s.change(ctx, key, token, idempotency.Failed, 0, ttl)
}

// Renew extends an unexpired processing owner using the authoritative server clock.
func (s *IdempotencyStore) Renew(ctx context.Context, key, token string, ttl time.Duration) error {
	return s.change(ctx, key, token, idempotency.Processing, 0, ttl)
}

// CleanupExpired deletes at most limit expired records without blocking active owners or other cleanup workers.
func (s *IdempotencyStore) CleanupExpired(ctx context.Context, limit int) (int64, error) {
	if limit < 1 || limit > 10000 {
		return 0, errors.New("postgres: cleanup limit must be 1..10000")
	}
	r, e := s.client.pool.Exec(ctx, `WITH expired AS(SELECT key FROM sdk_idempotency WHERE expires_at<=clock_timestamp() ORDER BY expires_at LIMIT $1 FOR UPDATE SKIP LOCKED) DELETE FROM sdk_idempotency d USING expired e WHERE d.key=e.key`, limit)
	return r.RowsAffected(), Classify(e)
}

var _ idempotency.Store = (*IdempotencyStore)(nil)
