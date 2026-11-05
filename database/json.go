package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// JSON is a nullable JSON SQL value. Use it when scanning a whole JSON column without a tagged struct field.
type JSON[T any] struct {
	Data  T
	Valid bool
}

// Scan accepts database text/bytes or SQL NULL and validates the complete JSON representation.
func (j *JSON[T]) Scan(src any) error {
	if j == nil {
		return fmt.Errorf("database: nil JSON destination")
	}
	if src == nil {
		var zero T
		j.Data = zero
		j.Valid = false
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("database: unsupported JSON source %T", src)
	}
	var value T
	if e := json.Unmarshal(b, &value); e != nil {
		return e
	}
	j.Data = value
	j.Valid = true
	return nil
}

// Value implements driver.Valuer for safe parameter binding.
func (j JSON[T]) Value() (driver.Value, error) {
	if !j.Valid {
		return nil, nil
	}
	return json.Marshal(j.Data)
}
