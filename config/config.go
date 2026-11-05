// Package config loads explicit, typed application configuration.
package config

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	redaction "github.com/cetrictech/go-platform-kit/internal/redact"
	"io"
	"net/url"
	"os"
	"reflect"
	"strconv"
	"strings"
	"time"
)

// Options controls loading. Precedence is defaults, existing values/file, environment.
type Options struct {
	Prefix   string
	Lookup   func(string) (string, bool)
	File     io.Reader
	Validate func(any) error
}

// Load updates a struct only after all conversions and validation succeed. Tags are env, default, required, secret.
func Load(dst any, options ...Options) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return errors.New("config: destination must be a non-nil struct pointer")
	}
	o := Options{}
	if len(options) > 0 {
		o = options[0]
	}
	if o.Lookup == nil {
		o.Lookup = os.LookupEnv
	}
	tmp := reflect.New(v.Elem().Type())
	tmp.Elem().Set(clone(v.Elem(), map[visit]reflect.Value{}))
	var err error
	if o.File != nil {
		d := json.NewDecoder(o.File)
		d.DisallowUnknownFields()
		if err = d.Decode(tmp.Interface()); err != nil {
			return fmt.Errorf("config: file: %w", err)
		}
		var extra any
		if err = d.Decode(&extra); err != io.EOF {
			return errors.New("config: trailing JSON data")
		}
	}
	if err = load(tmp.Elem(), o.Prefix, o.Lookup); err != nil {
		return err
	}
	if err = Validate(tmp.Interface()); err != nil {
		return err
	}
	if o.Validate != nil {
		if err = o.Validate(tmp.Interface()); err != nil {
			return err
		}
	}
	v.Elem().Set(tmp.Elem())
	return nil
}
func load(v reflect.Value, prefix string, lookup func(string) (string, bool)) error {
	return loadDepth(v, prefix, lookup, 0)
}
func loadDepth(v reflect.Value, prefix string, lookup func(string) (string, bool), depth int) error {
	if depth > 32 {
		return errors.New("config: nesting exceeds 32 levels")
	}
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := t.Field(i)
		x := v.Field(i)
		if !x.CanSet() {
			continue
		}
		key := f.Tag.Get("env")
		if key == "-" {
			continue
		}
		if key == "" {
			key = strings.ToUpper(f.Name)
		}
		key = prefix + key
		group := x
		if x.Kind() == reflect.Pointer && x.Type().Elem().Kind() == reflect.Struct && !x.Type().Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) && x.Type().Elem() != reflect.TypeOf(url.URL{}) {
			if x.IsNil() {
				x.Set(reflect.New(x.Type().Elem()))
			}
			group = x.Elem()
		}
		if group.Kind() == reflect.Struct && group.Type() != reflect.TypeOf(time.Time{}) && group.Type() != reflect.TypeOf(url.URL{}) {
			if err := loadDepth(group, key+"_", lookup, depth+1); err != nil {
				return err
			}
			continue
		}
		raw, ok := lookup(key)
		if !ok && x.IsZero() {
			raw, ok = f.Tag.Lookup("default")
		}
		if ok {
			if err := convert(x, raw); err != nil {
				return fmt.Errorf("config: %s: invalid %s", key, x.Type())
			}
		}
		if f.Tag.Get("required") == "true" && x.IsZero() {
			return fmt.Errorf("config: %s is required", key)
		}
	}
	return nil
}
func convert(v reflect.Value, s string) error {
	if v.Kind() == reflect.Pointer {
		v.Set(reflect.New(v.Type().Elem()))
		return convert(v.Elem(), s)
	}
	if v.CanAddr() {
		if u, ok := v.Addr().Interface().(encoding.TextUnmarshaler); ok {
			return u.UnmarshalText([]byte(s))
		}
	}
	if v.Type() == reflect.TypeOf(time.Duration(0)) {
		d, e := time.ParseDuration(s)
		v.SetInt(int64(d))
		return e
	}
	if v.Type() == reflect.TypeOf(url.URL{}) {
		u, e := url.ParseRequestURI(s)
		if e == nil {
			v.Set(reflect.ValueOf(*u))
		}
		return e
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString(s)
	case reflect.Bool:
		b, e := strconv.ParseBool(s)
		if e != nil {
			return e
		}
		v.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, e := strconv.ParseInt(s, 10, v.Type().Bits())
		if e != nil {
			return e
		}
		v.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, e := strconv.ParseUint(s, 10, v.Type().Bits())
		if e != nil {
			return e
		}
		v.SetUint(n)
	case reflect.Float32, reflect.Float64:
		n, e := strconv.ParseFloat(s, v.Type().Bits())
		if e != nil {
			return e
		}
		v.SetFloat(n)
	case reflect.Slice:
		parts := strings.Split(s, ",")
		if s == "" {
			parts = nil
		}
		out := reflect.MakeSlice(v.Type(), len(parts), len(parts))
		for i, p := range parts {
			if e := convert(out.Index(i), strings.TrimSpace(p)); e != nil {
				return e
			}
		}
		v.Set(out)
	default:
		return errors.New("unsupported type")
	}
	return nil
}

