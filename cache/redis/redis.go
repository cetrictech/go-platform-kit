// Package redis provides isolated go-redis integration and ownership-token locks.
package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"github.com/cetrictech/go-platform-kit/encodingx"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/cetrictech/go-platform-kit/security"
	r "github.com/redis/go-redis/v9"
	"strings"
	"time"
)

var ErrNotFound = r.Nil
var ErrLockBusy = errors.New("redis: lock busy")
var ErrOwnership = errors.New("redis: lock ownership lost")

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Mode                                         Mode
	SentinelUsername, SentinelPassword           string
	AllowPlaintext                               bool
	Observer                                     instrument.Hook
	Addresses                                    []string
	MasterName, Username, Password, Prefix       string
	DB, PoolSize, MinIdleConnections, MaxRetries int
	DialTimeout, ReadTimeout, WriteTimeout       time.Duration
	TLS                                          *tls.Config
	Serializer                                   encodingx.Serializer
}

// Client owns its configured infrastructure client and supports concurrent operations.
type Client struct {
	client     r.UniversalClient
	prefix     string
	serializer encodingx.Serializer
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(ctx context.Context, c Config) (*Client, error) {
	if len(c.Addresses) == 0 || c.PoolSize < 0 || c.DB < 0 || c.MinIdleConnections < 0 || c.DialTimeout < 0 || c.ReadTimeout < 0 || c.WriteTimeout < 0 || c.MaxRetries < -1 {
		return nil, errors.New("redis: invalid config")
	}
	for _, a := range c.Addresses {
		if strings.TrimSpace(a) == "" {
			return nil, errors.New("redis: empty address")
		}
	}
	mode := c.Mode
	if mode == "" {
		if c.MasterName != "" {
			mode = Sentinel
		} else if len(c.Addresses) > 1 {
			mode = Cluster
		} else {
			mode = Standalone
		}
	}
	switch mode {
	case Standalone:
		if c.MasterName != "" || len(c.Addresses) != 1 {
			return nil, errors.New("redis: standalone requires one address and no master")
		}
	case Sentinel:
		if c.MasterName == "" {
			return nil, errors.New("redis: sentinel requires master name")
		}
	case Cluster:
		if c.MasterName != "" || c.DB != 0 {
			return nil, errors.New("redis: cluster requires DB zero and no sentinel master")
		}
	default:
		return nil, errors.New("redis: invalid connection mode")
	}
	if c.TLS != nil && c.TLS.InsecureSkipVerify && c.TLS.VerifyPeerCertificate == nil && c.TLS.VerifyConnection == nil {
		return nil, errors.New("redis: TLS verification required")
	}
	if c.TLS == nil && !c.AllowPlaintext {
		return nil, errors.New("redis: TLS required; explicitly opt into plaintext for local development")
	}
	o := &r.UniversalOptions{SentinelUsername: c.SentinelUsername, SentinelPassword: c.SentinelPassword, ContextTimeoutEnabled: true, Addrs: append([]string(nil), c.Addresses...), MasterName: c.MasterName, Username: c.Username, Password: c.Password, DB: c.DB, PoolSize: c.PoolSize, MinIdleConns: c.MinIdleConnections, MaxRetries: c.MaxRetries, DialTimeout: c.DialTimeout, ReadTimeout: c.ReadTimeout, WriteTimeout: c.WriteTimeout}
	if c.TLS != nil {
		o.TLSConfig = c.TLS.Clone()
	}
	var native r.UniversalClient
	switch mode {
	case Standalone:
		native = r.NewClient(o.Simple())
	case Sentinel:
		native = r.NewFailoverClient(o.Failover())
	case Cluster:
		native = r.NewClusterClient(o.Cluster())
	}
	if c.Observer != nil {
		native.AddHook(redisHook{c.Observer})
	}
	if e := native.Ping(ctx).Err(); e != nil {
		native.Close()
		return nil, e
	}
	s := c.Serializer
	if s == nil {
		s = encodingx.JSON{}
	}
	return &Client{native, c.Prefix, s}, nil
}

// Native provides an explicit advanced driver escape hatch; preserve SDK ownership rules.
func (c *Client) Native() r.UniversalClient { return c.client }
func (c *Client) key(k string) string       { return c.prefix + k }
func (c *Client) keys(keys []string) []string {
	out := make([]string, len(keys))
	for i := range keys {
		out[i] = c.key(keys[i])
	}
	return out
}

// Ping checks connectivity using the supplied context.
func (c *Client) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }

