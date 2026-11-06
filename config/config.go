// Package config loads explicit, typed application configuration.
package config

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cetrictech/go-platform-kit/id"
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
	Environment  string
	Overrides    map[string]io.Reader
	Values       map[string]string
	MaxFileBytes int64
	Prefix       string
	Lookup       func(string) (string, bool)
	File         io.Reader
	Validate     func(any) error
}

// Load updates a struct only after all conversions and validation succeed. Tags are env, default, required, secret.
func Load(dst any, options ...Options) error {
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer || v.IsNil() || v.Elem().Kind() != reflect.Struct {
		return errors.New("config: destination must be a non-nil struct pointer")
	}
	if len(options) > 1 {
		return errors.New("config: at most one options value allowed")
	}
	o := Options{}
	if len(options) == 1 {
		o = options[0]
	}
	if o.MaxFileBytes < 0 || o.MaxFileBytes > 16<<20 {
		return errors.New("config: invalid file size bound")
	}
	if o.MaxFileBytes == 0 {
		o.MaxFileBytes = 1 << 20
	}
	if o.Lookup == nil {
		o.Lookup = os.LookupEnv
	}
	var override io.Reader
	if o.Environment != "" {
		var ok bool
		override, ok = o.Overrides[o.Environment]
		if !ok || override == nil {
			return errors.New("config: environment override not configured")
		}
	}
	budget := 10000
	if e := checkGraph(v.Elem(), 0, &budget); e != nil {
		return e
	}
	tmp := reflect.New(v.Elem().Type())
	tmp.Elem().Set(clone(v.Elem(), map[visit]reflect.Value{}))
	if _, e := loadFields(tmp.Elem(), o.Prefix, nil, true, 0); e != nil {
		return e
	}
	for _, reader := range []io.Reader{o.File, override} {
		if reader == nil {
			continue
		}
		limited := &io.LimitedReader{R: reader, N: o.MaxFileBytes + 1}
		d := json.NewDecoder(limited)
		d.DisallowUnknownFields()
		if e := d.Decode(tmp.Interface()); e != nil {
			return fmt.Errorf("config: file: %w", e)
		}
		var extra any
		e := d.Decode(&extra)
		if limited.N == 0 {
			return errors.New("config: file size limit exceeded")
		}
		if e != io.EOF {
			return errors.New("config: trailing JSON data")
		}
	}
	lookup := func(key string) (string, bool) {
		if value, ok := o.Values[key]; ok {
			return value, true
		}
		return o.Lookup(key)
	}
	if _, e := loadFields(tmp.Elem(), o.Prefix, lookup, false, 0); e != nil {
		return e
	}
	if e := Validate(tmp.Interface()); e != nil {
		return e
	}
	if o.Validate != nil {
		if e := o.Validate(tmp.Interface()); e != nil {
			return e
		}
	}
	v.Elem().Set(tmp.Elem())
	return nil
}
func groupType(t reflect.Type) bool {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t.Kind() == reflect.Struct && t != reflect.TypeFor[time.Time]() && t != reflect.TypeFor[url.URL]() && !reflect.PointerTo(t).Implements(reflect.TypeFor[encoding.TextUnmarshaler]())
}
func loadFields(v reflect.Value, prefix string, lookup func(string) (string, bool), defaults bool, depth int) (bool, error) {
	if depth > 32 {
		return false, errors.New("config: nesting exceeds 32 levels")
	}
	touched := false
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
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
		if groupType(x.Type()) {
			group := x
			fresh := false
			if group.Kind() == reflect.Pointer {
				fresh = group.IsNil()
				if fresh {
					group = reflect.New(group.Type().Elem())
				}
				group = group.Elem()
			}
			changed, e := loadFields(group, key+"_", lookup, defaults, depth+1)
			if e != nil {
				return false, e
			}
			if fresh && changed {
				x.Set(group.Addr())
			}
			touched = touched || changed
			continue
		}
		var raw string
		var present bool
		if defaults {
			if x.IsZero() {
				raw, present = f.Tag.Lookup("default")
			}
		} else {
			raw, present = lookup(key)
		}
		if present {
			if e := convert(x, raw); e != nil {
				return false, fmt.Errorf("config: %s: invalid %s", key, x.Type())
			}
			touched = true
		}
	}
	return touched, nil
}
func checkGraph(v reflect.Value, depth int, budget *int) error {
	if !v.IsValid() {
		return nil
	}
	if depth > 32 || *budget <= 0 {
		return errors.New("config: cyclic or excessively large/nested configuration")
	}
	*budget--
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			return checkGraph(v.Elem(), depth+1, budget)
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				if e := checkGraph(v.Field(i), depth+1, budget); e != nil {
					return e
				}
			}
		}
	case reflect.Map:
		it := v.MapRange()
		for it.Next() {
			if e := checkGraph(it.Value(), depth+1, budget); e != nil {
				return e
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if e := checkGraph(v.Index(i), depth+1, budget); e != nil {
				return e
			}
		}
	}
	return nil
}
func convert(v reflect.Value, s string) error {
	if v.Kind() == reflect.Pointer {
		v.Set(reflect.New(v.Type().Elem()))
		return convert(v.Elem(), s)
	}
	if v.Type() == reflect.TypeFor[time.Time]() {
		t, e := parseTime(s)
		if e != nil {
			return e
		}
		v.Set(reflect.ValueOf(t))
		return nil
	}
	if v.Type() == reflect.TypeFor[id.UUID]() {
		u, e := id.Parse(strings.TrimSpace(s))
		if e != nil {
			return e
		}
		v.Set(reflect.ValueOf(u))
		return nil
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

// Validate checks required tags recursively and runs nested/root validation hooks after field checks.
func Validate(value any) error {
	v := reflect.ValueOf(value)
	if !v.IsValid() || v.Kind() == reflect.Pointer && v.IsNil() {
		return errors.New("config: non-nil struct required")
	}
	base := v
	for base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	if base.Kind() != reflect.Struct {
		return errors.New("config: struct required")
	}
	budget := 10000
	if e := checkGraph(v, 0, &budget); e != nil {
		return e
	}
	return validateValue(v, "")
}
func validateValue(v reflect.Value, path string) error {
	original := v
	for v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	for i := 0; i < v.NumField(); i++ {
		f := v.Type().Field(i)
		if f.PkgPath != "" {
			continue
		}
		x := v.Field(i)
		name := f.Name
		if path != "" {
			name = path + "." + name
		}
		if f.Tag.Get("required") == "true" && x.IsZero() {
			return fmt.Errorf("config: %s is required", name)
		}
		if groupType(x.Type()) {
			if e := validateValue(x, name); e != nil {
				return e
			}
		}
	}
	if original.CanInterface() {
		if hook, ok := original.Interface().(interface{ Validate() error }); ok {
			return hook.Validate()
		}
	}
	if v.CanAddr() && v.Addr().CanInterface() {
		if hook, ok := v.Addr().Interface().(interface{ Validate() error }); ok {
			return hook.Validate()
		}
	}
	return nil
}

// Dump returns JSON with secret-tagged and sensitive-name fields redacted, recursively.
func Dump(v any) ([]byte, error) { return json.Marshal(redaction.Redact(v)) }

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
	return time.Time{}, errors.New("invalid time")
}

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
