package pagination

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/id"
	"strings"
	"time"
)

// KeyType fixes each decoded cursor parameter's type; no float64 intermediary is used for integer IDs.
type KeyType string

const (
	KeyString  KeyType = "string"
	KeyInt64   KeyType = "int64"
	KeyFloat64 KeyType = "float64"
	KeyTime    KeyType = "time"
	KeyUUID    KeyType = "uuid"
	KeyBool    KeyType = "bool"
	KeyBytes   KeyType = "bytes"
)

// Query describes a developer-owned SQL template containing exactly one {{where}}, {{order}} and {{limit}} marker.
// Args bind base filters, whose placeholders must precede generated placeholders. Never put user input in Template or Columns.
type Query struct {
	Dialect, Template, Identity, Filter string
	Args                                []any
	Columns                             map[string]string
	Sort                                []Sort
	KeyTypes                            []KeyType
}

// SelectFunc matches both SQL adapters without importing either driver. It must fill a slice pointer and close its streams.
type SelectFunc func(context.Context, any, string, ...any) error

// CursorResult contains final presentation order and signed navigation cursors.
type CursorResult[T any] struct {
	Data []T        `json:"data"`
	Meta CursorMeta `json:"meta"`
}

func decodeKeys(raw []json.RawMessage, types []KeyType) ([]any, error) {
	if len(raw) != len(types) {
		return nil, errors.New("pagination: cursor key arity mismatch")
	}
	out := make([]any, len(raw))
	for i, b := range raw {
		if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
			continue
		}
		var target any
		switch types[i] {
		case KeyString:
			target = new(string)
		case KeyInt64:
			target = new(int64)
		case KeyFloat64:
			target = new(float64)
		case KeyBool:
			target = new(bool)
		case KeyBytes:
			target = new([]byte)
		case KeyTime, KeyUUID:
			target = new(string)
		default:
			return nil, errors.New("pagination: unsupported cursor key type")
		}
		if e := json.Unmarshal(b, target); e != nil {
			return nil, errors.New("pagination: cursor key type mismatch")
		}
		switch v := target.(type) {
		case *string:
			if types[i] == KeyTime {
				t, e := time.Parse(time.RFC3339Nano, *v)
				if e != nil {
					return nil, errors.New("pagination: invalid time key")
				}
				out[i] = t
			} else if types[i] == KeyUUID {
				u, e := id.Parse(*v)
				if e != nil {
					return nil, errors.New("pagination: invalid UUID key")
				}
				out[i] = u
			} else {
				out[i] = *v
			}
		case *int64:
			out[i] = *v
		case *float64:
			out[i] = *v
		case *bool:
			out[i] = *v
		case *[]byte:
			out[i] = *v
		}
	}
	return out, nil
}
func validateQuery(q Query) error {
	if q.Identity == "" || len(q.Sort) == 0 || len(q.Sort) != len(q.KeyTypes) {
		return errors.New("pagination: query identity, sorts and matching key types required")
	}
	for _, m := range []string{"{{where}}", "{{order}}", "{{limit}}"} {
		if strings.Count(q.Template, m) != 1 {
			return fmt.Errorf("pagination: query must contain exactly one %s marker", m)
		}
	}
	for _, t := range q.KeyTypes {
		switch t {
		case KeyString, KeyInt64, KeyFloat64, KeyTime, KeyUUID, KeyBool, KeyBytes:
		default:
			return errors.New("pagination: unsupported cursor key type")
		}
	}
	return nil
}