// Close releases owned resources. See package documentation for shutdown ordering.
func (c *Client) Close() error { return c.client.Close() }

// PoolStats contains the Redis connection pool statistics.
type PoolStats = r.PoolStats

// Stats returns a concurrency-safe statistics snapshot.
func (c *Client) Stats() *PoolStats { return c.client.PoolStats() }

// SetJSON serializes and stores a value with expiration.
func (c *Client) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	b, e := c.serializer.Encode(value)
	if e != nil {
		return e
	}
	return c.Set(ctx, key, b, ttl)
}

// GetJSON reads and deserializes a stored value.
func (c *Client) GetJSON(ctx context.Context, key string, dst any) error {
	b, e := c.GetBytes(ctx, key)
	if e != nil {
		return e
	}
	return c.serializer.Decode(b, dst)
}

// CacheAside may run load concurrently for misses. Use external coordination for expensive duplicate loads.
func (c *Client) CacheAside(ctx context.Context, key string, dst any, ttl time.Duration, load func(context.Context) (any, error)) error {
	if load == nil {
		return errors.New("redis: cache loader required")
	}
	e := c.GetJSON(ctx, key, dst)
	if e == nil {
		return nil
	}
	if !errors.Is(e, ErrNotFound) {
		return e
	}
	v, e := load(ctx)
	if e != nil {
		return e
	}
	b, e := c.serializer.Encode(v)
	if e != nil {
		return e
	}
	if e = c.serializer.Decode(b, dst); e != nil {
		return e
	}
	return c.Set(ctx, key, b, ttl)
}

// Eval executes Lua with namespaced keys and bound arguments.
func (c *Client) Eval(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return c.client.Eval(ctx, script, c.keys(keys), args...).Result()
}

// EvalSHA executes a cached Lua script with namespaced keys.
func (c *Client) EvalSHA(ctx context.Context, sha string, keys []string, args ...any) (any, error) {
	return c.client.EvalSha(ctx, sha, c.keys(keys), args...).Result()
}

// ScriptLoad loads a script into the Redis script cache.
func (c *Client) ScriptLoad(ctx context.Context, script string) (string, error) {
	return c.client.ScriptLoad(ctx, script).Result()
}

// Pipeline is an advanced native callback. Keys inside the callback are not automatically namespaced.
func (c *Client) Pipeline(ctx context.Context, fn func(r.Pipeliner) error) error {
	_, e := c.client.Pipelined(ctx, fn)
	return e
}

// Transaction executes an advanced native MULTI/EXEC pipeline callback.
func (c *Client) Transaction(ctx context.Context, fn func(r.Pipeliner) error) error {
	_, e := c.client.TxPipelined(ctx, fn)
	return e
}

// Watch executes an advanced native optimistic transaction callback.
func (c *Client) Watch(ctx context.Context, fn func(*r.Tx) error, keys ...string) error {
	return c.client.Watch(ctx, fn, c.keys(keys)...)
}

// Message contains transport-independent message data.
type Message struct{ Channel, Pattern, Payload string }

// Subscribe owns its subscription until ctx cancellation or handler error. It waits for the subscription acknowledgment.
func (c *Client) Subscribe(ctx context.Context, patterns bool, handler func(context.Context, Message) error, channels ...string) error {
	if handler == nil || len(channels) == 0 {
		return errors.New("redis: invalid subscription")
	}
	var sub *r.PubSub
	if patterns {
		patterns := make([]string, len(channels))
		prefix := escapeGlob(c.prefix)
		for i, p := range channels {
			patterns[i] = prefix + p
		}
		sub = c.client.PSubscribe(ctx, patterns...)
	} else {
		sub = c.client.Subscribe(ctx, c.keys(channels)...)
	}
	defer sub.Close()
	if _, e := sub.Receive(ctx); e != nil {
		return e
	}
	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case m, ok := <-ch:
			if !ok {
				return errors.New("redis: subscription closed")
			}
			if e := handler(ctx, Message{strings.TrimPrefix(m.Channel, c.prefix), strings.TrimPrefix(m.Pattern, escapeGlob(c.prefix)), m.Payload}); e != nil {
				return e
			}
		}
	}
}

