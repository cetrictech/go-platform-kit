//go:build integration

package postgres

import (
	"context"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/idempotency"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPostgresPrimitivesAndDurableCoordination(t *testing.T) {
	dsn := os.Getenv("POSTGRES_URL")
	if dsn == "" {
		t.Skip("POSTGRES_URL unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, e := New(ctx, Config{URL: dsn, AllowPlaintext: true, MaxConnections: 8})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	var n int
	if e = c.Get(ctx, &n, "SELECT @n::int + 1 /* @ignored */", NamedArgs{"n": 41}); e != nil || n != 42 {
		t.Fatal(n, e)
	}
	if e = c.Get(ctx, &n, "SELECT @missing::int", NamedArgs{"n": 1}); e == nil {
		t.Fatal("missing named parameter accepted")
	}
	table := fmt.Sprintf("gpk_copy_%d", time.Now().UnixNano())
	if _, e = c.Exec(ctx, "CREATE TABLE "+table+" (id BIGINT PRIMARY KEY)"); e != nil {
		t.Fatal(e)
	}
	defer c.Exec(context.Background(), "DROP TABLE "+table)
	if n, e := c.CopyFrom(ctx, []string{table}, []string{"id"}, [][]any{{int64(1)}, {int64(2)}}); e != nil || n != 2 {
		t.Fatal(n, e)
	}
	sentinel := errors.New("nested rollback")
	if e = c.WithinTx(ctx, TxOptions{}, func(tx *Tx) error {
		if e := tx.WithinTx(ctx, func(nested *Tx) error {
			_, e := nested.Exec(ctx, "INSERT INTO "+table+" VALUES (3)")
			if e != nil {
				return e
			}
			return sentinel
		}); !errors.Is(e, sentinel) {
			return fmt.Errorf("savepoint: %w", e)
		}
		_, e := tx.Exec(ctx, "INSERT INTO "+table+" VALUES (4)")
		return e
	}); e != nil {
		t.Fatal(e)
	}
	if e = c.Get(ctx, &n, "SELECT COUNT(*) FROM "+table); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("panic swallowed")
			}
		}()
		_ = c.WithinTx(ctx, TxOptions{}, func(tx *Tx) error { _, _ = tx.Exec(ctx, "INSERT INTO "+table+" VALUES (5)"); panic("rollback") })
	}()
	if e = c.Get(ctx, &n, "SELECT COUNT(*) FROM "+table); e != nil || n != 3 {
		t.Fatal(n, e)
	}
	release, e := c.AdvisoryLock(ctx, 912345)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.AdvisoryLock(ctx, 912345); e == nil {
		t.Fatal("duplicate advisory lock")
	}
	if e = release(ctx); e != nil {
		t.Fatal(e)
	}
	listenCtx, stop := context.WithCancel(ctx)
	defer stop()
	received := make(chan string, 1)
	done := make(chan error, 1)
	channel := fmt.Sprintf("gpk_notify_%d", time.Now().UnixNano())
	go func() {
		done <- c.Listen(listenCtx, channel, func(v string) error {
			select {
			case received <- v:
			default:
			}
			return nil
		})
	}()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if e = c.Notify(ctx, channel, "payload"); e != nil {
				t.Fatal(e)
			}
		case v := <-received:
			if v != "payload" {
				t.Fatal(v)
			}
			stop()
			if e = <-done; !errors.Is(e, context.Canceled) {
				t.Fatal(e)
			}
			goto coordinated
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
coordinated:
	if _, e = c.Exec(ctx, IdempotencySchema); e != nil {
		t.Fatal(e)
	}
	store, e := NewIdempotencyStore(c)
	if e != nil {
		t.Fatal(e)
	}
	key := fmt.Sprintf("gpk-idem-%d", time.Now().UnixNano())
	defer c.Exec(context.Background(), "DELETE FROM sdk_idempotency WHERE key=$1", key)
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := store.Begin(ctx, key, "fingerprint", time.Minute)
			if e == nil {
				won.Add(1)
				if e = store.Renew(ctx, key, r.Token, time.Minute); e != nil {
					t.Error(e)
				}
				if e = store.Complete(ctx, key, r.Token, 201, time.Hour); e != nil {
					t.Error(e)
				}
			} else if !errors.Is(e, idempotency.ErrBusy) && !errors.Is(e, idempotency.ErrCompleted) {
				t.Error(e)
			}
		}()
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatal(won.Load())
	}
	if _, e = store.Begin(ctx, key, "changed", time.Minute); !errors.Is(e, idempotency.ErrMismatch) {
		t.Fatal(e)
	}
	if e = store.Complete(ctx, key, "stale", 200, time.Minute); !errors.Is(e, idempotency.ErrOwnership) {
		t.Fatal(e)
	}
	row := c.QueryRow(ctx, "INSERT INTO "+table+" VALUES (99) RETURNING id")
	if e = row.Scan(&n); e != nil {
		t.Fatal(e)
	}
	if e = row.Scan(&n); e == nil {
		t.Fatal("repeated row Scan re-executed SQL")
	}
}
