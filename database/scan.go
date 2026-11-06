// Package database defines shared scanning, hooks and SQL value contracts without importing drivers.
package database

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/id"
	"reflect"
	"strings"
	"sync"
	"time"
)

// Rows owns a result stream. Close it when stopping before exhaustion.
type Rows interface {
	Next() bool
	Scan(...any) error
	Columns() ([]string, error)
	Err() error
	Close() error
}

// Row scans one result record.
type Row interface{ Scan(...any) error }
type fieldInfo struct {
	path []int
	json bool
}
type structInfo struct {
	fields map[string]fieldInfo
	err    error
}

var schemas sync.Map            // Immutable reflection metadata; no connections or application state.
func columnKey(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }
func schema(t reflect.Type) structInfo {
	if v, ok := schemas.Load(t); ok {
		return v.(structInfo)
	}
	s := structInfo{fields: map[string]fieldInfo{}}
	var walk func(reflect.Type, []int, map[reflect.Type]bool)
	walk = func(t reflect.Type, path []int, parents map[reflect.Type]bool) {
		if parents[t] {
			return
		}
		next := make(map[reflect.Type]bool, len(parents)+1)
		for k, v := range parents {
			next[k] = v
		}
		next[t] = true
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			tag := strings.Split(f.Tag.Get("db"), ",")
			if tag[0] == "-" {
				continue
			}
			p := append(append([]int(nil), path...), i)
			ft := f.Type
			for ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if f.Anonymous && ft.Kind() == reflect.Struct && tag[0] == "" && len(tag) == 1 && !reflect.PointerTo(ft).Implements(reflect.TypeFor[sql.Scanner]()) && ft != reflect.TypeFor[time.Time]() {
				walk(ft, p, next)
				continue
			}
			name := tag[0]
			if name == "" {
				name = f.Name
			}
			key := columnKey(name)
			if _, exists := s.fields[key]; exists {
				s.err = fmt.Errorf("database: ambiguous mapped column %q", name)
				continue
			}
			info := fieldInfo{path: p}
			for _, option := range tag[1:] {
				if option == "json" {
					info.json = true
				} else if option != "" {
					s.err = fmt.Errorf("database: unknown db tag option %q", option)
				}
			}
			s.fields[key] = info
		}
	}
	walk(t, nil, nil)
	v, _ := schemas.LoadOrStore(t, s)
	return v.(structInfo)
}
func fieldAt(v reflect.Value, path []int) reflect.Value {
	for _, i := range path {
		for v.Kind() == reflect.Pointer {
			if v.IsNil() {
				v.Set(reflect.New(v.Type().Elem()))
			}
			v = v.Elem()
		}
		v = v.Field(i)
	}
	return v
}

// Scan maps db tags or case-insensitive snake_case names. Anonymous exported struct pointers are allocated only when mapped.
// The json tag option decodes JSON into a field. Unknown columns are discarded. Select replaces its destination only on success.
func Scan(rows Rows, destination any, many bool) error {
	if rows == nil {
		return errors.New("database: nil rows")
	}
	rv := reflect.ValueOf(rows)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return errors.New("database: nil rows")
	}
	v := reflect.ValueOf(destination)
	if v.Kind() != reflect.Pointer || v.IsNil() {
		return errors.New("database: destination must be a non-nil pointer")
	}
	v = v.Elem()
	if many && v.Kind() != reflect.Slice {
		return errors.New("database: Select requires slice pointer")
	}
	cols, e := rows.Columns()
	if e != nil {
		return e
	}
	result := v
	if many {
		result = reflect.MakeSlice(v.Type(), 0, 0)
	}
	count := 0
	for rows.Next() {
		target := v
		if many {
			target = reflect.New(v.Type().Elem()).Elem()
		}
		base := target
		for base.Kind() == reflect.Pointer {
			if base.IsNil() {
				base.Set(reflect.New(base.Type().Elem()))
			}
			base = base.Elem()
		}
		args := make([]any, len(cols))
		if base.Kind() == reflect.Struct && base.Type() != reflect.TypeFor[time.Time]() && !base.Addr().Type().Implements(reflect.TypeFor[sql.Scanner]()) {
			s := schema(base.Type())
			if s.err != nil {
				return s.err
			}
			for i, c := range cols {
				f, ok := s.fields[columnKey(c)]
				if !ok {
					args[i] = new(any)
					continue
				}
				field := fieldAt(base, f.path)
				if !field.CanAddr() || !field.CanSet() {
					return fmt.Errorf("database: column %q is not addressable", c)
				}
				if f.json {
					args[i] = jsonField{field.Addr().Interface()}
				} else {
					args[i] = scanValue(field)
				}
			}
		} else {
			if len(cols) != 1 {
				return fmt.Errorf("database: scalar requires 1 column, got %d", len(cols))
			}
			args[0] = scanValue(base)
		}
		if e = rows.Scan(args...); e != nil {
			return e
		}
		count++
		if many {
			result = reflect.Append(result, target)
		} else {
			break
		}
	}
	if e = rows.Err(); e != nil {
		return e
	}
	if !many && count == 0 {
		return sql.ErrNoRows
	}
	if many {
		v.Set(result)
	}
	return nil
}

