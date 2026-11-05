package config

import (
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
