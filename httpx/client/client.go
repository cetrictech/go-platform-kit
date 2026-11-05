// Package client constructs bounded, context-aware native HTTP clients.
package client

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"github.com/cetrictech/go-platform-kit/clock"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/cetrictech/go-platform-kit/resilience"
	"github.com/cetrictech/go-platform-kit/retry"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	HTTPClient                        *http.Client
	ResponseHeaderTimeout             time.Duration
	Headers                           http.Header
	RequestID                         func(context.Context) string
	Propagate                         func(context.Context, http.Header)
	RetryPolicy                       *retry.Policy
	RetryMethods                      []string
	MaxRetryAfter                     time.Duration
	Clock                             clock.Clock
	Observer                          instrument.Hook
	Breaker                           *resilience.Breaker
	Timeout, DialTimeout, IdleTimeout time.Duration
	MaxIdleConns, MaxConnsPerHost     int
	TLS                               *tls.Config
	Transport                         http.RoundTripper
	MaxResponseBytes                  int64
}

// Client owns its configured infrastructure client and supports concurrent operations.
type Client struct {
	HTTP         *http.Client
	max          int64
	owned        *http.Transport
	config       Config
	headers      http.Header
	retryMethods map[string]bool
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(c Config) (*Client, error) {
	if c.Timeout < 0 || c.DialTimeout < 0 || c.MaxResponseBytes < 0 || c.MaxResponseBytes == int64(1<<63-1) || c.IdleTimeout < 0 || c.ResponseHeaderTimeout < 0 || c.MaxRetryAfter < 0 || c.MaxIdleConns < 0 || c.MaxConnsPerHost < 0 {
		return nil, errors.New("http client: invalid config")
	}
	if c.Timeout == 0 {
		c.Timeout = 30 * time.Second
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = 5 * time.Second
	}
	if c.IdleTimeout == 0 {
		c.IdleTimeout = 90 * time.Second
	}
	if c.MaxIdleConns == 0 {
		c.MaxIdleConns = 100
	}
	if c.MaxResponseBytes == 0 {
		c.MaxResponseBytes = 4 << 20
	}
	if c.MaxRetryAfter == 0 {
		c.MaxRetryAfter = time.Minute
	}
	if c.Clock == nil {
		c.Clock = clock.Real{}
	}
	if c.HTTPClient != nil && c.Transport != nil {
		return nil, errors.New("http client: specify HTTPClient or Transport, not both")
	}
	for _, values := range c.Headers {
		for _, v := range values {
			if strings.ContainsAny(v, "\r\n") {
				return nil, errors.New("http client: invalid default header")
			}
		}
	}
	if c.RetryPolicy != nil {
		p := *c.RetryPolicy
		if p.MaxAttempts < 1 || p.Delay < 0 || p.MaxDelay < 0 || p.MaxElapsed < 0 {
			return nil, errors.New("http client: invalid retry policy")
		}
		c.RetryPolicy = &p
	}
	out := &Client{max: c.MaxResponseBytes, config: c, headers: c.Headers.Clone(), retryMethods: map[string]bool{}}
	methods := c.RetryMethods
	if len(methods) == 0 {
		methods = []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions}
	}
	for _, m := range methods {
		out.retryMethods[m] = true
	}
	if c.HTTPClient != nil {
		out.HTTP = c.HTTPClient
		return out, nil
	}
	tr := c.Transport
	if tr == nil {
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.DialContext = (&net.Dialer{Timeout: c.DialTimeout, KeepAlive: 30 * time.Second}).DialContext
		t.IdleConnTimeout = c.IdleTimeout
		t.ResponseHeaderTimeout = c.ResponseHeaderTimeout
		t.ForceAttemptHTTP2 = true
		t.MaxIdleConns = c.MaxIdleConns
		t.MaxConnsPerHost = c.MaxConnsPerHost
		if c.TLS != nil {
			t.TLSClientConfig = c.TLS.Clone()
			if t.TLSClientConfig.InsecureSkipVerify && t.TLSClientConfig.VerifyPeerCertificate == nil && t.TLSClientConfig.VerifyConnection == nil {
				return nil, errors.New("http client: certificate verification required")
			}
		}
		tr = t
		out.owned = t
	}
	out.HTTP = &http.Client{Transport: tr, Timeout: c.Timeout}
	return out, nil
}

// Do returns a streaming response. The caller owns and must close its body.
func (c *Client) Do(r *http.Request) (*http.Response, error) { return c.do(r) }

// JSON bounds the response body and closes it. Non-2xx responses return a status error without exposing their body.
func (c *Client) JSON(ctx context.Context, method, url string, input, output any) error {
	var body io.Reader
	if input != nil {
		b, e := json.Marshal(input)
		if e != nil {
			return e
		}
		body = bytes.NewReader(b)
	}
	r, e := http.NewRequestWithContext(ctx, method, url, body)
	if e != nil {
		return e
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	res, e := c.Do(r)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return &StatusError{Status: res.StatusCode, RetryAfter: parseRetryAfter(res.Header.Get("Retry-After"), c.config.Clock.Now())}
	}
	b, e := io.ReadAll(io.LimitReader(res.Body, c.max+1))
	if e != nil {
		return e
	}
	if int64(len(b)) > c.max {
		return errors.New("http client: response limit exceeded")
	}
	if output == nil || len(b) == 0 {
		return nil
	}
	return json.Unmarshal(b, output)
}

// StatusError contains an HTTP failure status without its potentially sensitive response body.
type StatusError struct {
	Status     int
	RetryAfter time.Duration
}

// Error returns a diagnostic error description.
func (e *StatusError) Error() string { return "http client: " + http.StatusText(e.Status) }

// Close releases owned resources. See package documentation for shutdown ordering.
func (c *Client) Close() error {
	if c.owned != nil {
		c.owned.CloseIdleConnections()
	}
	return nil
}
