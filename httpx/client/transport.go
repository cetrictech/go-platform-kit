package client

import (
	"context"
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/id"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/cetrictech/go-platform-kit/retry"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrResponseTooLarge indicates a bounded stream exceeded its configured size. The underlying body has been closed.
var ErrResponseTooLarge = errors.New("http client: response limit exceeded")

func (c *Client) do(request *http.Request) (*http.Response, error) {
	if request == nil {
		return nil, errors.New("http client: nil request")
	}
	base := request.Clone(request.Context())
	if base.Header == nil {
		base.Header = make(http.Header)
	}
	for k, v := range c.headers {
		if _, exists := base.Header[k]; !exists {
			base.Header[k] = append([]string(nil), v...)
		}
	}
	if base.Header.Get("X-Request-ID") == "" {
		if c.config.RequestID != nil {
			base.Header.Set("X-Request-ID", c.config.RequestID(base.Context()))
		} else {
			u, e := id.NewV7()
			if e != nil {
				return nil, e
			}
			base.Header.Set("X-Request-ID", u.String())
		}
	}
	policy := retry.Policy{MaxAttempts: 1}
	if c.config.RetryPolicy != nil {
		policy = *c.config.RetryPolicy
	}
	operationCancel := func() {}
	if policy.MaxElapsed > 0 {
		operationCtx, cancel := context.WithTimeout(base.Context(), policy.MaxElapsed)
		operationCancel = cancel
		base = base.WithContext(operationCtx)
		policy.MaxElapsed = 0
	}
	policy.Clock = c.config.Clock
	policy.Idempotent = policy.Idempotent && c.retryMethods[base.Method] && (base.Body == nil || base.Body == http.NoBody || base.GetBody != nil)
	classify := policy.Retryable
	if classify == nil {
		classify = retryableHTTP
	}
	policy.Retryable = func(e error) bool {
		var status *StatusError
		if errors.As(e, &status) && status.RetryAfter > c.config.MaxRetryAfter {
			return false
		}
		return classify(e)
	}
	delay := policy.DelayFor
	policy.DelayFor = func(attempt int, e error, d time.Duration) time.Duration {
		if delay != nil {
			d = delay(attempt, e, d)
		}
		var status *StatusError
		if errors.As(e, &status) && status.RetryAfter > d {
			d = status.RetryAfter
		}
		return d
	}
	var response *http.Response
	onRetry := policy.OnRetry
	policy.OnRetry = func(attempt int, e error, d time.Duration) {
		if response != nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 32<<10))
			_ = response.Body.Close()
			response = nil
		}
		if onRetry != nil {
			onRetry(attempt, e, d)
		}
	}
	attempt := 0
	err := retry.Do(base.Context(), policy, func(ctx context.Context) error {
		attempt++
		r := base.Clone(ctx)
		if attempt > 1 && base.GetBody != nil {
			body, e := base.GetBody()
			if e != nil {
				return e
			}
			r.Body = body
		}
		var e error
		run := func(ctx context.Context) error {
			r = r.WithContext(ctx)
			response, e = c.attempt(r)
			if e != nil {
				return e
			}
			if response.StatusCode >= 400 {
				return &StatusError{Status: response.StatusCode, RetryAfter: parseRetryAfter(response.Header.Get("Retry-After"), c.config.Clock.Now())}
			}
			return nil
		}
		if c.config.Breaker != nil {
			return c.config.Breaker.Do(ctx, run)
		}
		return run(ctx)
	})
	if response != nil {
		response.Body = &contextBody{ReadCloser: response.Body, cancel: operationCancel}
		return response, nil
	}
	operationCancel()
	return nil, err
}
func (c *Client) attempt(r *http.Request) (*http.Response, error) {
	ctx, done := instrument.Start(r.Context(), c.config.Observer, "http.client.request")
	clone := r.Clone(ctx)
	if c.config.Propagate != nil {
		c.config.Propagate(ctx, clone.Header)
	}
	res, e := c.HTTP.Do(clone)
	if e != nil {
		done(e)
		return nil, e
	}
	if res.ContentLength > c.max {
		_ = res.Body.Close()
		done(ErrResponseTooLarge)
		return nil, ErrResponseTooLarge
	}
	res.Body = &boundedBody{ReadCloser: res.Body, max: c.max, done: done, status: res.StatusCode}
	return res, nil
}

type boundedBody struct {
	io.ReadCloser
	max, n   int64
	done     func(error)
	once     sync.Once
	status   int
	terminal error
}

func (b *boundedBody) finish(e error) {
	b.once.Do(func() {
		if e == nil && b.status >= 400 {
			e = &StatusError{Status: b.status}
		}
		b.done(e)
	})
}
func (b *boundedBody) Read(p []byte) (int, error) {
	if b.terminal != nil {
		return 0, b.terminal
	}
	if int64(len(p)) > b.max-b.n+1 {
		p = p[:b.max-b.n+1]
	}
	n, e := b.ReadCloser.Read(p)
	old := b.n
	b.n += int64(n)
	if b.n > b.max {
		b.terminal = ErrResponseTooLarge
		allowed := int(b.max - old)
		_ = b.ReadCloser.Close()
		b.finish(ErrResponseTooLarge)
		return allowed, ErrResponseTooLarge
	}
	if e != nil {
		if e == io.EOF {
			b.finish(nil)
		} else {
			b.finish(e)
		}
	}
	return n, e
}
func (b *boundedBody) Close() error { e := b.ReadCloser.Close(); b.finish(e); return e }
func parseRetryAfter(raw string, now time.Time) time.Duration {
	raw = strings.TrimSpace(raw)
	if seconds, e := strconv.ParseInt(raw, 10, 64); e == nil && seconds >= 0 {
		if seconds > int64((1<<63-1)/time.Second) {
			return time.Duration(1<<63 - 1)
		}
		return time.Duration(seconds) * time.Second
	}
	if when, e := http.ParseTime(raw); e == nil && when.After(now) {
		return when.Sub(now)
	}
	return 0
}
func retryableHTTP(e error) bool {
	if errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded) || errors.Is(e, ErrResponseTooLarge) {
		return false
	}
	var status *StatusError
	if errors.As(e, &status) {
		switch status.Status {
		case 408, 429, 500, 502, 503, 504:
			return true
		}
		return false
	}
	var network net.Error
	return errors.As(e, &network) && network.Timeout()
}

// Classify converts upstream/network failures into safe application errors without exposing response bodies.
func Classify(e error) error {
	if e == nil {
		return nil
	}
	var status *StatusError
	if errors.As(e, &status) {
		category := app.Network
		code := "HTTP_UPSTREAM_ERROR"
		if status.Status == 429 {
			category = app.RateLimit
			code = "HTTP_RATE_LIMIT"
		}
		return app.Wrap(e, "The upstream service could not complete the request.", app.WithCategory(category), app.WithCode(code), app.WithHTTPStatus(502), app.WithRetryable(retryableHTTP(e)), app.WithRetryAfter(status.RetryAfter))
	}
	return app.Classify(e)
}

type contextBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (b *contextBody) Read(p []byte) (int, error) {
	n, e := b.ReadCloser.Read(p)
	if e != nil {
		b.once.Do(b.cancel)
	}
	return n, e
}
func (b *contextBody) Close() error { e := b.ReadCloser.Close(); b.once.Do(b.cancel); return e }
