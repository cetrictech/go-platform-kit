package validation

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStruct(t *testing.T) {
	type Child struct {
		Email string `validate:"email"`
	}
	c := struct {
		Name     string `validate:"required,min=2"`
		Children []Child
	}{"ab", []Child{{"a@example.org"}}}
	if e := Struct(c); e != nil {
		t.Fatal(e)
	}
	c.Children[0].Email = "bad"
	if Struct(c) == nil {
		t.Fatal("invalid nested email")
	}
}

func TestNil(t *testing.T) {
	if Struct(nil) == nil {
		t.Fatal("nil accepted")
	}
	var p *struct{}
	if Struct(p) == nil {
		t.Fatal("nil pointer accepted")
	}
}

func TestHTTPDecode(t *testing.T) {
	var dst struct {
		Name string `json:"name" validate:"required"`
	}
	for _, body := range []string{`{"extra":true}`, `{} {}`, `{bad`} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if DecodeJSON(httptest.NewRecorder(), r, &dst, 100) == nil {
			t.Fatal(body)
		}
	}
	r := httptest.NewRequest("POST", "/", strings.NewReader(`{"name":"ok"}`))
	if e := DecodeJSON(httptest.NewRecorder(), r, &dst, 100); e != nil || dst.Name != "ok" {
		t.Fatal(dst, e)
	}
}
