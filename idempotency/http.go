package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// HTTPConfig requires a server-defined authorization/tenant scope. Bodies are fingerprinted, never persisted.
// Completed duplicates receive 409 and Idempotency-Status: completed; response bodies are not replayed.
// RequestTimeout must be shorter than Lease. A handler must honor cancellation; use durable business keys for irreversible side effects.
type HTTPConfig struct {
	Store                            Store
	Scope                            func(*http.Request) string
	Lease, Retention, RequestTimeout time.Duration
	MaxBodyBytes                     int64
	OnError                          func(context.Context, error)
}

// Middleware returns context-aware HTTP admission or coordination middleware.
func Middleware(c HTTPConfig) (func(http.Handler) http.Handler, error) {
	if c.Store == nil || c.Scope == nil || c.Lease <= 0 || c.Retention <= 0 || c.RequestTimeout <= 0 || c.RequestTimeout >= c.Lease || c.MaxBodyBytes < 1 {
		return nil, fmtError()
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			k := r.Header.Get("Idempotency-Key")
			scope := c.Scope(r)
			if len(k) < 1 || len(k) > 256 || scope == "" {
				http.Error(w, "Valid idempotency key and scope required", http.StatusBadRequest)
				return
			}
			var body []byte
			var e error
			if r.Body != nil {
				body, e = io.ReadAll(io.LimitReader(r.Body, c.MaxBodyBytes+1))
				_ = r.Body.Close()
			}
			if e != nil || int64(len(body)) > c.MaxBodyBytes {
				http.Error(w, "Invalid request body", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			h := sha256.New()
			_, _ = h.Write([]byte(r.Method + "\x00" + r.URL.RequestURI() + "\x00" + r.Header.Get("Content-Type") + "\x00"))
			_, _ = h.Write(body)
			fingerprint := hex.EncodeToString(h.Sum(nil))
			keyHash := sha256.Sum256([]byte(scope + "\x00" + k))
			key := hex.EncodeToString(keyHash[:])
			record, e := c.Store.Begin(r.Context(), key, fingerprint, c.Lease)
			if e != nil {
				if e == ErrBusy || e == ErrMismatch || e == ErrCompleted {
					w.Header().Set("Idempotency-Status", string(record.State))
					http.Error(w, "Duplicate request or payload mismatch", http.StatusConflict)
				} else {
					http.Error(w, "Idempotency unavailable", http.StatusServiceUnavailable)
				}
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), c.RequestTimeout)
			defer cancel()
			rw := &statusWriter{ResponseWriter: w, status: 200}
			defer func() {
				cleanup, done := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
				defer done()
				if p := recover(); p != nil {
					if e := c.Store.Fail(cleanup, key, record.Token, c.Retention); e != nil {
						report(c, cleanup, e)
					}
					panic(p)
				}
				var e error
				if rw.status >= 500 {
					e = c.Store.Fail(cleanup, key, record.Token, c.Retention)
				} else {
					e = c.Store.Complete(cleanup, key, record.Token, rw.status, c.Retention)
				}
				if e != nil {
					report(c, cleanup, e)
				}
			}()
			next.ServeHTTP(rw, r.WithContext(ctx))
		})
	}, nil
}
func fmtError() error { return errors.New("idempotency: invalid HTTP configuration") }

type statusWriter struct {
	http.ResponseWriter
	status  int
	written bool
}

// WriteHeader records and writes the HTTP status once.
func (w *statusWriter) WriteHeader(s int) {
	if !w.written {
		w.status = s
		w.written = true
		w.ResponseWriter.WriteHeader(s)
	}
}

// Write implements the wrapped HTTP writer with commitment tracking.
func (w *statusWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap returns the underlying wrapped resource or error cause.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func report(c HTTPConfig, ctx context.Context, e error) {
	if c.OnError != nil {
		c.OnError(ctx, e)
	} else {
		slog.ErrorContext(ctx, "idempotency state transition failed", "error", e)
	}
}
