package pagination

import (
	"context"
	"errors"
	"sync"
)

// OffsetConfig can reduce the default/max page size and bound database offsets. MaxOffset=0 disables the additional offset cap.
type OffsetConfig struct{ DefaultLimit, MaxLimit, MaxOffset int }

func offsetConfig(options []OffsetConfig) (OffsetConfig, error) {
	c := OffsetConfig{DefaultLimit: 20, MaxLimit: 1000}
	if len(options) > 1 {
		return c, errors.New("pagination: at most one config allowed")
	}
	if len(options) == 1 {
		c = options[0]
		if c.DefaultLimit == 0 {
			c.DefaultLimit = 20
		}
		if c.MaxLimit == 0 {
			c.MaxLimit = 1000
		}
	}
	if c.DefaultLimit < 1 || c.MaxLimit < 1 || c.MaxLimit > 1000 || c.DefaultLimit > c.MaxLimit || c.MaxOffset < 0 {
		return c, errors.New("pagination: invalid offset config")
	}
	return c, nil
}

// ValidateOffset enforces bounds without computing an overflowing offset.
func ValidateOffset(p Offset, options ...OffsetConfig) error {
	c, e := offsetConfig(options)
	if e != nil {
		return e
	}
	if e = p.Validate(); e != nil {
		return e
	}
	if p.Limit > c.MaxLimit || c.MaxOffset > 0 && p.Skip() > c.MaxOffset {
		return errors.New("pagination: page size or offset exceeds configured maximum")
	}
	return nil
}

// CalculateOffset returns a validated zero-based SQL offset.
func CalculateOffset(p Offset) (int, error) {
	if e := p.Validate(); e != nil {
		return 0, e
	}
	return p.Skip(), nil
}

// KeyProvider is the consumer boundary for in-memory key rings, KMS-backed stores or other application-owned key sources.
type KeyProvider interface {
	Active(context.Context) (string, []byte, error)
	Key(context.Context, string) ([]byte, error)
}

// KeyRing supports atomic rotation. Keys are copied on write/read so callers cannot mutate concurrent signing material.
type KeyRing struct {
	mu     sync.RWMutex
	active string
	keys   map[string][]byte
}

// NewKeyRing validates the active key and retains previous verification keys for rotating cursors.
func NewKeyRing(active string, keys map[string][]byte) (*KeyRing, error) {
	r := &KeyRing{}
	if e := r.Rotate(active, keys); e != nil {
		return nil, e
	}
	return r, nil
}

// Rotate replaces the entire ring atomically. Retain old keys until their issued cursors expire.
func (r *KeyRing) Rotate(active string, keys map[string][]byte) error {
	if active == "" || len(active) > 128 || len(keys[active]) < 32 {
		return errors.New("pagination: invalid active signing key")
	}
	copy := make(map[string][]byte, len(keys))
	for id, key := range keys {
		if id == "" || len(id) > 128 || len(key) < 32 {
			return errors.New("pagination: invalid verification key")
		}
		copy[id] = append([]byte(nil), key...)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = active
	r.keys = copy
	return nil
}

// Active obtains the current signing identity and a private key copy.
func (r *KeyRing) Active(ctx context.Context) (string, []byte, error) {
	if e := ctx.Err(); e != nil {
		return "", nil, e
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.active == "" {
		return "", nil, errors.New("pagination: uninitialized key ring")
	}
	return r.active, append([]byte(nil), r.keys[r.active]...), nil
}

// Key looks up an allowed verification key; unknown key IDs never trigger network discovery.
func (r *KeyRing) Key(ctx context.Context, id string) ([]byte, error) {
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	key, ok := r.keys[id]
	if !ok {
		return nil, errors.New("pagination: unknown key ID")
	}
	return append([]byte(nil), key...), nil
}
