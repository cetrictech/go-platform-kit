package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSON(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{"ok":true}`)) }))
	defer s.Close()
	c, e := New(Config{})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	var v map[string]bool
	if e = c.JSON(context.Background(), "GET", s.URL, nil, &v); e != nil || !v["ok"] {
		t.Fatal(v, e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if c.JSON(ctx, "GET", s.URL, nil, &v) == nil {
		t.Fatal("cancel")
	}
}
