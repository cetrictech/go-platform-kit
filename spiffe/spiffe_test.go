package spiffe

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	pb "github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"github.com/spiffe/go-spiffe/v2/spiffeid"
	"github.com/spiffe/go-spiffe/v2/spiffetls/tlsconfig"
	"google.golang.org/grpc"
	"math/big"
	"net"
	"net/url"
	"path/filepath"
	"testing"
	"time"
)

type workload struct {
	pb.UnimplementedSpiffeWorkloadAPIServer
	initial *pb.X509SVIDResponse
	rotate  chan *pb.X509SVIDResponse
}

func (w *workload) FetchX509SVID(_ *pb.X509SVIDRequest, s grpc.ServerStreamingServer[pb.X509SVIDResponse]) error {
	if e := s.Send(w.initial); e != nil {
		return e
	}
	for {
		select {
		case <-s.Context().Done():
			return s.Context().Err()
		case r := <-w.rotate:
			if e := s.Send(r); e != nil {
				return e
			}
		}
	}
}
func TestWorkloadTLSAndRotation(t *testing.T) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	ca, e = x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	makeSVID := func(serial int64) *pb.X509SVIDResponse {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		uri, _ := url.Parse("spiffe://test.internal/service")
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), URIs: []*url.URL{uri}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
		cert, e := x509.CreateCertificate(rand.Reader, leaf, ca, &k.PublicKey, key)
		if e != nil {
			t.Fatal(e)
		}
		pk, e := x509.MarshalPKCS8PrivateKey(k)
		if e != nil {
			t.Fatal(e)
		}
		return &pb.X509SVIDResponse{Svids: []*pb.X509SVID{{SpiffeId: uri.String(), X509Svid: cert, X509SvidKey: pk, Bundle: der}}}
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	l, e := net.Listen("unix", sock)
	if e != nil {
		t.Fatal(e)
	}
	g := grpc.NewServer()
	w := &workload{initial: makeSVID(2), rotate: make(chan *pb.X509SVIDResponse, 1)}
	pb.RegisterSpiffeWorkloadAPIServer(g, w)
	go g.Serve(l)
	defer g.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, e := New(ctx, Config{Socket: "unix://" + sock, TrustDomain: "test.internal", ExpectedID: "spiffe://test.internal/service", AllowedPeers: []string{"spiffe://test.internal/service"}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	listener, e := tls.Listen("tcp", "127.0.0.1:0", s.ServerTLS())
	if e != nil {
		t.Fatal(e)
	}
	defer listener.Close()
	serverDone := make(chan error, 1)
	go func() {
		conn, e := listener.Accept()
		if e != nil {
			serverDone <- e
			return
		}
		defer conn.Close()
		serverDone <- conn.(*tls.Conn).HandshakeContext(ctx)
	}()
	conn, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", listener.Addr().String(), s.ClientTLS())
	if e != nil {
		t.Fatal(e)
	}
	conn.Close()
	if e = <-serverDone; e != nil {
		t.Fatal(e)
	}
	w.rotate <- makeSVID(3)
	for {
		v, e := s.source.GetX509SVID()
		if e != nil {
			t.Fatal(e)
		}
		if v.Certificates[0].SerialNumber.Int64() == 3 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("rotation timeout")
		case <-time.After(time.Millisecond):
		}
	}
	// An explicit mismatched allowlist rejects the authenticated peer.
	rejected := tlsconfig.MTLSClientConfig(s, s.source, tlsconfig.AuthorizeID(spiffeid.RequireFromString("spiffe://test.internal/other")))
	go func() {
		c, e := listener.Accept()
		if e == nil {
			defer c.Close()
			_ = c.(*tls.Conn).HandshakeContext(ctx)
		}
	}()
	if c, e := tls.DialWithDialer(&net.Dialer{Timeout: time.Second}, "tcp", listener.Addr().String(), rejected); e == nil {
		c.Close()
		t.Fatal("rejected policy succeeded")
	}
}
func TestRejectPolicy(t *testing.T) {
	if _, e := New(context.Background(), Config{TrustDomain: "test.internal"}); e == nil {
		t.Fatal("missing authorization accepted")
	}
	if _, e := Parse("http://example.org"); e == nil {
		t.Fatal("invalid SPIFFE ID")
	}
}

func TestMissingSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sock")
	_, e := New(context.Background(), Config{Socket: "unix://" + path, TrustDomain: "test.internal", ExpectedID: "spiffe://test.internal/service", AllowTrustDomain: true, Timeout: 50 * time.Millisecond})
	if e == nil {
		t.Fatal("missing socket accepted")
	}
}

func TestFromEnv(t *testing.T) {
	t.Setenv("GRPC_SECURITY_MODE", "")
	if _, enabled, e := FromEnv(); e != nil || enabled {
		t.Fatalf("disabled: %v %v", enabled, e)
	}
	t.Setenv("GRPC_SECURITY_MODE", "spire")
	t.Setenv("SPIFFE_TRUST_DOMAIN", "darena.internal")
	t.Setenv("SPIFFE_ID", "spiffe://darena.internal/prod/cl_identity")
	t.Setenv("APP_SPIFFE_ALLOWED_IDS", "spiffe://darena.internal/prod/cl_authorization")
	c, enabled, e := FromEnv()
	if e != nil || !enabled {
		t.Fatal(e)
	}
	if c.Socket != defaultSocket || c.ExpectedID != "spiffe://darena.internal/prod/cl_identity" || c.AllowTrustDomain || len(c.AllowedPeers) != 1 {
		t.Fatalf("%+v", c)
	}
	t.Setenv("APP_SPIFFE_ALLOWED_IDS", "")
	c, enabled, e = FromEnv()
	if e != nil || !enabled || !c.AllowTrustDomain || len(c.AllowedPeers) != 0 {
		t.Fatalf("%+v %v %v", c, enabled, e)
	}
	t.Setenv("GRPC_SECURITY_MODE", "mtls")
	if _, _, e = FromEnv(); e == nil {
		t.Fatal("invalid mode accepted")
	}
}

func TestTrustDomainPeers(t *testing.T) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	ca, e = x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	uri, _ := url.Parse("spiffe://test.internal/service")
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), URIs: []*url.URL{uri}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	cert, e := x509.CreateCertificate(rand.Reader, leaf, ca, &k.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	pk, e := x509.MarshalPKCS8PrivateKey(k)
	if e != nil {
		t.Fatal(e)
	}
	sock := filepath.Join(t.TempDir(), "agent.sock")
	l, e := net.Listen("unix", sock)
	if e != nil {
		t.Fatal(e)
	}
	g := grpc.NewServer()
	pb.RegisterSpiffeWorkloadAPIServer(g, &workload{initial: &pb.X509SVIDResponse{Svids: []*pb.X509SVID{{SpiffeId: uri.String(), X509Svid: cert, X509SvidKey: pk, Bundle: der}}}, rotate: make(chan *pb.X509SVIDResponse)})
	go g.Serve(l)
	defer g.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s, e := New(ctx, Config{Socket: "unix://" + sock, TrustDomain: "test.internal", ExpectedID: "spiffe://test.internal/service", AllowTrustDomain: true})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if !s.Allows("spiffe://test.internal/other") || s.Allows("spiffe://other.internal/service") {
		t.Fatal("trust-domain policy")
	}
	if id, e := s.Identity(); e != nil || id != "spiffe://test.internal/service" {
		t.Fatal(id, e)
	}
}
