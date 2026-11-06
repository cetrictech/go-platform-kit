package redis

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cetrictech/go-platform-kit/idempotency"
	"github.com/cetrictech/go-platform-kit/ratelimit"
	"github.com/cetrictech/go-platform-kit/security"
	"time"
)

// IdempotencyStore stores only fingerprints, ownership tokens and response status, never response bodies.
type IdempotencyStore struct{ Client *Client }

// Begin atomically attempts to acquire a fingerprint-bound processing lease.
func (s IdempotencyStore) Begin(ctx context.Context, key, fingerprint string, lease time.Duration) (idempotency.Record, error) {
	if key == "" || fingerprint == "" || lease < time.Millisecond {
		return idempotency.Record{}, errors.New("redis: invalid idempotency request")
	}
	token, e := security.Token(32)
	if e != nil {
		return idempotency.Record{}, e
	}
	r := idempotency.Record{Fingerprint: fingerprint, Token: token, State: idempotency.Processing, Expires: time.Now().Add(lease)}
	b, e := json.Marshal(r)
	if e != nil {
		return r, e
	}
	v, e := s.Client.Eval(ctx, `local old=redis.call('GET',KEYS[1]); if old then local r=cjson.decode(old); if r.fingerprint ~= ARGV[1] then return {1,old} end; if r.state == 'processing' then return {2,old} end; if r.state == 'completed' then return {3,old} end end; redis.call('SET',KEYS[1],ARGV[2],'PX',ARGV[3]); return {0,ARGV[2]}`, []string{key}, fingerprint, string(b), lease.Milliseconds())
	if e != nil {
		return r, e
	}
	a := v.([]any)
	if e = json.Unmarshal([]byte(a[1].(string)), &r); e != nil {
		return r, e
	}
	switch a[0].(int64) {
	case 1:
		return r, idempotency.ErrMismatch
	case 2:
		return r, idempotency.ErrBusy
	case 3:
		return r, idempotency.ErrCompleted
	}
	return r, nil
}
func (s IdempotencyStore) change(ctx context.Context, key, token string, state idempotency.State, status int, ttl time.Duration) error {
	if ttl < time.Millisecond {
		return errors.New("redis: invalid idempotency TTL")
	}
	v, e := s.Client.Eval(ctx, `local old=redis.call('GET',KEYS[1]); if not old then return 0 end; local r=cjson.decode(old); if r.token ~= ARGV[1] or r.state ~= 'processing' then return 0 end; r.state=ARGV[2]; r.status=tonumber(ARGV[3]); r.expires=ARGV[5]; redis.call('SET',KEYS[1],cjson.encode(r),'PX',ARGV[4]); return 1`, []string{key}, token, string(state), status, ttl.Milliseconds(), time.Now().Add(ttl).UTC().Format(time.RFC3339Nano))
	if e != nil {
		return e
	}
	if v != int64(1) {
		return idempotency.ErrOwnership
	}
	return nil
}

// Complete records successful processing only for the current ownership token.
func (s IdempotencyStore) Complete(ctx context.Context, key, token string, status int, ttl time.Duration) error {
	return s.change(ctx, key, token, idempotency.Completed, status, ttl)
}

// Fail records failed processing only for the current ownership token.
func (s IdempotencyStore) Fail(ctx context.Context, key, token string, ttl time.Duration) error {
	return s.change(ctx, key, token, idempotency.Failed, 0, ttl)
}

// Renew extends a current processing lease.
func (s IdempotencyStore) Renew(ctx context.Context, key, token string, ttl time.Duration) error {
	return s.change(ctx, key, token, idempotency.Processing, 0, ttl)
}

// Limiter uses atomic Redis fixed windows, unlike the in-memory token bucket.
type Limiter struct {
	Client *Client
	Limit  int64
	Window time.Duration
}

// Allow evaluates the configured rate policy for a key.
func (l Limiter) Allow(ctx context.Context, key string) (ratelimit.Decision, error) {
	ok, remaining, retry, e := l.Client.RateLimit(ctx, key, l.Limit, l.Window)
	return ratelimit.Decision{Allowed: ok, Remaining: int(remaining), RetryAfter: retry, Limit: int(l.Limit)}, e
}

var _ idempotency.Store = IdempotencyStore{}
var _ ratelimit.Limiter = Limiter{}
