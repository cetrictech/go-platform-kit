package client

import (
	"bytes"
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/retry"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSafeRetryAndStreamingLifetime(t *testing.T) {
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Request-ID") == "" || r.Header.Get("X-Default") != "yes" {
			t.Error("headers missing")
		}
		if n.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(503)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer s.Close()
	c, e := New(Config{Headers: http.Header{"X-Default": []string{"yes"}}, RetryPolicy: &retry.Policy{MaxAttempts: 2, Idempotent: true, MaxElapsed: time.Second}})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	r, _ := http.NewRequest("GET", s.URL, nil)
	res, e := c.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	b, e := io.ReadAll(res.Body)
	res.Body.Close()
	if e != nil || string(b) != "ok" || n.Load() != 2 {
		t.Fatal(string(b), e, n.Load())
	}
	n.Store(0)
	r, _ = http.NewRequest("POST", s.URL, strings.NewReader("input"))
	res, e = c.Do(r)
	if e != nil || res.StatusCode != 503 || n.Load() != 1 {
		t.Fatal(res, e, n.Load())
	}
	res.Body.Close()
}
func TestStreamLimitAndOwnedClient(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		_, _ = w.Write([]byte("123456"))
	}))
	defer s.Close()
	native := s.Client()
	c, e := New(Config{HTTPClient: native, MaxResponseBytes: 3})
	if e != nil || c.HTTP != native {
		t.Fatal(e)
	}
	r, _ := http.NewRequest("GET", s.URL, nil)
	res, e := c.Do(r)
	if e != nil {
		t.Fatal(e)
	}
	_, e = io.ReadAll(res.Body)
	if !errors.Is(e, ErrResponseTooLarge) {
		t.Fatal(e)
	}
	var b [1]byte
	if _, e = res.Body.Read(b[:]); !errors.Is(e, ErrResponseTooLarge) {
		t.Fatal(e)
	}
	res.Body.Close()
}
func TestMultipartAndAtomicDownload(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "input")
	_ = os.WriteFile(source, []byte("file-data"), 0600)
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			if e := r.ParseMultipartForm(1024); e != nil {
				t.Error(e)
				return
			}
			defer r.MultipartForm.RemoveAll()
			f, _, e := r.FormFile("upload")
			if e != nil {
				t.Error(e)
				return
			}
			b, _ := io.ReadAll(f)
			f.Close()
			if string(b) != "file-data" || r.FormValue("field") != "value" {
				t.Error("multipart mismatch")
			}
			_, _ = w.Write([]byte(`{"ok":true}`))
			return
		}
		_, _ = w.Write([]byte("too-long"))
	}))
	defer s.Close()
	c, _ := New(Config{MaxResponseBytes: 100})
	defer c.Close()
	var output map[string]bool
	if e := c.Multipart(context.Background(), s.URL, map[string]string{"field": "value"}, []MultipartFile{{Field: "upload", Path: source}}, &output); e != nil || !output["ok"] {
		t.Fatal(output, e)
	}
	destination := filepath.Join(dir, "download")
	_ = os.WriteFile(destination, []byte("original"), 0600)
	small, _ := New(Config{MaxResponseBytes: 2})
	defer small.Close()
	if e := small.DownloadFile(context.Background(), s.URL, destination); !errors.Is(e, ErrResponseTooLarge) {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(destination)
	if !bytes.Equal(b, []byte("original")) {
		t.Fatal("failed download replaced destination")
	}
}
