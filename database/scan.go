// Package database defines shared row scanning contracts for SQL adapters.
package database

import (
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

// Rows owns a result stream. Call Close even when scanning stops early.
type Rows interface {
	Next() bool
	Scan(...any) error
	Columns() ([]string, error)
	Err() error
	Close() error
}

// Row scans a single SQL result row.
type Row interface{ Scan(...any) error }

// Scan maps columns to exported fields by db tag or case-insensitive field name. Unknown columns are discarded.
func Scan(rows Rows, destination any, many bool) error {
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
	count := 0
	for rows.Next() {
		var target reflect.Value
		if many {
			target = reflect.New(v.Type().Elem()).Elem()
			if target.Kind() == reflect.Pointer {
				target.Set(reflect.New(target.Type().Elem()))
			}
		} else {
			target = v
		}
		base := target
		for base.Kind() == reflect.Pointer {
			if base.IsNil() {
				base.Set(reflect.New(base.Type().Elem()))
			}
			base = base.Elem()
		}
		args := make([]any, len(cols))
		if base.Kind() == reflect.Struct && base.Type() != reflect.TypeOf(time.Time{}) && !(base.CanAddr() && isScanner(base.Addr().Interface())) {
			fields := map[string]reflect.Value{}
			collect(base, fields)
			for i, c := range cols {
				if f, ok := fields[strings.ToLower(c)]; ok && f.CanAddr() {
					args[i] = f.Addr().Interface()
				} else {
					args[i] = new(any)
				}
			}
		} else {
			if len(cols) != 1 {
				return fmt.Errorf("database: scalar requires 1 column, got %d", len(cols))
			}
			args[0] = base.Addr().Interface()
		}
		if e = rows.Scan(args...); e != nil {
			return e
		}
		count++
		if many {
			v.Set(reflect.Append(v, target))
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
	return nil
}
func isScanner(v any) bool { _, ok := v.(sql.Scanner); return ok }
func collect(v reflect.Value, out map[string]reflect.Value) {
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.PkgPath != "" {
			continue
		}
		x := v.Field(i)
		tag := f.Tag.Get("db")
		if tag == "-" {
			continue
		}
		if f.Anonymous && x.Kind() == reflect.Struct {
			collect(x, out)
			continue
		}
		if tag == "" {
			tag = f.Name
		}
		out[strings.ToLower(tag)] = x
	}
}