func scanValue(field reflect.Value) any {
	base := field.Type()
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	switch base {
	case reflect.TypeFor[time.Time]():
		return timeScanner{field}
	case reflect.TypeFor[id.UUID]():
		return idScanner{field}
	default:
		return field.Addr().Interface()
	}
}

type timeScanner struct{ field reflect.Value }

func (s timeScanner) Scan(src any) error {
	if src == nil {
		if s.field.Kind() == reflect.Pointer {
			s.field.SetZero()
			return nil
		}
		return errors.New("database: NULL time")
	}
	t, e := timeFrom(src)
	if e != nil {
		return e
	}
	setField(s.field, reflect.ValueOf(t))
	return nil
}

type idScanner struct{ field reflect.Value }

func (s idScanner) Scan(src any) error {
	if src == nil || src == "" || (isEmptyBytes(src)) {
		if s.field.Kind() == reflect.Pointer {
			s.field.SetZero()
			return nil
		}
		setField(s.field, reflect.ValueOf(id.UUID{}))
		return nil
	}
	u, e := uuidFrom(src)
	if e != nil {
		return e
	}
	setField(s.field, reflect.ValueOf(u))
	return nil
}

func isEmptyBytes(src any) bool {
	b, ok := src.([]byte)
	return ok && len(b) == 0
}

func setField(field, value reflect.Value) {
	for field.Kind() == reflect.Pointer {
		if field.IsNil() {
			field.Set(reflect.New(field.Type().Elem()))
		}
		field = field.Elem()
	}
	field.Set(value)
}

func timeFrom(src any) (time.Time, error) {
	switch v := src.(type) {
	case time.Time:
		return v, nil
	case string:
		return parseTime(v)
	case []byte:
		return parseTime(string(v))
	default:
		return time.Time{}, fmt.Errorf("database: unsupported time source %T", src)
	}
}

func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{
		time.RFC3339Nano,
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05Z07:00",
	} {
		if t, e := time.Parse(layout, s); e == nil {
			return t, nil
		}
	}
	for _, layout := range []string{
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02",
	} {
		if t, e := time.ParseInLocation(layout, s, time.UTC); e == nil {
			return t, nil
		}
	}
	return time.Time{}, errors.New("database: invalid time")
}

func uuidFrom(src any) (id.UUID, error) {
	switch v := src.(type) {
	case id.UUID:
		return v, nil
	case []byte:
		if len(v) == len(id.UUID{}) {
			var u id.UUID
			copy(u[:], v)
			return u, nil
		}
		return parseUUID(string(v))
	case string:
		return parseUUID(v)
	default:
		return id.UUID{}, fmt.Errorf("database: unsupported UUID source %T", src)
	}
}

func parseUUID(s string) (id.UUID, error) {
	u, e := id.Parse(strings.TrimSpace(s))
	if e != nil {
		return id.UUID{}, errors.New("database: invalid UUID")
	}
	return u, nil
}

type jsonField struct{ destination any }

func (j jsonField) Scan(src any) error {
	if src == nil {
		return json.Unmarshal([]byte("null"), j.destination)
	}
	switch b := src.(type) {
	case []byte:
		return json.Unmarshal(b, j.destination)
	case string:
		return json.Unmarshal([]byte(b), j.destination)
	default:
		return fmt.Errorf("database: JSON column contains %T", src)
	}
}
