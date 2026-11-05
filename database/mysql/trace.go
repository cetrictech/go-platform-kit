package mysql

import (
	db "github.com/cetrictech/go-platform-kit/database"
	"sync"
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
