package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	j "github.com/golang-jwt/jwt/v5"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func sign(t *testing.T, key *rsa.PrivateKey, kid string, now time.Time, mutate func(j.MapClaims)) string {
	t.Helper()
	claims := j.MapClaims{"iss": "https://issuer.example", "aud": []string{"sdk"}, "sub": "subject", "exp": now.Add(time.Hour).Unix(), "nbf": now.Add(-time.Second).Unix(), "iat": now.Unix(), "roles": []string{"reader"}}
	if mutate != nil {
		mutate(claims)
	}
	token := j.NewWithClaims(j.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	s, e := token.SignedString(key)
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSignatureClaimsAndHTTP(t *testing.T) {
	key, e := rsa.GenerateKey(rand.Reader, 2048)
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now().UTC().Truncate(time.Second)
	v, e := New(context.Background(), Config{Issuer: "https://issuer.example", Audience: []string{"sdk"}, Algorithms: []string{"RS256"}, Keys: map[string]any{"a": &key.PublicKey}, Now: func() time.Time { return now }})
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	token := sign(t, key, "a", now, nil)
	claims, e := v.Verify(context.Background(), token)
	if e != nil || claims["sub"] != "subject" {
		t.Fatal(claims, e)
	}
	for name, change := range map[string]func(j.MapClaims){"expired": func(c j.MapClaims) { c["exp"] = now.Unix() }, "missing expiration": func(c j.MapClaims) { delete(c, "exp") }, "issuer": func(c j.MapClaims) { c["iss"] = "other" }, "audience": func(c j.MapClaims) { c["aud"] = "other" }, "future": func(c j.MapClaims) { c["nbf"] = now.Add(time.Minute).Unix() }, "issued future": func(c j.MapClaims) { c["iat"] = now.Add(time.Minute).Unix() }} {
		t.Run(name, func(t *testing.T) {
			if _, e = v.Verify(context.Background(), sign(t, key, "a", now, change)); !errors.Is(e, ErrInvalidToken) {
				t.Fatal(e)
			}
		})
	}
	if _, e = v.Verify(context.Background(), token[:len(token)-3]+"bad"); e == nil {
		t.Fatal("tamper accepted")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	v.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		first, ok := FromContext(r.Context())
		if !ok {
			t.Fatal("claims missing")
		}
		first["roles"].([]any)[0] = "admin"
		again, _ := FromContext(r.Context())
		if again["roles"].([]any)[0] != "reader" {
			t.Fatal("claims alias")
		}
		w.WriteHeader(204)
	})).ServeHTTP(w, r)
	if w.Code != 204 {
		t.Fatal(w.Code)
	}
	r.Header.Set("Authorization", "Bearer secret-invalid-token")
	w = httptest.NewRecorder()
	v.Middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unauthorized handler called") })).ServeHTTP(w, r)
	if w.Code != 401 || strings.Contains(w.Body.String(), "secret-invalid-token") {
		t.Fatal(w.Body.String())
	}
}
func TestJWKSRotationThrottleAndFailClosed(t *testing.T) {
	first, _ := rsa.GenerateKey(rand.Reader, 2048)
	second, _ := rsa.GenerateKey(rand.Reader, 2048)
	var now atomic.Int64
	now.Store(time.Now().Unix())
	var calls atomic.Int32
	var mu sync.RWMutex
	key := first
	id := "first"
	unavailable := false
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		mu.RLock()
		defer mu.RUnlock()
		if unavailable {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": id, "use": "sig", "alg": "RS256", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}})
	}))
	defer s.Close()
	clock := func() time.Time { return time.Unix(now.Load(), 0) }
	v, e := New(context.Background(), Config{Issuer: "https://issuer.example", Audience: []string{"sdk"}, Algorithms: []string{"RS256"}, JWKSURL: s.URL, HTTPClient: s.Client(), Now: clock, CacheTTL: time.Minute, MinRefreshInterval: time.Second})
	if e != nil {
		t.Fatal(e)
	}
	defer v.Close()
	if _, e = v.Verify(context.Background(), sign(t, first, "first", clock(), nil)); e != nil {
		t.Fatal(e)
	}
	unknown := sign(t, second, "unknown", clock(), nil)
	for i := 0; i < 10; i++ {
		if _, e = v.Verify(context.Background(), unknown); e == nil {
			t.Fatal("unknown kid")
		}
	}
	if calls.Load() != 1 {
		t.Fatal("unknown key refresh storm", calls.Load())
	}
	mu.Lock()
	key = second
	id = "second"
	mu.Unlock()
	now.Add(2)
	if _, e = v.Verify(context.Background(), sign(t, second, "second", clock(), nil)); e != nil {
		t.Fatal(e)
	}
	mu.Lock()
	unavailable = true
	mu.Unlock()
	now.Add(61)
	if _, e = v.Verify(context.Background(), sign(t, second, "second", clock(), nil)); e == nil {
		t.Fatal("expired cache accepted during outage")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = v.Verify(ctx, unknown); !errors.Is(e, context.Canceled) {
		t.Fatal(e)
	}
}
func TestPolicyAndBounds(t *testing.T) {
	for _, c := range []Config{{}, {Issuer: "issuer", Audience: []string{"a"}, Algorithms: []string{"none"}, Keys: map[string]any{"a": []byte("short")}}} {
		if _, e := New(context.Background(), c); e == nil {
			t.Fatal("invalid policy")
		}
	}
	if _, e := parseJWK(jwk{Kty: "oct"}); e == nil {
		t.Fatal("remote HMAC accepted")
	}
}
