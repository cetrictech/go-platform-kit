// Package spiffe integrates rotating X.509 identities from the SPIFFE Workload API.
package spiffe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"github.com/spiffe/go-spiffe/v2/svid/x509svid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
)

const defaultSocket = "unix:///run/spire/agent/public/api.sock"

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Socket, TrustDomain, ExpectedID string
	AllowedPeers                    []string
	Authorize                       func(string) bool
	AllowTrustDomain                bool
	Timeout                         time.Duration
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
	domain, expected, socket, policy, e := c.policy()
	if e != nil {
		return nil, e
	}
	if e = checkSocket(socket); e != nil {
		return nil, e
	}
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	opts := []workloadapi.X509SourceOption{}
	if socket != "" {
		opts = append(opts, workloadapi.WithClientOptions(workloadapi.WithAddr(socket)))
	}
	if expected != (spiffeid.ID{}) {
		want := expected
		opts = append(opts, workloadapi.WithDefaultX509SVIDPicker(func(svids []*x509svid.SVID) *x509svid.SVID {
			for _, svid := range svids {
				if svid.ID == want {
					return svid
				}
			}
			return nil
		}))
	}
	s, e := workloadapi.NewX509Source(ctx, opts...)
	if e != nil {
		if errors.Is(e, context.DeadlineExceeded) {
			return nil, fmt.Errorf("spiffe: timed out waiting for %s on %s: %w", c.ExpectedID, socket, e)
		}
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
	return &Source{source: s, authorize: policy, domain: domain, expected: c.ExpectedID}, nil
}

func (c Config) policy() (spiffeid.TrustDomain, spiffeid.ID, string, tlsconfig.Authorizer, error) {
	domain, e := spiffeid.TrustDomainFromString(c.TrustDomain)
	if e != nil {
		return spiffeid.TrustDomain{}, spiffeid.ID{}, "", nil, e
	}
	if len(c.AllowedPeers) == 0 && c.Authorize == nil && !c.AllowTrustDomain {
		return domain, spiffeid.ID{}, "", nil, errors.New("spiffe: peer authorization policy required")
	}
	allowed := map[spiffeid.ID]bool{}
	for _, raw := range c.AllowedPeers {
		id, e := spiffeid.FromString(raw)
		if e != nil {
			return domain, spiffeid.ID{}, "", nil, e
		}
		if id.TrustDomain() != domain {
			return domain, spiffeid.ID{}, "", nil, errors.New("spiffe: peer outside configured trust domain")
		}
		allowed[id] = true
	}
	var expected spiffeid.ID
	if c.ExpectedID != "" {
		expected, e = spiffeid.FromString(c.ExpectedID)
		if e != nil || expected.TrustDomain() != domain {
			return domain, spiffeid.ID{}, "", nil, errors.New("spiffe: expected identity must belong to the trust domain")
		}
	}
	socket := strings.TrimSpace(c.Socket)
	if strings.HasPrefix(socket, "/") {
		socket = "unix://" + socket
	}
	if socket != "" {
		if e = validateSocketURL(socket); e != nil {
			return domain, expected, "", nil, e
		}
	}
	policy := tlsconfig.Authorizer(func(id spiffeid.ID, _ [][]*x509.Certificate) error {
		if id.TrustDomain() != domain {
			return errors.New("spiffe: peer trust domain rejected")
		}
		if len(allowed) == 0 && c.AllowTrustDomain {
			return nil
		}
		if allowed[id] || c.Authorize != nil && c.Authorize(id.String()) {
			return nil
		}
		return errors.New("spiffe: peer identity rejected")
	})
	return domain, expected, socket, policy, nil
}

func validateSocketURL(socket string) error {
	u, e := url.Parse(socket)
	if e != nil || u.Scheme != "unix" || u.Host != "" || u.Path == "" || u.Path[0] != '/' || u.RawQuery != "" || u.Fragment != "" {
		return errors.New("spiffe: socket must be an absolute unix:/// URL")
	}
	return nil
}

func checkSocket(socket string) error {
	if socket == "" {
		return nil
	}
	path := strings.TrimPrefix(socket, "unix://")
	info, e := os.Stat(path)
	if e != nil {
		return fmt.Errorf("spiffe: socket %s: %w", path, e)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("spiffe: socket %s is not a unix socket", path)
	}
	return nil
}

// ClientTLS builds a rotating, authorized mutual TLS client configuration.
func (s *Source) ClientTLS() *tls.Config {
	config := tlsconfig.MTLSClientConfig(s, s.source, s.authorize)
	config.MinVersion = tls.VersionTLS13
	return config
}

// ServerTLS builds a rotating, authorized mutual TLS server configuration.
func (s *Source) ServerTLS() *tls.Config {
	config := tlsconfig.MTLSServerConfig(s, s.source, s.authorize)
	config.MinVersion = tls.VersionTLS13
	return config
}

// Identity returns the currently validated workload SPIFFE ID.
func (s *Source) Identity() (string, error) {
	v, e := s.GetX509SVID()
	if e != nil {
		return "", e
	}
	return v.ID.String(), nil
}

// Allows reports whether the peer SPIFFE ID satisfies the configured authorization policy.
func (s *Source) Allows(peer string) bool {
	id, e := spiffeid.FromString(peer)
	if e != nil {
		return false
	}
	return s.authorize(id, nil) == nil
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
