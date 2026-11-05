// Package security provides scoped standard cryptographic utilities.
package security

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"github.com/cetrictech/go-platform-kit/internal/redact"
	"golang.org/x/crypto/bcrypt"
)

// RandomBytes returns cryptographically secure random bytes of a bounded positive length.
func RandomBytes(n int) ([]byte, error) {
	if n < 1 || n > 1<<20 {
		return nil, errors.New("security: size must be 1..1048576")
	}
	b := make([]byte, n)
	_, e := rand.Read(b)
	return b, e
}

// Token returns a base64url token backed by cryptographic randomness.
func Token(n int) (string, error) {
	b, e := RandomBytes(n)
	return base64.RawURLEncoding.EncodeToString(b), e
}

// Equal compares byte strings using constant-time comparison.
func Equal(a, b []byte) bool { return subtle.ConstantTimeCompare(a, b) == 1 }

// SHA256 returns the SHA-256 digest; do not use this as a password hash.
func SHA256(b []byte) [32]byte { return sha256.Sum256(b) }

// Sign computes an HMAC-SHA256 tag with a key of at least 32 bytes.
func Sign(key, data []byte) ([]byte, error) {
	if len(key) < 32 {
		return nil, errors.New("security: HMAC key must contain at least 32 bytes")
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(data)
	return h.Sum(nil), nil
}

// Verify checks an HMAC-SHA256 authentication tag.
func Verify(key, data, tag []byte) bool {
	s, e := Sign(key, data)
	return e == nil && hmac.Equal(s, tag)
}

// HashPassword hashes a password with an explicit validated bcrypt cost.
func HashPassword(password []byte, cost int) ([]byte, error) {
	if len(password) == 0 {
		return nil, errors.New("security: empty password")
	}
	if cost < 10 || cost > 16 {
		return nil, errors.New("security: bcrypt cost must be 10..16")
	}
	return bcrypt.GenerateFromPassword(password, cost)
}

// CheckPassword verifies a bcrypt password hash.
func CheckPassword(hash, password []byte) error { return bcrypt.CompareHashAndPassword(hash, password) }

// Sensitive identifies structured fields that must never be logged.
func Sensitive(key string) bool { return redact.Sensitive(key) }

// Redact returns a recursively redacted copy of JSON-like data. Free text must not contain secrets.
func Redact(v any) any { return redact.Redact(v) }

// Zero clears the supplied byte slice; copies are outside its control.
func Zero(b []byte) { clear(b) }
