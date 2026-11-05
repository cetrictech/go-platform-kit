package jwt

import (
	"context"
	"net/http"
	"strings"
)

type claimsKey struct{}

// Authenticate verifies a standard Bearer authorization value and returns a context carrying private verified claims.
func (v *Verifier) Authenticate(ctx context.Context, authorization string) (context.Context, error) {
	parts := strings.Fields(authorization)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ctx, ErrInvalidToken
	}
	claims, e := v.Verify(ctx, parts[1])
	if e != nil {
		return ctx, e
	}
	return context.WithValue(ctx, claimsKey{}, copyClaims(claims)), nil
}

// FromContext returns a private copy, preventing one consumer from mutating another consumer's identity claims.
func FromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey{}).(Claims)
	if !ok {
		return nil, false
	}
	return copyClaims(claims), true
}
func copyClaims(c Claims) Claims {
	out := make(Claims, len(c))
	for k, v := range c {
		out[k] = copyValue(v)
	}
	return out
}
func copyValue(v any) any {
	switch x := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(x))
		for k, v := range x {
			m[k] = copyValue(v)
		}
		return m
	case []any:
		a := make([]any, len(x))
		for i := range x {
			a[i] = copyValue(x[i])
		}
		return a
	default:
		return v
	}
}

// Middleware rejects invalid/missing bearer credentials with a generic 401 and never echoes tokens or claim diagnostics.
func (v *Verifier) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		values := r.Header.Values("Authorization")
		if len(values) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		ctx, e := v.Authenticate(r.Context(), values[0])
		if e != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