// Validate invokes an explicit validation hook.
func Validate(v any) error {
	if x, ok := v.(interface{ Validate() error }); ok {
		return x.Validate()
	}
	return nil
}

// Dump returns JSON with secret-tagged and sensitive-name fields redacted, recursively.
func Dump(v any) ([]byte, error) { return json.Marshal(redact(reflect.ValueOf(v))) }
func redact(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		return redact(v.Elem())
	}
	switch v.Kind() {
	case reflect.Struct:
		m := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if f.PkgPath != "" {
				continue
			}
			n := f.Name
			if f.Tag.Get("secret") == "true" || redaction.Sensitive(n) {
				m[n] = "[REDACTED]"
			} else {
				m[n] = redact(v.Field(i))
			}
		}
		return m
	case reflect.Map:
		m := map[string]any{}
		it := v.MapRange()
		for it.Next() {
			k := fmt.Sprint(it.Key().Interface())
			if redaction.Sensitive(k) {
				m[k] = "[REDACTED]"
			} else {
				m[k] = redact(it.Value())
			}
		}
		return m
	case reflect.Slice, reflect.Array:
		a := make([]any, v.Len())
		for i := range a {
			a[i] = redact(v.Index(i))
		}
		return a
	default:
		return v.Interface()
	}
}

// GetString reads an environment variable, returning the fallback when absent.
func GetString(k, f string) string {
	if v, ok := os.LookupEnv(k); ok {
		return v
	}
	return f
}

// GetInt reads an integer environment variable, returning the fallback on invalid input.
func GetInt(k string, f int) int {
	v, e := strconv.Atoi(os.Getenv(k))
	if e != nil {
		return f
	}
	return v
}

// GetBool reads a boolean environment variable, returning the fallback on invalid input.
func GetBool(k string, f bool) bool {
	v, e := strconv.ParseBool(os.Getenv(k))
	if e != nil {
		return f
	}
	return v
}

// GetDuration parses an environment duration, returning the fallback on invalid input.
func GetDuration(k string, f time.Duration) time.Duration {
	v, e := time.ParseDuration(os.Getenv(k))
	if e != nil {
		return f
	}
	return v
}

// GetStringSlice reads comma-separated environment values or copies the fallback.
func GetStringSlice(k string, f []string) []string {
	v, ok := os.LookupEnv(k)
	if !ok {
		return append([]string(nil), f...)
	}
	if v == "" {
		return []string{}
	}
	a := strings.Split(v, ",")
	for i := range a {
		a[i] = strings.TrimSpace(a[i])
	}
	return a
}

type visit struct {
	typ reflect.Type
	ptr uintptr
}

func clone(v reflect.Value, seen map[visit]reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		key := visit{v.Type(), v.Pointer()}
		if old, ok := seen[key]; ok {
			return old
		}
		out := reflect.New(v.Type().Elem())
		seen[key] = out
		out.Elem().Set(clone(v.Elem(), seen))
		return out
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.New(v.Type()).Elem()
		out.Set(clone(v.Elem(), seen))
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		out.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				out.Field(i).Set(clone(v.Field(i), seen))
			}
		}
		return out
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(clone(v.Index(i), seen))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		it := v.MapRange()
		for it.Next() {
			out.SetMapIndex(it.Key(), clone(it.Value(), seen))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(clone(v.Index(i), seen))
		}
		return out
	default:
		return v
	}
}
