// Package jwt verifies JWTs with explicit issuer/audience/algorithm policy and bounded, rotating JWKS caches.
// It creates no global key registry or background refresh goroutines and does not implement an authorization server.
package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"fmt"
	j "github.com/golang-jwt/jwt/v5"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ErrInvalidToken is safe to classify as authentication failure. Wrapped diagnostics must not be returned verbatim to clients.
var ErrInvalidToken = errors.New("jwt: token verification failed")
var ErrKeyNotFound = errors.New("jwt: signing key not found")

// Config requires issuer, audience, explicit algorithms, and either static verification keys or a fixed JWKS URL.
// Static []byte keys are HMAC secrets; remote JWKS may contain only asymmetric verification keys.
type Config struct {
	Issuer                                            string
	Audience, Algorithms                              []string
	Keys                                              map[string]any
	JWKSURL                                           string
	HTTPClient                                        *http.Client
	JWKSHeaders                                       http.Header
	CacheTTL, MinRefreshInterval, Leeway, HTTPTimeout time.Duration
	MaxTokenBytes                                     int
	MaxJWKSBytes                                      int64
	MaxKeys                                           int
	Now                                               func() time.Time
	TokenType                                         string
	AllowMissingKeyID, AllowInsecureHTTP              bool
}

// Claims contains verified JSON claim values. Context helpers return private copies.
type Claims map[string]any
type keyEntry struct {
	key       any
	algorithm string
}

// Verifier owns its cache and any internally constructed HTTP transport. Injected clients remain caller-owned.
type Verifier struct {
	config               Config
	parser               *j.Parser
	client               *http.Client
	owned                *http.Transport
	mu                   sync.RWMutex
	keys                 map[string]keyEntry
	expires, lastAttempt time.Time
	refreshMu            sync.Mutex
	closed               atomic.Bool
}

// New validates policy and fetches initial JWKS if configured. All network work respects ctx and a finite HTTP timeout.
func New(ctx context.Context, c Config) (*Verifier, error) {
	if ctx == nil {
		return nil, errors.New("jwt: context required")
	}
	if c.Issuer == "" || len(c.Audience) == 0 || len(c.Algorithms) == 0 || len(c.Keys) == 0 && c.JWKSURL == "" || len(c.Keys) > 0 && c.JWKSURL != "" {
		return nil, errors.New("jwt: issuer, audience, algorithms and exactly one key source required")
	}
	for _, a := range c.Audience {
		if a == "" {
			return nil, errors.New("jwt: empty audience")
		}
	}
	for _, a := range c.Algorithms {
		if a == "none" || j.GetSigningMethod(a) == nil {
			return nil, errors.New("jwt: unsupported algorithm")
		}
	}
	if c.CacheTTL < 0 || c.MinRefreshInterval < 0 || c.Leeway < 0 || c.HTTPTimeout < 0 || c.MaxTokenBytes < 0 || c.MaxJWKSBytes < 0 || c.MaxKeys < 0 {
		return nil, errors.New("jwt: invalid bounds")
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if c.CacheTTL == 0 {
		c.CacheTTL = 5 * time.Minute
	}
	if c.MinRefreshInterval == 0 {
		c.MinRefreshInterval = time.Second
	}
	if c.MinRefreshInterval > c.CacheTTL {
		return nil, errors.New("jwt: refresh interval exceeds cache lifetime")
	}
	if c.HTTPTimeout == 0 {
		c.HTTPTimeout = 5 * time.Second
	}
	if c.MaxTokenBytes == 0 {
		c.MaxTokenBytes = 16 << 10
	}
	if c.MaxJWKSBytes == 0 {
		c.MaxJWKSBytes = 1 << 20
	}
	if c.MaxJWKSBytes > 16<<20 {
		return nil, errors.New("jwt: JWKS size exceeds hard bound")
	}
	if c.MaxKeys == 0 {
		c.MaxKeys = 100
	}
	if c.MaxKeys > 1024 {
		return nil, errors.New("jwt: too many verification keys")
	}
	c.Algorithms = append([]string(nil), c.Algorithms...)
	c.Audience = append([]string(nil), c.Audience...)
	c.JWKSHeaders = c.JWKSHeaders.Clone()
	v := &Verifier{config: c, keys: map[string]keyEntry{}}
	if len(c.Keys) > c.MaxKeys {
		return nil, errors.New("jwt: too many static keys")
	}
	for id, key := range c.Keys {
		if id == "" || len(id) > 256 {
			return nil, errors.New("jwt: invalid key ID")
		}
		copy, e := verificationKey(key, c.Algorithms)
		if e != nil {
			return nil, e
		}
		v.keys[id] = keyEntry{key: copy}
	}
	v.config.Keys = nil
	v.parser = j.NewParser(j.WithValidMethods(c.Algorithms), j.WithIssuer(c.Issuer), j.WithAudience(c.Audience...), j.WithExpirationRequired(), j.WithIssuedAt(), j.WithLeeway(c.Leeway), j.WithTimeFunc(c.Now), j.WithJSONNumber())
	if c.JWKSURL != "" {
		u, e := url.Parse(c.JWKSURL)
		if e != nil || u.Host == "" || u.User != nil || u.Fragment != "" || u.Scheme != "https" && !(c.AllowInsecureHTTP && u.Scheme == "http") {
			return nil, errors.New("jwt: fixed HTTPS JWKS URL required")
		}
		client := http.Client{Timeout: c.HTTPTimeout}
		if c.HTTPClient != nil {
			client = *c.HTTPClient
		} else {
			transport := http.DefaultTransport.(*http.Transport).Clone()
			v.owned = transport
			client.Transport = transport
		}
		if client.Timeout == 0 || client.Timeout > c.HTTPTimeout {
			client.Timeout = c.HTTPTimeout
		}
		previous := client.CheckRedirect
		client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
			if r.URL.Host != u.Host || r.URL.Scheme != u.Scheme || len(via) >= 5 {
				return errors.New("jwt: JWKS redirect rejected")
			}
			if previous != nil {
				return previous(r, via)
			}
			return nil
		}
		v.client = &client
		if e = v.Refresh(ctx); e != nil {
			v.Close()
			return nil, e
		}
	}
	return v, nil
}
func verificationKey(key any, algorithms []string) (any, error) {
	switch k := key.(type) {
	case []byte:
		minimum := 32
		for _, a := range algorithms {
			if a == "HS384" {
				minimum = max(minimum, 48)
			}
			if a == "HS512" {
				minimum = 64
			}
		}
		if len(k) < minimum {
			return nil, errors.New("jwt: HMAC key too short")
		}
		return append([]byte(nil), k...), nil
	case *rsa.PublicKey:
		if k == nil || k.N == nil || k.N.BitLen() < 2048 || k.N.BitLen() > 8192 || k.E < 3 || k.E%2 == 0 {
			return nil, errors.New("jwt: invalid RSA public key")
		}
	case *ecdsa.PublicKey:
		if k == nil || k.Curve == nil {
			return nil, errors.New("jwt: invalid EC public key")
		}
		if _, err := k.Bytes(); err != nil {
			return nil, errors.New("jwt: invalid EC public key")
		}
	case ed25519.PublicKey:
		if len(k) != ed25519.PublicKeySize {
			return nil, errors.New("jwt: invalid Ed25519 public key")
		}
	default:
		return nil, errors.New("jwt: unsupported verification key type")
	}
	der, e := x509.MarshalPKIXPublicKey(key)
	if e != nil {
		return nil, e
	}
	return x509.ParsePKIXPublicKey(der)
}

