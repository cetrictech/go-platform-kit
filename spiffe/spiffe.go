// Package spiffe integrates rotating X.509 identities from the SPIFFE Workload API.
package spiffe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
	"strings"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Socket, TrustDomain, ExpectedID string
	AllowedPeers                    []string
	Authorize                       func(string) bool
}

// Source owns a rotating SPIFFE Workload API identity and peer authorization policy.
type Source struct {
	source    *workloadapi.X509Source
	authorize tlsconfig.Authorizer
	domain    spiffeid.TrustDomain
	expected  string
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(ctx context.Context, c Config) (*Source, error) {
	domain, e := spiffeid.TrustDomainFromString(c.TrustDomain)
	if e != nil {
		return nil, e
	}
	if len(c.AllowedPeers) == 0 && c.Authorize == nil {
		return nil, errors.New("spiffe: peer authorization policy required")
	}
	allowed := map[spiffeid.ID]bool{}
	for _, raw := range c.AllowedPeers {
		id, e := spiffeid.FromString(raw)
		if e != nil {
			return nil, e
		}
		if id.TrustDomain() != domain {
			return nil, errors.New("spiffe: peer outside configured trust domain")
		}
		allowed[id] = true
	}
	opts := []workloadapi.X509SourceOption{}
	if strings.HasPrefix(c.Socket, "/") {
		c.Socket = "unix://" + c.Socket
	}
	if c.Socket != "" {
		opts = append(opts, workloadapi.WithClientOptions(workloadapi.WithAddr(c.Socket)))
	}
	s, e := workloadapi.NewX509Source(ctx, opts...)
	if e != nil {
		return nil, e
	}
	id, e := s.GetX509SVID()
	if e != nil {
		s.Close()
		return nil, e
	}
	if id.ID.TrustDomain() != domain || c.ExpectedID != "" && id.ID.String() != c.ExpectedID {
		s.Close()
		return nil, errors.New("spiffe: workload identity mismatch")
	}
	policy := tlsconfig.Authorizer(func(id spiffeid.ID, _ [][]*x509.Certificate) error {
		if id.TrustDomain() != domain {
			return errors.New("spiffe: peer trust domain rejected")
		}
		if allowed[id] || c.Authorize != nil && c.Authorize(id.String()) {
			return nil
		}
		return errors.New("spiffe: peer identity rejected")
	})
	return &Source{source: s, authorize: policy, domain: domain, expected: c.ExpectedID}, nil
}

// ClientTLS builds a rotating, authorized mutual TLS client configuration.
func (s *Source) ClientTLS() *tls.Config {
	return tlsconfig.MTLSClientConfig(s, s.source, s.authorize)
}

// ServerTLS builds a rotating, authorized mutual TLS server configuration.
func (s *Source) ServerTLS() *tls.Config {
	return tlsconfig.MTLSServerConfig(s, s.source, s.authorize)
}

// Identity returns the currently validated workload SPIFFE ID.
func (s *Source) Identity() (string, error) {
	v, e := s.GetX509SVID()
	if e != nil {
		return "", e
	}
	return v.ID.String(), nil
}

// Close releases owned resources. See package documentation for shutdown ordering.
func (s *Source) Close() error { return s.source.Close() }

// Parse parses and validates an identifier.
func Parse(raw string) (string, error) { id, e := spiffeid.FromString(raw); return id.String(), e }

// GetX509SVID supplies the current identity to TLS while rechecking configured identity constraints after rotation.
func (s *Source) GetX509SVID() (*x509svid.SVID, error) {
	v, e := s.source.GetX509SVID()
	if e != nil {
		return nil, e
	}
	if v.ID.TrustDomain() != s.domain || s.expected != "" && v.ID.String() != s.expected {
		return nil, errors.New("spiffe: rotated workload identity mismatch")
	}
	return v, nil
}
