package middleware

import (
	"context"
	"github.com/cetrictech/go-platform-kit/id"
	"github.com/cetrictech/go-platform-kit/logger"
	"log/slog"
	"net/http"
	"strings"
)

// Authenticate attaches a verified identity context before authorization. Failure responses never expose verifier diagnostics.
func Authenticate(check func(*http.Request) (context.Context, error)) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if check == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			ctx, e := check(r)
			if e != nil || ctx == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CorrelationID generates a server-owned correlation ID unless a bounded forwarded ID is explicitly trusted.
func CorrelationID(trustForwarded bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			correlation := w.Header().Get("X-Request-ID")
			if trustForwarded {
				v := r.Header.Values("X-Correlation-ID")
				if len(v) == 1 && validID(v[0]) {
					correlation = v[0]
				}
			}
			if correlation == "" {
				u, e := id.NewV7()
				if e != nil {
					http.Error(w, "Internal server error", http.StatusInternalServerError)
					return
				}
				correlation = u.String()
			}
			w.Header().Set("X-Correlation-ID", correlation)
			next.ServeHTTP(w, r.WithContext(logger.Context(r.Context(), slog.String("correlation_id", correlation))))
		})
	}
}
func validID(s string) bool {
	return len(s) > 0 && len(s) <= 128 && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	}) < 0
}