// FetchCursor executes limit+1 rows and implements genuine reverse navigation by reversing SQL sort and presentation order.
// Keys must return values in Sort order. Count is optional and must count the same server-bound filter set.
func FetchCursor[T any](ctx context.Context, selectRows SelectFunc, q Query, request CursorRequest, codec Codec, keys func(T) ([]any, error), count func(context.Context) (int64, error)) (CursorResult[T], error) {
	out := CursorResult[T]{Data: []T{}, Meta: CursorMeta{Type: "cursor", Limit: request.Limit}}
	if selectRows == nil || keys == nil || request.Limit < 1 || request.Limit > 1000 {
		return out, errors.New("pagination: invalid fetch parameters")
	}
	if e := validateQuery(q); e != nil {
		return out, e
	}
	var values []any
	previous := false
	hadCursor := request.Cursor != ""
	if hadCursor {
		v, e := codec.DecodeCursorContext(ctx, request.Cursor, q.Identity, q.Filter)
		if e != nil {
			return out, e
		}
		previous = v.Direction == "previous"
		values, e = decodeKeys(v.Keys, q.KeyTypes)
		if e != nil {
			return out, e
		}
	}
	seek, e := Keyset(q.Dialect, q.Columns, q.Sort, values, previous, len(q.Args))
	if e != nil {
		return out, e
	}
	where := seek.Where
	if where == "" {
		where = "TRUE"
	}
	args := append(append([]any(nil), q.Args...), seek.Args...)
	args = append(args, request.Limit+1)
	limit := "?"
	if q.Dialect == "postgres" {
		limit = fmt.Sprintf("$%d", len(args))
	}
	sql := strings.NewReplacer("{{where}}", where, "{{order}}", seek.OrderBy, "{{limit}}", limit).Replace(q.Template)
	var rows []T
	if e = selectRows(ctx, &rows, sql, args...); e != nil {
		return out, e
	}
	extra := len(rows) > request.Limit
	if extra {
		rows = rows[:request.Limit]
	}
	if previous {
		for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
			rows[i], rows[j] = rows[j], rows[i]
		}
	}
	if len(rows) > 0 {
		out.Data = rows
		out.Meta.HasNext = extra
		out.Meta.HasPrevious = hadCursor
		if previous {
			out.Meta.HasPrevious = extra
			out.Meta.HasNext = hadCursor
		}
		encode := func(row T, direction string) (string, error) {
			v, e := keys(row)
			if e != nil {
				return "", e
			}
			if len(v) != len(q.Sort) {
				return "", errors.New("pagination: extracted key arity mismatch")
			}
			raw := make([]json.RawMessage, len(v))
			for i, value := range v {
				b, e := json.Marshal(value)
				if e != nil {
					return "", e
				}
				raw[i] = b
			}
			if _, e = decodeKeys(raw, q.KeyTypes); e != nil {
				return "", e
			}
			return codec.EncodeCursorContext(ctx, Cursor{Direction: direction, Keys: raw, Query: q.Identity, Filter: q.Filter})
		}
		if out.Meta.HasNext {
			out.Meta.Next, e = encode(rows[len(rows)-1], "next")
			if e != nil {
				return out, e
			}
		}
		if out.Meta.HasPrevious {
			out.Meta.Previous, e = encode(rows[0], "previous")
			if e != nil {
				return out, e
			}
		}
	}
	if count != nil {
		n, e := count(ctx)
		if e != nil {
			return out, e
		}
		if n < 0 {
			return out, errors.New("pagination: negative total count")
		}
		out.Meta.Total = &n
	}
	return out, nil
}

// OffsetSQL expands explicit {{limit}}/{{offset}} markers with bound values and validates configured offset limits.
func OffsetSQL(dialect, template string, args []any, p Offset, options ...OffsetConfig) (string, []any, error) {
	if e := ValidateOffset(p, options...); e != nil {
		return "", nil, e
	}
	if dialect != "postgres" && dialect != "mysql" {
		return "", nil, errors.New("pagination: unsupported dialect")
	}
	if strings.Count(template, "{{limit}}") != 1 || strings.Count(template, "{{offset}}") != 1 {
		return "", nil, errors.New("pagination: offset markers required exactly once")
	}
	limit, offset := "?", "?"
	if dialect == "postgres" {
		limit = fmt.Sprintf("$%d", len(args)+1)
		offset = fmt.Sprintf("$%d", len(args)+2)
	}
	bound := append(append([]any(nil), args...), p.Limit, p.Skip())
	return strings.NewReplacer("{{limit}}", limit, "{{offset}}", offset).Replace(template), bound, nil
}
