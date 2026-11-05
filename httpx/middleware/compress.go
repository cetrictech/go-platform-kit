package middleware

import (
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
)

// Compression enables gzip when accepted, excluding protocol upgrades and bodyless responses.
func Compression(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Accept-Encoding")
		accept := false
		for _, item := range strings.Split(r.Header.Get("Accept-Encoding"), ",") {
			parts := strings.Split(item, ";")
			if strings.TrimSpace(parts[0]) != "gzip" {
				continue
			}
			quality := 1.0
			for _, p := range parts[1:] {
				pair := strings.SplitN(strings.TrimSpace(p), "=", 2)
				if len(pair) == 2 && pair[0] == "q" {
					v, e := strconv.ParseFloat(pair[1], 64)
					if e != nil || v < 0 || v > 1 {
						quality = 0
					} else {
						quality = v
					}
				}
			}
			accept = quality > 0
		}
		if !accept || r.Header.Get("Upgrade") != "" {
			next.ServeHTTP(w, r)
			return
		}
		gz := gzip.NewWriter(w)
		rw := &gzipWriter{ResponseWriter: w, gz: gz}
		defer func() {
			if rw.encoded {
				_ = gz.Close()
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz               *gzip.Writer
	written, encoded bool
}

// WriteHeader records and writes the HTTP status once.
func (w *gzipWriter) WriteHeader(status int) {
	if w.written {
		return
	}
	if status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	w.written = true
	if status != http.StatusNoContent && status != http.StatusNotModified && w.Header().Get("Content-Encoding") == "" {
		w.encoded = true
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length")
	}
	w.ResponseWriter.WriteHeader(status)
}

// Write implements the wrapped HTTP writer with commitment tracking.
func (w *gzipWriter) Write(b []byte) (int, error) {
	if !w.written {
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", http.DetectContentType(b))
		}
		w.WriteHeader(http.StatusOK)
	}
	if w.encoded {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

// Flush waits for pending publication outcomes, bounded by context cancellation.
func (w *gzipWriter) Flush() {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	if w.encoded {
		_ = w.gz.Flush()
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap returns the underlying wrapped resource or error cause.
func (w *gzipWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
