package database

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/cetrictech/go-platform-kit/id"
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
func TestScanTimeAndID(t *testing.T) {
	user := "f47ac10b-58cc-4372-a567-0e02b2c3d479"
	parsed, e := id.Parse(user)
	if e != nil {
		t.Fatal(e)
	}
	type model struct {
		Created time.Time  `db:"created"`
		When    *time.Time `db:"when_at"`
		User    id.UUID    `db:"user_id"`
		Owner   *id.UUID   `db:"owner_id"`
	}
	when := []byte("2026-11-05 15:04:05.5")
	rows := &memoryRows{columns: []string{"created", "when_at", "user_id", "owner_id"}, values: [][]any{{
		"2026-11-05T00:00:00.123456789Z", when, user, append([]byte(nil), parsed[:]...),
	}}}
	var got []model
	if e = Scan(rows, &got, true); e != nil || len(got) != 1 {
		t.Fatal(got, e)
	}
	wantCreated := time.Date(2026, 11, 5, 0, 0, 0, 123456789, time.UTC)
	wantWhen := time.Date(2026, 11, 5, 15, 4, 5, 500000000, time.UTC)
	if !got[0].Created.Equal(wantCreated) || got[0].When == nil || !got[0].When.Equal(wantWhen) || got[0].User != parsed || got[0].Owner == nil || *got[0].Owner != parsed {
		t.Fatal(got)
	}
	rows = &memoryRows{columns: []string{"created", "when_at", "user_id", "owner_id"}, values: [][]any{{time.Now().UTC(), nil, []byte(nil), nil}}}
	var one model
	if e = Scan(rows, &one, false); e != nil || one.When != nil || one.Owner != nil || one.User != (id.UUID{}) {
		t.Fatal(one, e)
	}
	rows = &memoryRows{columns: []string{"created"}, values: [][]any{{nil}}}
	if e = Scan(rows, &one.Created, false); e == nil || one.Created.IsZero() {
		t.Fatal(one.Created, e)
	}
	rows = &memoryRows{columns: []string{"user_id"}, values: [][]any{{"bad"}}}
	if e = Scan(rows, &one.User, false); e == nil {
		t.Fatal("invalid UUID accepted")
	}
	rows = &memoryRows{columns: []string{"created"}, values: [][]any{{"yesterday"}}}
	var created time.Time
	if e = Scan(rows, &created, false); e == nil || !created.IsZero() {
		t.Fatal(created, e)
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
