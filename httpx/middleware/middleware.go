// Package middleware supplies composable native HTTP middleware.
package middleware

import (
	"context"
	"fmt"
	"github.com/cetrictech/go-platform-kit/id"
	"github.com/cetrictech/go-platform-kit/logger"
	"github.com/cetrictech/go-platform-kit/response"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strings"
	"time"
)

// Middleware returns context-aware HTTP admission or coordination middleware.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware in left-to-right request order.
func Chain(h http.Handler, m ...Middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

// RequestID generates a server-owned request ID and adds it to logging context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, e := id.NewV7()
		if e != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		s := u.String()
		w.Header().Set("X-Request-ID", s)
		ctx := logger.Context(r.Context(), slog.String("request_id", s))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logging returns structured request-logging instrumentation.
func Logging(l *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &response.Writer{ResponseWriter: w}
			next.ServeHTTP(rw, r)
			status := rw.Status
			if status == 0 {
				status = 200
			}
			l.InfoContext(r.Context(), "HTTP request", "method", r.Method, "path", r.URL.Path, "status", status, "duration", time.Since(start))
		})
	}
}

// Recovery returns a reporting panic-recovery interceptor or middleware.
func Recovery(report func(context.Context, error)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			rw, ok := w.(*response.Writer)
			if !ok {
				rw = &response.Writer{ResponseWriter: w}
			}
			defer func() {
				if p := recover(); p != nil {
					e := fmt.Errorf("handler panic: %v", p)
					if report != nil {
						report(r.Context(), e)
					} else {
						slog.ErrorContext(r.Context(), "handler panic", "error", e)
					}
					if !rw.Written {
						http.Error(rw, "Internal server error", http.StatusInternalServerError)
					}
				}
			}()
			next.ServeHTTP(rw, r)
		})
	}
}

// BodyLimit limits request-body reading and rejects oversized declared bodies.
func BodyLimit(n int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if n <= 0 {
				http.Error(w, "Invalid body limit", http.StatusInternalServerError)
				return
			}
			if r.ContentLength > n {
				http.Error(w, "Request body too large", http.StatusRequestEntityTooLarge)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, n)
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout uses the standard buffered HTTP timeout handler; it is unsuitable for streaming.
func Timeout(d time.Duration) Middleware {
	return func(next http.Handler) http.Handler { return http.TimeoutHandler(next, d, "Request timed out") }
}

// SecurityHeaders sets conservative browser response security headers.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// ContentType enforces a parsed request MIME type when a body is present.
func ContentType(want string) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.ContentLength != 0 {
				actual, _, e := mime.ParseMediaType(r.Header.Get("Content-Type"))
				if e != nil || actual != want {
					http.Error(w, "Unsupported media type", http.StatusUnsupportedMediaType)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Authorize applies an explicit caller-supplied authorization policy.
func Authorize(check func(*http.Request) error) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if check == nil || check(r) != nil {
				http.Error(w, "Forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// CORSConfig contains explicitly allowed origins, methods, headers and credential policy.
type CORSConfig struct {
	Origins, Methods, Headers []string
	Credentials               bool
	MaxAge                    time.Duration
}

// CORS validates CORS policy and returns its native middleware.
func CORS(c CORSConfig) (Middleware, error) {
	origins := map[string]bool{}
	for _, o := range c.Origins {
		if o == "*" && c.Credentials {
			return nil, fmt.Errorf("CORS: wildcard origin with credentials")
		}
		origins[o] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			w.Header().Add("Vary", "Origin")
			if origin != "" && (origins[origin] || origins["*"]) {
				value := origin
				if origins["*"] {
					value = "*"
				}
				w.Header().Set("Access-Control-Allow-Origin", value)
				if c.Credentials {
					w.Header().Set("Access-Control-Allow-Credentials", "true")
				}
				if r.Method == "OPTIONS" && r.Header.Get("Access-Control-Request-Method") != "" {
					w.Header().Add("Vary", "Access-Control-Request-Method")
					w.Header().Add("Vary", "Access-Control-Request-Headers")
					w.Header().Set("Access-Control-Allow-Methods", strings.Join(c.Methods, ", "))
					w.Header().Set("Access-Control-Allow-Headers", strings.Join(c.Headers, ", "))
					w.Header().Set("Access-Control-Max-Age", fmt.Sprint(int(c.MaxAge.Seconds())))
					w.WriteHeader(204)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

// ClientIP trusts X-Forwarded-For only for explicitly trusted proxy CIDRs, peeling from the right.
func ClientIP(r *http.Request, trusted []*net.IPNet) net.IP {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	isTrusted := func(ip net.IP) bool {
		for _, n := range trusted {
			if n.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !isTrusted(ip) {
		return ip
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate := net.ParseIP(strings.TrimSpace(parts[i]))
		if candidate == nil {
			return ip
		}
		ip = candidate
		if !isTrusted(ip) {
			break
		}
	}
	return ip
}
