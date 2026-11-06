// Package validation validates struct tags without framework dependencies.
package validation

import (
	"fmt"
	"github.com/google/uuid"
	"net/mail"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// FieldError describes a safe field-level validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// Errors collects field-level validation failures.
type Errors []FieldError

// Error returns a diagnostic error description.
func (e Errors) Error() string { return fmt.Sprintf("validation: %d invalid fields", len(e)) }

// Validator applies built-in rules and optional application validators.
type Validator struct{ Rules map[string]func(any) bool }

// Struct validates nested struct fields and collects safe field-level errors.
func Struct(v any) error { return (Validator{}).Struct(v) }

// Struct validates nested struct fields and collects safe field-level errors.
func (v Validator) Struct(x any) error {
	var errs Errors
	value := reflect.ValueOf(x)
	if !value.IsValid() || value.Kind() == reflect.Pointer && value.IsNil() {
		return Errors{{Field: "", Rule: "required", Message: "A value is required."}}
	}
	v.walk(reflect.ValueOf(x), "", &errs)
	if len(errs) > 0 {
		return errs
	}
	return nil
}
func (v Validator) walk(x reflect.Value, path string, out *Errors) {
	if !x.IsValid() {
		*out = append(*out, FieldError{path, "required", "A value is required."})
		return
	}
	for x.Kind() == reflect.Pointer || x.Kind() == reflect.Interface {
		if x.IsNil() {
			return
		}
		x = x.Elem()
	}
	if x.Kind() == reflect.Slice || x.Kind() == reflect.Array {
		for i := 0; i < x.Len(); i++ {
			v.walk(x.Index(i), fmt.Sprintf("%s[%d]", path, i), out)
		}
		return
	}
	if x.Kind() != reflect.Struct {
		return
	}
	for i := 0; i < x.NumField(); i++ {
		f := x.Type().Field(i)
		if f.PkgPath != "" {
			continue
		}
		tag := f.Tag.Get("validate")
		if tag == "-" {
			continue
		}
		p := f.Name
		if path != "" {
			p = path + "." + p
		}
		a := x.Field(i)
		skip := false
		for _, rule := range strings.Split(tag, ",") {
			if rule == "omitempty" && a.IsZero() {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		for _, rule := range strings.Split(tag, ",") {
			if rule == "" || rule == "omitempty" {
				continue
			}
			parts := strings.SplitN(rule, "=", 2)
			name := parts[0]
			arg := ""
			if len(parts) == 2 {
				arg = parts[1]
			}
			if !v.check(a, name, arg) {
				*out = append(*out, FieldError{p, name, "Field does not satisfy " + rule + "."})
			}
		}
		v.walk(a, p, out)
	}
}
func (v Validator) check(x reflect.Value, rule, arg string) bool {
	if rule == "required" {
		return !x.IsZero()
	}
	for x.Kind() == reflect.Pointer {
		if x.IsNil() {
			return false
		}
		x = x.Elem()
	}
	s := ""
	if x.Kind() == reflect.String {
		s = x.String()
	}
	switch rule {
	case "email":
		a, e := mail.ParseAddress(s)
		return e == nil && a.Address == s
	case "url":
		u, e := url.ParseRequestURI(s)
		return e == nil && u.Scheme != "" && u.Host != ""
	case "uuid":
		return uuid.Validate(s) == nil
	case "oneof":
		for _, a := range strings.Fields(arg) {
			if fmt.Sprint(x.Interface()) == a {
				return true
			}
		}
		return false
	case "min", "max", "len":
		n, e := strconv.ParseFloat(arg, 64)
		if e != nil {
			return false
		}
		var val float64
		switch x.Kind() {
		case reflect.String:
			val = float64(utf8.RuneCountInString(s))
		case reflect.Slice, reflect.Array, reflect.Map:
			val = float64(x.Len())
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			val = float64(x.Int())
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			val = float64(x.Uint())
		case reflect.Float32, reflect.Float64:
			val = x.Float()
		default:
			return false
		}
		if rule == "min" {
			return val >= n
		}
		if rule == "max" {
			return val <= n
		}
		return val == n
	default:
		f := v.Rules[rule]
		return f != nil && f(x.Interface())
	}
}
