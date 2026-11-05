package database

import (
	"database/sql"
	"testing"
)

type testRows struct{ done bool }

func (r *testRows) Next() bool {
	if r.done {
		return false
	}
	r.done = true
	return true
}
func (*testRows) Scan(a ...any) error        { *a[0].(*int) = 42; return nil }
func (*testRows) Columns() ([]string, error) { return []string{"id"}, nil }
func (*testRows) Err() error                 { return nil }
func (*testRows) Close() error               { return nil }
func TestScan(t *testing.T) {
	var dst []struct {
		ID int `db:"id"`
	}
	if e := Scan(&testRows{}, &dst, true); e != nil || len(dst) != 1 || dst[0].ID != 42 {
		t.Fatal(dst, e)
	}
	var n int
	if e := Scan(&testRows{done: true}, &n, false); e != sql.ErrNoRows {
		t.Fatal(e)
	}
}

func BenchmarkStructScanning(b *testing.B) {
	for b.Loop() {
		var dst struct{ ID int }
		_ = Scan(&testRows{}, &dst, false)
	}
}
