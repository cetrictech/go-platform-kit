// Package encodingx provides bounded serialization with typed errors.
package encodingx

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Serializer encodes and decodes transport representations.
type Serializer interface {
	Encode(any) ([]byte, error)
	Decode([]byte, any) error
}

// Error contains a typed serialization failure with its underlying cause.
type Error struct {
	Operation string
	Cause     error
}

// Error returns a diagnostic error description.
func (e *Error) Error() string { return fmt.Sprintf("encodingx %s: %v", e.Operation, e.Cause) }

// Unwrap returns the underlying wrapped resource or error cause.
func (e *Error) Unwrap() error { return e.Cause }

// JSON implements bounded standard JSON serialization.
type JSON struct{ MaxBytes int }

func (j JSON) limit() int {
	if j.MaxBytes <= 0 {
		return 1 << 20
	}
	return j.MaxBytes
}

// Encode serializes a value and enforces its configured size limit.
func (j JSON) Encode(v any) ([]byte, error) {
	b, e := json.Marshal(v)
	if e == nil && len(b) > j.limit() {
		e = errors.New("size limit exceeded")
	}
	if e != nil {
		return nil, &Error{"encode", e}
	}
	return b, nil
}

// Decode decodes a size-bounded representation into the destination.
func (j JSON) Decode(b []byte, v any) error {
	if len(b) > j.limit() {
		return &Error{"decode", errors.New("size limit exceeded")}
	}
	if e := json.Unmarshal(b, v); e != nil {
		return &Error{"decode", e}
	}
	return nil
}

// Raw copies bounded byte representations.
type Raw struct{ MaxBytes int }

// Encode serializes a value and enforces its configured size limit.
func (r Raw) Encode(v any) ([]byte, error) {
	b, ok := v.([]byte)
	if !ok {
		return nil, &Error{"encode", errors.New("expected []byte")}
	}
	limit := r.MaxBytes
	if limit <= 0 {
		limit = 1 << 20
	}
	if len(b) > limit {
		return nil, &Error{"encode", errors.New("size limit exceeded")}
	}
	return append([]byte(nil), b...), nil
}

// Decode decodes a size-bounded representation into the destination.
func (r Raw) Decode(b []byte, v any) error {
	p, ok := v.(*[]byte)
	if !ok || p == nil {
		return &Error{"decode", errors.New("expected non-nil *[]byte")}
	}
	a, e := r.Encode(b)
	if e == nil {
		*p = a
	}
	return e
}