// Lock identifies a time-limited Redis ownership lease, without fencing guarantees.
type Lock struct {
	client     *Client
	key, token string
}

// Acquire attempts to acquire an expiring lock with a cryptographic ownership token.
func (c *Client) Acquire(ctx context.Context, key string, ttl time.Duration) (*Lock, error) {
	if key == "" || ttl < time.Millisecond {
		return nil, errors.New("redis: invalid lock key or TTL")
	}
	token, e := security.Token(32)
	if e != nil {
		return nil, e
	}
	ok, e := c.SetNX(ctx, key, token, ttl)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, ErrLockBusy
	}
	return &Lock{c, key, token}, nil
}

// Release atomically deletes a lock only if its ownership token matches.
func (l *Lock) Release(ctx context.Context) error {
	v, e := l.client.Eval(ctx, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`, []string{l.key}, l.token)
	if e != nil {
		return e
	}
	if v != int64(1) {
		return ErrOwnership
	}
	return nil
}

// Renew extends a current processing lease.
func (l *Lock) Renew(ctx context.Context, ttl time.Duration) error {
	if ttl < time.Millisecond {
		return errors.New("redis: invalid TTL")
	}
	v, e := l.client.Eval(ctx, `if redis.call('GET',KEYS[1]) == ARGV[1] then return redis.call('PEXPIRE',KEYS[1],ARGV[2]) else return 0 end`, []string{l.key}, l.token, ttl.Milliseconds())
	if e != nil {
		return e
	}
	if v != int64(1) {
		return ErrOwnership
	}
	return nil
}

// Owned checks whether this lease still owns the stored token.
func (l *Lock) Owned(ctx context.Context) (bool, error) {
	v, e := l.client.Get(ctx, l.key)
	if errors.Is(e, ErrNotFound) {
		return false, nil
	}
	return v == l.token, e
}

// RateLimit is an atomic fixed-window counter. A key must describe one policy/window.
func (c *Client) RateLimit(ctx context.Context, key string, limit int64, window time.Duration) (allowed bool, remaining int64, retry time.Duration, err error) {
	if limit < 1 || window < time.Millisecond {
		return false, 0, 0, errors.New("redis: invalid rate policy")
	}
	v, e := c.Eval(ctx, `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('PEXPIRE',KEYS[1],ARGV[1]) end; return {n,redis.call('PTTL',KEYS[1])}`, []string{key}, window.Milliseconds())
	if e != nil {
		return false, 0, 0, e
	}
	a := v.([]any)
	n := a[0].(int64)
	remaining = max(0, limit-n)
	return n <= limit, remaining, time.Duration(a[1].(int64)) * time.Millisecond, nil
}

// ScoredMember describes a sorted-set member and score.
type ScoredMember = r.Z

// Stream contains Redis stream messages.
type Stream = r.XStream

// StreamMessage contains one Redis stream entry.
type StreamMessage = r.XMessage

// Pending summarizes pending stream deliveries.
type Pending = r.XPending

// Set stores a value with the requested expiration.
func (c *Client) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	return c.client.Set(ctx, c.key(key), value, ttl).Err()
}

// Get reads one value into the requested representation.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	return c.client.Get(ctx, c.key(key)).Result()
}

// GetBytes returns the stored value as bytes, or ErrNotFound.
func (c *Client) GetBytes(ctx context.Context, key string) ([]byte, error) {
	return c.client.Get(ctx, c.key(key)).Bytes()
}

// Delete deletes the specified keys.
func (c *Client) Delete(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Del(ctx, c.keys(keys)...).Result()
}

// Exists counts existing keys.
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	return c.client.Exists(ctx, c.keys(keys)...).Result()
}

// Expire updates key expiration.
func (c *Client) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return c.client.Expire(ctx, c.key(key), ttl).Result()
}

// TTL returns remaining key lifetime using Redis duration semantics.
func (c *Client) TTL(ctx context.Context, key string) (time.Duration, error) {
	return c.client.TTL(ctx, c.key(key)).Result()
}

// Increment atomically increments a key.
func (c *Client) Increment(ctx context.Context, key string) (int64, error) {
	return c.client.Incr(ctx, c.key(key)).Result()
}

// Decrement atomically decrements a key.
func (c *Client) Decrement(ctx context.Context, key string) (int64, error) {
	return c.client.Decr(ctx, c.key(key)).Result()
}

// IncrementBy atomically adds the requested integer.
func (c *Client) IncrementBy(ctx context.Context, key string, n int64) (int64, error) {
	return c.client.IncrBy(ctx, c.key(key), n).Result()
}

// GetAndDelete atomically reads and removes a key.
func (c *Client) GetAndDelete(ctx context.Context, key string) (string, error) {
	return c.client.GetDel(ctx, c.key(key)).Result()
}

// SetNX sets a value only when its key is absent.
func (c *Client) SetNX(ctx context.Context, key string, value any, ttl time.Duration) (bool, error) {
	return c.client.SetNX(ctx, c.key(key), value, ttl).Result()
}

// HSet sets hash fields.
func (c *Client) HSet(ctx context.Context, key string, values ...any) (int64, error) {
	return c.client.HSet(ctx, c.key(key), values...).Result()
}

// HGet reads a hash field.
func (c *Client) HGet(ctx context.Context, key, field string) (string, error) {
	return c.client.HGet(ctx, c.key(key), field).Result()
}

// HGetAll reads all hash fields.
func (c *Client) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	return c.client.HGetAll(ctx, c.key(key)).Result()
}

// HDel removes hash fields.
func (c *Client) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	return c.client.HDel(ctx, c.key(key), fields...).Result()
}

// HExists checks whether a hash field exists.
func (c *Client) HExists(ctx context.Context, key, field string) (bool, error) {
	return c.client.HExists(ctx, c.key(key), field).Result()
}

// HIncrBy atomically increments a hash field.
func (c *Client) HIncrBy(ctx context.Context, key, field string, n int64) (int64, error) {
	return c.client.HIncrBy(ctx, c.key(key), field, n).Result()
}

// LPush prepends list values.
func (c *Client) LPush(ctx context.Context, key string, values ...any) (int64, error) {
	return c.client.LPush(ctx, c.key(key), values...).Result()
}

// RPush appends list values.
func (c *Client) RPush(ctx context.Context, key string, values ...any) (int64, error) {
	return c.client.RPush(ctx, c.key(key), values...).Result()
}

// LPop removes and returns the first list value.
func (c *Client) LPop(ctx context.Context, key string) (string, error) {
	return c.client.LPop(ctx, c.key(key)).Result()
}

// RPop removes and returns the last list value.
func (c *Client) RPop(ctx context.Context, key string) (string, error) {
	return c.client.RPop(ctx, c.key(key)).Result()
}

// LRange reads an inclusive list range.
func (c *Client) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.LRange(ctx, c.key(key), start, stop).Result()
}

// LTrim retains an inclusive list range.
func (c *Client) LTrim(ctx context.Context, key string, start, stop int64) error {
	return c.client.LTrim(ctx, c.key(key), start, stop).Err()
}

// SAdd adds set members.
func (c *Client) SAdd(ctx context.Context, key string, members ...any) (int64, error) {
	return c.client.SAdd(ctx, c.key(key), members...).Result()
}

// SRem removes set members.
func (c *Client) SRem(ctx context.Context, key string, members ...any) (int64, error) {
	return c.client.SRem(ctx, c.key(key), members...).Result()
}

// SMembers returns set members.
func (c *Client) SMembers(ctx context.Context, key string) ([]string, error) {
	return c.client.SMembers(ctx, c.key(key)).Result()
}

// SIsMember tests set membership.
func (c *Client) SIsMember(ctx context.Context, key string, member any) (bool, error) {
	return c.client.SIsMember(ctx, c.key(key), member).Result()
}

// SCard returns set cardinality.
func (c *Client) SCard(ctx context.Context, key string) (int64, error) {
	return c.client.SCard(ctx, c.key(key)).Result()
}

// ZAdd adds scored sorted-set members.
func (c *Client) ZAdd(ctx context.Context, key string, members ...ScoredMember) (int64, error) {
	return c.client.ZAdd(ctx, c.key(key), members...).Result()
}

// ZRem removes sorted-set members.
func (c *Client) ZRem(ctx context.Context, key string, members ...any) (int64, error) {
	return c.client.ZRem(ctx, c.key(key), members...).Result()
}

// ZRange reads a sorted-set index range.
func (c *Client) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return c.client.ZRange(ctx, c.key(key), start, stop).Result()
}

// ZRangeByScore reads a sorted-set score range.
func (c *Client) ZRangeByScore(ctx context.Context, key string, min, max string) ([]string, error) {
	return c.client.ZRangeByScore(ctx, c.key(key), &r.ZRangeBy{Min: min, Max: max}).Result()
}

// ZScore reads a member score.
func (c *Client) ZScore(ctx context.Context, key, member string) (float64, error) {
	return c.client.ZScore(ctx, c.key(key), member).Result()
}

// ZCard returns sorted-set cardinality.
func (c *Client) ZCard(ctx context.Context, key string) (int64, error) {
	return c.client.ZCard(ctx, c.key(key)).Result()
}

// Publish publishes with the configured acknowledgment policy.
func (c *Client) Publish(ctx context.Context, channel string, value any) (int64, error) {
	return c.client.Publish(ctx, c.key(channel), value).Result()
}

// XAdd appends a stream entry.
func (c *Client) XAdd(ctx context.Context, key string, values map[string]any) (string, error) {
	return c.client.XAdd(ctx, &r.XAddArgs{Stream: c.key(key), Values: values}).Result()
}

// XAck acknowledges stream entries for a consumer group.
func (c *Client) XAck(ctx context.Context, key, group string, ids ...string) (int64, error) {
	return c.client.XAck(ctx, c.key(key), group, ids...).Result()
}

// XGroupCreate creates a stream consumer group, creating the stream if needed.
func (c *Client) XGroupCreate(ctx context.Context, key, group, start string) error {
	return c.client.XGroupCreateMkStream(ctx, c.key(key), group, start).Err()
}

// XGroupDestroy removes a stream consumer group.
func (c *Client) XGroupDestroy(ctx context.Context, key, group string) (int64, error) {
	return c.client.XGroupDestroy(ctx, c.key(key), group).Result()
}

// XDelete deletes stream entries.
func (c *Client) XDelete(ctx context.Context, key string, ids ...string) (int64, error) {
	return c.client.XDel(ctx, c.key(key), ids...).Result()
}

// XPending summarizes pending consumer-group entries.
func (c *Client) XPending(ctx context.Context, key, group string) (*Pending, error) {
	return c.client.XPending(ctx, c.key(key), group).Result()
}

// XClaim transfers ownership of sufficiently idle pending entries.
func (c *Client) XClaim(ctx context.Context, key, group, consumer string, idle time.Duration, ids ...string) ([]StreamMessage, error) {
	return c.client.XClaim(ctx, &r.XClaimArgs{Stream: c.key(key), Group: group, Consumer: consumer, MinIdle: idle, Messages: ids}).Result()
}

// XRead reads a stream from the supplied entry ID.
func (c *Client) XRead(ctx context.Context, key, lastID string, count int64, block time.Duration) ([]Stream, error) {
	return c.client.XRead(ctx, &r.XReadArgs{Streams: []string{c.key(key), lastID}, Count: count, Block: block}).Result()
}

// XReadGroup reads stream entries for a consumer group.
func (c *Client) XReadGroup(ctx context.Context, key, group, consumer, lastID string, count int64, block time.Duration) ([]Stream, error) {
	return c.client.XReadGroup(ctx, &r.XReadGroupArgs{Group: group, Consumer: consumer, Streams: []string{c.key(key), lastID}, Count: count, Block: block}).Result()
}

// XAutoClaim claims idle pending entries and returns the next scan position.
func (c *Client) XAutoClaim(ctx context.Context, key, group, consumer, start string, idle time.Duration, count int64) ([]StreamMessage, string, error) {
	return c.client.XAutoClaim(ctx, &r.XAutoClaimArgs{Stream: c.key(key), Group: group, Consumer: consumer, Start: start, MinIdle: idle, Count: count}).Result()
}

// Mode explicitly selects a universal-client topology, including cluster discovery from a single seed.
type Mode string

const (
	Standalone Mode = "standalone"
	Sentinel   Mode = "sentinel"
	Cluster    Mode = "cluster"
)

// KeepTTL preserves an existing key lifetime on Set, following Redis KEEPTTL semantics.
const KeepTTL time.Duration = r.KeepTTL

func escapeGlob(prefix string) string {
	var b strings.Builder
	for _, r := range prefix {
		switch r {
		case '*', '?', '[', ']', '\\':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