// Verify validates signature, algorithm, issuer, audience, expiration and present not-before/issued-at claims.
// Header-controlled URLs are never fetched. Unknown critical extensions are rejected.
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	if ctx == nil {
		return nil, ErrInvalidToken
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if v.closed.Load() {
		return nil, errors.New("jwt: verifier closed")
	}
	if token == "" || len(token) > v.config.MaxTokenBytes {
		return nil, ErrInvalidToken
	}
	parsed, e := v.parser.ParseWithClaims(token, j.MapClaims{}, func(token *j.Token) (any, error) {
		if _, critical := token.Header["crit"]; critical {
			return nil, errors.New("jwt: unsupported critical header")
		}
		if v.config.TokenType != "" && token.Header["typ"] != v.config.TokenType {
			return nil, errors.New("jwt: unexpected token type")
		}
		id, _ := token.Header["kid"].(string)
		if id == "" && v.config.AllowMissingKeyID {
			v.mu.RLock()
			if len(v.keys) == 1 {
				for only := range v.keys {
					id = only
				}
			}
			v.mu.RUnlock()
		}
		if id == "" || len(id) > 256 {
			return nil, ErrKeyNotFound
		}
		key, e := v.lookup(ctx, id)
		if e != nil {
			return nil, e
		}
		if key.algorithm != "" && key.algorithm != token.Method.Alg() {
			return nil, errors.New("jwt: key algorithm mismatch")
		}
		return key.key, nil
	})
	if e != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidToken, e)
	}
	if !parsed.Valid {
		return nil, ErrInvalidToken
	}
	return Claims(parsed.Claims.(j.MapClaims)), nil
}
func (v *Verifier) lookup(ctx context.Context, id string) (keyEntry, error) {
	v.mu.RLock()
	key, ok := v.keys[id]
	fresh := v.config.JWKSURL == "" || v.config.Now().Before(v.expires)
	v.mu.RUnlock()
	if ok && fresh {
		return key, nil
	}
	if v.config.JWKSURL == "" {
		return keyEntry{}, ErrKeyNotFound
	}
	if e := v.refresh(ctx, false); e != nil {
		return keyEntry{}, e
	}
	v.mu.RLock()
	defer v.mu.RUnlock()
	key, ok = v.keys[id]
	if !ok || !v.config.Now().Before(v.expires) {
		return keyEntry{}, ErrKeyNotFound
	}
	return key, nil
}

// Close releases only an internally owned transport and prevents future verification/refresh operations.
func (v *Verifier) Close() error {
	if v.closed.CompareAndSwap(false, true) && v.owned != nil {
		v.owned.CloseIdleConnections()
	}
	return nil
}
