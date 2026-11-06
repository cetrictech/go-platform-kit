package jwt

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
)

type jwk struct {
	Kty    string   `json:"kty"`
	Kid    string   `json:"kid"`
	Use    string   `json:"use"`
	Alg    string   `json:"alg"`
	KeyOps []string `json:"key_ops"`
	N      string   `json:"n"`
	E      string   `json:"e"`
	Crv    string   `json:"crv"`
	X      string   `json:"x"`
	Y      string   `json:"y"`
}

// Refresh explicitly refreshes the fixed JWKS URL, bypassing the unknown-key request throttle.
func (v *Verifier) Refresh(ctx context.Context) error { return v.refresh(ctx, true) }
func (v *Verifier) refresh(ctx context.Context, explicit bool) error {
	if v.closed.Load() {
		return errors.New("jwt: verifier closed")
	}
	if v.config.JWKSURL == "" {
		return errors.New("jwt: no JWKS source configured")
	}
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()
	if e := ctx.Err(); e != nil {
		return e
	}
	now := v.config.Now()
	v.mu.Lock()
	if !explicit && !v.lastAttempt.IsZero() && now.Sub(v.lastAttempt) < v.config.MinRefreshInterval {
		v.mu.Unlock()
		return nil
	}
	v.lastAttempt = now
	v.mu.Unlock()
	request, e := http.NewRequestWithContext(ctx, http.MethodGet, v.config.JWKSURL, nil)
	if e != nil {
		return e
	}
	request.Header = v.config.JWKSHeaders.Clone()
	if request.Header == nil {
		request.Header = make(http.Header)
	}
	request.Header.Set("Accept", "application/json")
	res, e := v.client.Do(request)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("jwt: JWKS HTTP status %d", res.StatusCode)
	}
	raw, e := io.ReadAll(io.LimitReader(res.Body, v.config.MaxJWKSBytes+1))
	if e != nil {
		return e
	}
	if int64(len(raw)) > v.config.MaxJWKSBytes {
		return errors.New("jwt: JWKS size limit exceeded")
	}
	var document struct {
		Keys []jwk `json:"keys"`
	}
	if e = json.Unmarshal(raw, &document); e != nil {
		return errors.New("jwt: invalid JWKS JSON")
	}
	if len(document.Keys) == 0 || len(document.Keys) > v.config.MaxKeys {
		return errors.New("jwt: invalid JWKS key count")
	}
	keys := map[string]keyEntry{}
	for _, raw := range document.Keys {
		if raw.Use != "" && raw.Use != "sig" {
			continue
		}
		if len(raw.KeyOps) > 0 {
			verify := false
			for _, op := range raw.KeyOps {
				verify = verify || op == "verify"
			}
			if !verify {
				continue
			}
		}
		if raw.Kid == "" || len(raw.Kid) > 256 {
			return errors.New("jwt: JWKS key ID required")
		}
		if _, duplicate := keys[raw.Kid]; duplicate {
			return errors.New("jwt: duplicate JWKS key ID")
		}
		if raw.Alg != "" {
			allowed := false
			for _, a := range v.config.Algorithms {
				allowed = allowed || a == raw.Alg
			}
			if !allowed {
				continue
			}
		}
		key, e := parseJWK(raw)
		if e != nil {
			return e
		}
		key, e = verificationKey(key, v.config.Algorithms)
		if e != nil {
			return e
		}
		keys[raw.Kid] = keyEntry{key: key, algorithm: raw.Alg}
	}
	if len(keys) == 0 {
		return errors.New("jwt: no supported signing keys")
	}
	v.mu.Lock()
	v.keys = keys
	v.expires = v.config.Now().Add(v.config.CacheTTL)
	v.mu.Unlock()
	return nil
}
func parseJWK(k jwk) (any, error) {
	decode := base64.RawURLEncoding.DecodeString
	switch k.Kty {
	case "RSA":
		n, e := decode(k.N)
		if e != nil || len(n) < 256 || len(n) > 1024 {
			return nil, errors.New("jwt: invalid RSA modulus")
		}
		exp, e := decode(k.E)
		if e != nil || len(exp) == 0 || len(exp) > 4 {
			return nil, errors.New("jwt: invalid RSA exponent")
		}
		var exponent int64
		for _, b := range exp {
			exponent = exponent<<8 | int64(b)
		}
		if exponent > 1<<31-1 {
			return nil, errors.New("jwt: invalid RSA exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: int(exponent)}, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, errors.New("jwt: unsupported EC curve")
		}
		x, e := decode(k.X)
		if e != nil {
			return nil, errors.New("jwt: invalid EC coordinate")
		}
		y, e := decode(k.Y)
		if e != nil {
			return nil, errors.New("jwt: invalid EC coordinate")
		}
		length := (curve.Params().BitSize + 7) / 8
		if len(x) != length || len(y) != length {
			return nil, errors.New("jwt: invalid EC coordinate size")
		}
		point := append([]byte{4}, x...)
		point = append(point, y...)
		return ecdsa.ParseUncompressedPublicKey(curve, point)
	case "OKP":
		if k.Crv != "Ed25519" {
			return nil, errors.New("jwt: unsupported OKP curve")
		}
		key, e := decode(k.X)
		if e != nil || len(key) != ed25519.PublicKeySize {
			return nil, errors.New("jwt: invalid Ed25519 key")
		}
		return ed25519.PublicKey(key), nil
	default:
		return nil, errors.New("jwt: unsupported or symmetric remote JWK")
	}
}
