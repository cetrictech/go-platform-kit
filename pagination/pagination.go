// Package pagination implements bounded offset and authenticated cursor pagination.
package pagination

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Offset contains a validated page number and bounded page size.
type Offset struct {
	Page  int `json:"page"`
	Limit int `json:"limit"`
}

// OffsetMeta contains offset pagination totals and navigation flags.
type OffsetMeta struct {
	Offset
	Total       int64 `json:"total"`
	Pages       int64 `json:"pages"`
	HasNext     bool  `json:"has_next"`
	HasPrevious bool  `json:"has_previous"`
}

// ParseOffset parses and validates page/limit query parameters.
func ParseOffset(r *http.Request) (Offset, error) {
	p := Offset{1, 20}
	for k, d := range map[string]*int{"page": &p.Page, "limit": &p.Limit} {
		if v := r.URL.Query().Get(k); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil {
				return p, fmt.Errorf("pagination: invalid %s", k)
			}
			*d = n
		}
	}
	return p, p.Validate()
}

// Validate checks the supplied value according to package validation rules.
func (p Offset) Validate() error {
	if p.Page < 1 || p.Limit < 1 || p.Limit > 1000 || p.Page > int(^uint(0)>>1)/p.Limit {
		return errors.New("pagination: invalid page or limit")
	}
	return nil
}

// Skip returns the SQL offset; call Validate before use.
func (p Offset) Skip() int { return (p.Page - 1) * p.Limit }

// NewOffsetMeta calculates page counts and navigation flags without overflowing totals.
func NewOffsetMeta(p Offset, total int64) (OffsetMeta, error) {
	if e := p.Validate(); e != nil {
		return OffsetMeta{}, e
	}
	if total < 0 {
		return OffsetMeta{}, errors.New("pagination: negative total")
	}
	pages := total / int64(p.Limit)
	if total%int64(p.Limit) > 0 {
		pages++
	}
	return OffsetMeta{p, total, pages, int64(p.Page) < pages, p.Page > 1}, nil
}

// Cursor contains versioned, query-bound composite pagination keys.
type Cursor struct {
	Version   int               `json:"v"`
	KeyID     string            `json:"kid"`
	Direction string            `json:"direction"`
	Keys      []json.RawMessage `json:"keys"`
	Query     string            `json:"query"`
	Filter    string            `json:"filter"`
	Expires   int64             `json:"exp,omitempty"`
}

// Codec keys must be at least 32 random bytes. Keep previous keys available during rotation.
type Codec struct {
	ActiveKey string
	Keys      map[string][]byte
	TTL       time.Duration
	MaxBytes  int
	Now       func() time.Time
}

func (c Codec) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}
func (c Codec) max() int {
	if c.MaxBytes <= 0 {
		return 4096
	}
	return c.MaxBytes
}

// EncodeCursor authenticates cursor contents with the configured active HMAC key.
func (c Codec) EncodeCursor(v Cursor) (string, error) {
	v.Version = 1
	v.KeyID = c.ActiveKey
	if c.TTL < 0 {
		return "", errors.New("pagination: negative TTL")
	}
	if c.TTL > 0 {
		v.Expires = c.now().Add(c.TTL).Unix()
	}
	if e := validate(v); e != nil {
		return "", e
	}
	key := c.Keys[v.KeyID]
	if len(key) < 32 {
		return "", errors.New("pagination: invalid signing key")
	}
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	if len(b) > c.max() {
		return "", errors.New("pagination: cursor too large")
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(b)
	return base64.RawURLEncoding.EncodeToString(b) + "." + base64.RawURLEncoding.EncodeToString(h.Sum(nil)), nil
}

// DecodeCursor verifies size, signature, expiration and query/filter binding before returning keys.
func (c Codec) DecodeCursor(token, query, filter string) (Cursor, error) {
	var v Cursor
	if len(token) > base64.RawURLEncoding.EncodedLen(c.max())+45 {
		return v, errors.New("pagination: cursor too large")
	}
	p := strings.Split(token, ".")
	if len(p) != 2 {
		return v, errors.New("pagination: malformed cursor")
	}
	b, e := base64.RawURLEncoding.DecodeString(p[0])
	if e != nil || len(b) > c.max() {
		return v, errors.New("pagination: invalid payload")
	}
	if e = json.Unmarshal(b, &v); e != nil {
		return Cursor{}, errors.New("pagination: invalid JSON")
	}
	tag, e := base64.RawURLEncoding.DecodeString(p[1])
	key := c.Keys[v.KeyID]
	if e != nil || len(key) < 32 {
		return Cursor{}, errors.New("pagination: invalid signature")
	}
	h := hmac.New(sha256.New, key)
	_, _ = h.Write(b)
	if !hmac.Equal(tag, h.Sum(nil)) {
		return Cursor{}, errors.New("pagination: invalid signature")
	}
	if e = validate(v); e != nil {
		return Cursor{}, e
	}
	if v.Query != query || v.Filter != filter {
		return Cursor{}, errors.New("pagination: cursor binding mismatch")
	}
	if v.Expires != 0 && c.now().Unix() >= v.Expires {
		return Cursor{}, errors.New("pagination: cursor expired")
	}
	return v, nil
}
func validate(v Cursor) error {
	if v.Version != 1 || v.KeyID == "" || v.Query == "" || len(v.Keys) == 0 || (v.Direction != "next" && v.Direction != "previous") {
		return errors.New("pagination: invalid cursor")
	}
	return nil
}

// CursorMeta describes a limit+1 query. Previous navigation requires reversing all sort directions then reversing the returned rows.
type CursorMeta struct {
	Next        string `json:"next,omitempty"`
	Previous    string `json:"previous,omitempty"`
	HasNext     bool   `json:"has_next"`
	HasPrevious bool   `json:"has_previous"`
}

// NewCursorMeta constructs navigation metadata from issued cursors.
func NewCursorMeta(next, previous string) CursorMeta {
	return CursorMeta{next, previous, next != "", previous != ""}
}

// CursorRequest contains bounded query parameters. Ordering direction is authenticated inside the cursor.
type CursorRequest struct {
	Cursor string
	Limit  int
}

// ParseCursor parses bounded cursor and limit request parameters.
func ParseCursor(r *http.Request) (CursorRequest, error) {
	out := CursorRequest{Cursor: r.URL.Query().Get("cursor"), Limit: 20}
	if len(out.Cursor) > 8192 {
		return out, errors.New("pagination: cursor too large")
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, e := strconv.Atoi(raw)
		if e != nil {
			return out, errors.New("pagination: invalid limit")
		}
		out.Limit = n
	}
	if out.Limit < 1 || out.Limit > 1000 {
		return out, errors.New("pagination: limit must be 1..1000")
	}
	return out, nil
}

// ValidateCursor verifies a cursor without returning its decoded payload.
func (c Codec) ValidateCursor(token, query, filter string) error {
	_, e := c.DecodeCursor(token, query, filter)
	return e
}
