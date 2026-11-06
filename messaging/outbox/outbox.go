// Package outbox implements SQL transactional event storage and at-least-once Kafka delivery.
package outbox

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/id"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"strings"
	"time"
)

// Record contains persisted processing or event metadata.
type Record struct {
	ID      string
	Message kafka.Message
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(m kafka.Message) (Record, error) {
	u, e := id.NewV7()
	if e != nil {
		return Record{}, e
	}
	if m.Topic == "" {
		return Record{}, errors.New("outbox: topic required")
	}
	return Record{u.String(), m}, nil
}

// InsertSQL returns bound SQL to execute in the SAME transaction as application state.
func InsertSQL(dialect string, r Record) (string, []any, error) {
	if r.ID == "" || r.Message.Topic == "" {
		return "", nil, errors.New("outbox: invalid record")
	}
	b, e := json.Marshal(r.Message)
	if e != nil {
		return "", nil, e
	}
	q := "INSERT INTO sdk_outbox (id, payload, attempts, next_attempt, created_at) VALUES (?, ?, 0, ?, ?)"
	if dialect == "postgres" {
		q = "INSERT INTO sdk_outbox (id, payload, attempts, next_attempt, created_at) VALUES ($1, $2, 0, $3, $4)"
	} else if dialect != "mysql" {
		return "", nil, errors.New("outbox: unsupported dialect")
	}
	now := time.Now().UTC()
	return q, []any{r.ID, b, now, now}, nil
}

// SQLStore uses database row locks and SKIP LOCKED for claims. Run migrations explicitly.
type SQLStore struct {
	DB                     *sql.DB
	Dialect                string
	Producer               kafka.Producer
	BatchSize, MaxAttempts int
	RetryDelay             time.Duration
}

// Dispatch holds row locks through bounded broker publishing and commits delivery state afterward.
// A broker acknowledgment followed by a failed SQL commit can deliver duplicates. Event IDs are propagated for deduplication.
func (s SQLStore) Dispatch(ctx context.Context) (int, error) {
	if s.DB == nil || s.Producer == nil || s.BatchSize < 1 || s.BatchSize > 1000 || s.MaxAttempts < 1 || s.RetryDelay <= 0 || (s.Dialect != "postgres" && s.Dialect != "mysql") {
		return 0, errors.New("outbox: invalid configuration")
	}
	tx, e := s.DB.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	ph := "?"
	if s.Dialect == "postgres" {
		ph = "$1"
	}
	q := fmt.Sprintf("SELECT id,payload,attempts FROM sdk_outbox WHERE delivered_at IS NULL AND dead_at IS NULL AND next_attempt <= %s ORDER BY created_at,id LIMIT %d FOR UPDATE SKIP LOCKED", ph, s.BatchSize)
	rows, e := tx.QueryContext(ctx, q, time.Now().UTC())
	if e != nil {
		return 0, e
	}
	type item struct {
		id       string
		b        []byte
		attempts int
	}
	var items []item
	for rows.Next() {
		var v item
		if e = rows.Scan(&v.id, &v.b, &v.attempts); e != nil {
			rows.Close()
			return 0, e
		}
		items = append(items, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return 0, e
	}
	published := 0
	var failures []error
	for _, v := range items {
		var m kafka.Message
		if e = json.Unmarshal(v.b, &m); e == nil {
			if m.Headers == nil {
				m.Headers = map[string][]byte{}
			}
			m.Headers["event-id"] = []byte(v.id)
			e = s.Producer.Publish(ctx, m)
		}
		now := time.Now().UTC()
		var args []any
		var update string
		if e == nil {
			update = "UPDATE sdk_outbox SET delivered_at=? WHERE id=?"
			args = []any{now, v.id}
			published++
		} else {
			failures = append(failures, e)
			v.attempts++
			if v.attempts >= s.MaxAttempts {
				update = "UPDATE sdk_outbox SET attempts=?,dead_at=? WHERE id=?"
				args = []any{v.attempts, now, v.id}
			} else {
				update = "UPDATE sdk_outbox SET attempts=?,next_attempt=? WHERE id=?"
				args = []any{v.attempts, now.Add(s.RetryDelay), v.id}
			}
		}
		if s.Dialect == "postgres" {
			index := 0
			var b strings.Builder
			for _, c := range update {
				if c == '?' {
					index++
					fmt.Fprintf(&b, "$%d", index)
				} else {
					b.WriteRune(c)
				}
			}
			update = b.String()
		}
		if _, e = tx.ExecContext(ctx, update, args...); e != nil {
			return 0, e
		}
	}
	if e = tx.Commit(); e != nil {
		return 0, e
	}
	return published, errors.Join(failures...)
}

// Cleanup deletes completed records older than cutoff; failed records require explicit operator disposition.
func (s SQLStore) Cleanup(ctx context.Context, cutoff time.Time) (int64, error) {
	if s.DB == nil {
		return 0, errors.New("outbox: database required")
	}
	q := "DELETE FROM sdk_outbox WHERE delivered_at < ?"
	if s.Dialect == "postgres" {
		q = "DELETE FROM sdk_outbox WHERE delivered_at < $1"
	} else if s.Dialect != "mysql" {
		return 0, errors.New("outbox: unsupported dialect")
	}
	r, e := s.DB.ExecContext(ctx, q, cutoff)
	if e != nil {
		return 0, e
	}
	return r.RowsAffected()
}
