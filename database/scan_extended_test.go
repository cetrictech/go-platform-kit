package database

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
)

type Embedded struct{ CreatedAt int }
type memoryRows struct {
	columns []string
	values  [][]any
	index   int
	err     error
}

func (r *memoryRows) Columns() ([]string, error) { return r.columns, nil }
func (r *memoryRows) Next() bool {
	if r.index >= len(r.values) {
		return false
	}
	r.index++
	return true
}
func (r *memoryRows) Err() error   { return r.err }
func (r *memoryRows) Close() error { return nil }
func (r *memoryRows) Scan(dest ...any) error {
	for i, v := range r.values[r.index-1] {
		if s, ok := dest[i].(sql.Scanner); ok {
			if e := s.Scan(v); e != nil {
				return e
			}
			continue
		}
		x := reflect.ValueOf(dest[i]).Elem()
		if v == nil {
			x.SetZero()
		} else {
			x.Set(reflect.ValueOf(v))
		}
	}
	return nil
}
func TestEmbeddedJSONAndReplacement(t *testing.T) {
	type model struct {
		*Embedded
		Payload map[string]int `db:"payload,json"`
	}
	dst := []*model{{Embedded: &Embedded{CreatedAt: 99}}}
	r := &memoryRows{columns: []string{"created_at", "payload"}, values: [][]any{{42, []byte(`{"n":7}`)}}}
	if e := Scan(r, &dst, true); e != nil || len(dst) != 1 || dst[0].CreatedAt != 42 || dst[0].Payload["n"] != 7 {
		t.Fatal(dst, e)
	}
	before := dst
	r = &memoryRows{columns: []string{"created_at"}, err: errors.New("stream failure")}
	if e := Scan(r, &dst, true); e == nil || !reflect.DeepEqual(before, dst) {
		t.Fatal(dst, e)
	}
}
func TestScanInvalidAndJSON(t *testing.T) {
	if Scan(nil, new(int), false) == nil {
		t.Fatal("nil rows")
	}
	var value JSON[map[string]int]
	if e := value.Scan([]byte(`{"n":1}`)); e != nil || !value.Valid || value.Data["n"] != 1 {
		t.Fatal(value, e)
	}
	if _, e := value.Value(); e != nil {
		t.Fatal(e)
	}
	if e := value.Scan(nil); e != nil || value.Valid {
		t.Fatal(value, e)
	}
	if e := value.Scan(42); e == nil {
		t.Fatal("invalid JSON source")
	}
}
