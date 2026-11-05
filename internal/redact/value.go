package redact

import (
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strings"
	"time"
)

// SafeValue handles typed JSON-like values, secret tags and cycles with a bounded traversal budget.
// It deliberately avoids invoking arbitrary MarshalJSON/String methods on structs.
func SafeValue(v any) any { budget := 10000; return walk(reflect.ValueOf(v), 0, &budget) }
func walk(v reflect.Value, depth int, budget *int) any {
	if !v.IsValid() {
		return nil
	}
	if depth > 64 || *budget <= 0 {
		return "[TRUNCATED]"
	}
	*budget--
	for v.Kind() == reflect.Interface || v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
		depth++
		if depth > 64 {
			return "[TRUNCATED]"
		}
	}
	if v.CanInterface() {
		switch x := v.Interface().(type) {
		case rsa.PrivateKey, ecdsa.PrivateKey:
			return "[REDACTED_PRIVATE_KEY]"
		case time.Time:
			return x.UTC().Format(time.RFC3339Nano)
		case url.URL:
			return SafeURL(x)
		case json.Number:
			return x
		case json.RawMessage:
			if len(x) > 1<<20 {
				return "[TRUNCATED]"
			}
			var decoded any
			if e := json.Unmarshal(x, &decoded); e != nil {
				return "[INVALID_JSON]"
			}
			return walk(reflect.ValueOf(decoded), depth+1, budget)
		}
	}
	switch v.Kind() {
	case reflect.Struct:
		m := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			if *budget <= 0 {
				m["_truncated"] = "[TRUNCATED]"
				break
			}
			f := v.Type().Field(i)
			if f.PkgPath != "" {
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			if Sensitive(name) || f.Tag.Get("secret") == "true" {
				m[name] = "[REDACTED]"
			} else {
				m[name] = walk(v.Field(i), depth+1, budget)
			}
		}
		return m
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		m := make(map[string]any, min(v.Len(), *budget))
		iter := v.MapRange()
		for iter.Next() {
			if *budget <= 0 {
				m["_truncated"] = "[TRUNCATED]"
				break
			}
			k := iter.Key()
			var name string
			switch k.Kind() {
			case reflect.String:
				name = k.String()
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				name = fmt.Sprint(k.Int())
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				name = fmt.Sprint(k.Uint())
			default:
				name = "[UNSUPPORTED_KEY]"
			}
			if Sensitive(name) {
				m[name] = "[REDACTED]"
				*budget--
			} else {
				m[name] = walk(iter.Value(), depth+1, budget)
			}
		}
		return m
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return "[BINARY]"
		}
		n := min(v.Len(), *budget)
		a := make([]any, n)
		for i := range a {
			a[i] = walk(v.Index(i), depth+1, budget)
		}
		return a
	case reflect.String:
		s := v.String()
		if strings.Contains(s, "://") {
			if u, e := url.Parse(s); e == nil && u.Scheme != "" {
				return SafeURL(*u)
			}
		}
		return SafeText(s)
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint()
	case reflect.Float32, reflect.Float64:
		return v.Float()
	default:
		return "[UNSUPPORTED]"
	}
}

// SafeURL removes passwords and sensitive query values. It returns a copy and never changes the caller's URL.
func SafeURL(u url.URL) string {
	if u.User != nil {
		u.User = url.User("[REDACTED]")
	}
	q := u.Query()
	changed := false
	for k := range q {
		if Sensitive(k) {
			q.Set(k, "[REDACTED]")
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}
