//go:build integration

package integration

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"github.com/cetrictech/go-platform-kit/grpcx"
	"github.com/cetrictech/go-platform-kit/spiffe"
	pb "github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/peer"
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
func TestSDKWorkloadGRPCRotation(t *testing.T) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, e := spiffe.New(ctx, spiffe.Config{Socket: "unix://" + sock, TrustDomain: "test.internal", ExpectedID: "spiffe://test.internal/service", AllowedPeers: []string{"spiffe://test.internal/service"}})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	server, e := grpcx.NewServer(grpcx.ServerConfig{TLS: s.ServerTLS()})
	if e != nil {
		t.Fatal(e)
	}
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go server.Serve(listener)
	defer server.GRPC.Stop()
	check := func(want int64) {
		client, e := grpcx.NewClient(grpcx.ClientConfig{Target: listener.Addr().String(), TLS: s.ClientTLS()})
		if e != nil {
			t.Fatal(e)
		}
		defer client.Close()
		var p peer.Peer
		if _, e := healthpb.NewHealthClient(client).Check(ctx, &healthpb.HealthCheckRequest{}, grpc.Peer(&p)); e != nil {
			t.Fatal(e)
		}
		info, ok := p.AuthInfo.(credentials.TLSInfo)
		if !ok || len(info.State.PeerCertificates) == 0 {
			t.Fatal("missing authenticated peer")
		}
		if got := info.State.PeerCertificates[0].SerialNumber.Int64(); got != want {
			t.Fatalf("serial=%d want=%d", got, want)
		}
	}
	check(2)
	w.rotate <- makeSVID(3)
	for {
		v, e := s.GetX509SVID()
		if e != nil {
			t.Fatal(e)
		}
		if v.Certificates[0].SerialNumber.Int64() == 3 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Millisecond):
		}
	}
	check(3)
	rejected, e := spiffe.New(ctx, spiffe.Config{Socket: "unix://" + sock, TrustDomain: "test.internal", AllowedPeers: []string{"spiffe://test.internal/other"}})
	if e != nil {
		t.Fatal(e)
	}
	defer rejected.Close()
	bad, e := grpcx.NewClient(grpcx.ClientConfig{Target: listener.Addr().String(), TLS: rejected.ClientTLS()})
	if e != nil {
		t.Fatal(e)
	}
	defer bad.Close()
	short, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	if _, e := healthpb.NewHealthClient(bad).Check(short, &healthpb.HealthCheckRequest{}); e == nil {
		t.Fatal("rejected SPIFFE peer accepted")
	}
}
