package config

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestLoadAtomic(t *testing.T) {
	type cfg struct {
		Port   int    `default:"80"`
		Secret string `secret:"true"`
		Nested struct {
			Timeout time.Duration `default:"2s"`
		}
	}
	c := cfg{Secret: "hidden"}
	if e := Load(&c, Options{Prefix: "APP_", Lookup: func(k string) (string, bool) {
		if k == "APP_PORT" {
			return "bad", true
		}
		return "", false
	}}); e == nil || c.Port != 0 {
		t.Fatal(e, c)
	}
	if e := Load(&c, Options{Lookup: func(string) (string, bool) { return "", false }}); e != nil || c.Port != 80 || c.Nested.Timeout != 2*time.Second {
		t.Fatal(e, c)
	}
	b, e := Dump(c)
	if e != nil || strings.Contains(string(b), "hidden") {
		t.Fatal(string(b), e)
	}
}
func TestRequired(t *testing.T) {
	var c struct {
		Token string `required:"true"`
	}
	if Load(&c, Options{Lookup: func(string) (string, bool) { return "", false }}) == nil {
		t.Fatal("missing required accepted")
	}
}

func TestProgrammaticSecretAndPointers(t *testing.T) {
	type nested struct {
		Port int `default:"80"`
	}
	c := struct {
		Secret string `json:"-"`
		Nested *nested
	}{Secret: "preserved"}
	if e := Load(&c, Options{Lookup: func(string) (string, bool) { return "", false }}); e != nil || c.Secret != "preserved" || c.Nested == nil || c.Nested.Port != 80 {
		t.Fatal(c, e)
	}
}

func TestDumpMapSecrets(t *testing.T) {
	b, e := Dump(map[string]any{"API_KEY": "hidden", "nested": map[string]any{"authorization": "hidden"}})
	if e != nil || strings.Contains(string(b), "hidden") {
		t.Fatal(string(b), e)
	}
}

func TestFileFalseAndZeroOverrideDefaults(t *testing.T) {
	var c struct {
		Enabled bool `default:"true"`
		Count   int  `default:"12"`
	}
	if e := Load(&c, Options{File: strings.NewReader(`{"Enabled":false,"Count":0}`), Lookup: func(string) (string, bool) { return "", false }}); e != nil || c.Enabled || c.Count != 0 {
		t.Fatal(c, e)
	}
}
func TestProfilesProgrammaticAndStandaloneValidation(t *testing.T) {
	var c struct {
		Port   int    `default:"80"`
		Secret string `required:"true"`
	}
	e := Load(&c, Options{Prefix: "APP_", Environment: "prod", File: strings.NewReader(`{"Port":81,"Secret":"file"}`), Overrides: map[string]io.Reader{"prod": strings.NewReader(`{"Port":82}`)}, Lookup: func(key string) (string, bool) {
		if key == "APP_PORT" {
			return "83", true
		}
		return "", false
	}, Values: map[string]string{"APP_PORT": "84"}})
	if e != nil || c.Port != 84 || c.Secret != "file" {
		t.Fatal(c, e)
	}
	c.Secret = ""
	if Validate(c) == nil {
		t.Fatal("standalone required validation skipped")
	}
}
func TestConfigCycleAndSizeFailAtomically(t *testing.T) {
	c := struct{ Data map[string]any }{Data: map[string]any{}}
	c.Data["self"] = c.Data
	if e := Load(&c); e == nil {
		t.Fatal("cyclic map accepted")
	}
	var dst struct{ Value string }
	e := Load(&dst, Options{File: strings.NewReader(`{"Value":"` + strings.Repeat("x", 100) + `"}`), MaxFileBytes: 32})
	if e == nil || dst.Value != "" {
		t.Fatal(dst, e)
	}
}
