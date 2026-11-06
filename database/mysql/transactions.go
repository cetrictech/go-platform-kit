package mysql

import (
	"context"
	"database/sql"
	"errors"
	db "github.com/cetrictech/go-platform-kit/database"
	"time"
)

// CommitContext checks cancellation before committing. The SQL transaction remains tied to its BeginTx context.
func (t *Tx) CommitContext(ctx context.Context) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	start := time.Now()
	e := t.tx.Commit()
	if !errors.Is(e, sql.ErrTxDone) {
		db.ReportTransaction(ctx, t.hooks, "commit", start, e)
	}
	return Classify(e)
}

// RollbackContext always attempts cleanup, including for a canceled context. Context is used for diagnostic correlation.
func (t *Tx) RollbackContext(ctx context.Context) error {
	start := time.Now()
	e := t.tx.Rollback()
	if !errors.Is(e, sql.ErrTxDone) {
		db.ReportTransaction(ctx, t.hooks, "rollback", start, e)
	}
	return Classify(e)
}
