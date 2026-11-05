package middleware

import (
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecurity(t *testing.T) {
	if _, e := CORS(CORSConfig{Origins: []string{"*"}, Credentials: true}); e == nil {
		t.Fatal("unsafe CORS")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "192.0.2.1:80"
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	if !ClientIP(r, nil).Equal(net.ParseIP("192.0.2.1")) {
		t.Fatal("untrusted proxy")
	}
	w := httptest.NewRecorder()
	Recovery(func(_ctx context.Context, _err error) {})(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("bad") })).ServeHTTP(w, r)
	if w.Code != 500 {
		t.Fatal(w.Code)
	}
}

func TestCompression(t *testing.T) {
	handler := Compression(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hello")) }))
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	gz, e := gzip.NewReader(w.Body)
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(gz)
	gz.Close()
	if e != nil || string(b) != "hello" {
		t.Fatal(string(b), e)
	}
	r.Header.Set("Accept-Encoding", "gzip;q=0")
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Header().Get("Content-Encoding") != "" {
		t.Fatal("q=0 ignored")
	}
}
