package grpcx

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"google.golang.org/grpc/codes"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"net"
	"testing"
	"time"
)

func TestDefaultsAndHealth(t *testing.T) {
	if _, e := NewServer(ServerConfig{}); e == nil {
		t.Fatal("plaintext accepted")
	}
	if _, e := NewClient(ClientConfig{Target: "localhost:1"}); e == nil {
		t.Fatal("plaintext accepted")
	}
	s, e := NewServer(ServerConfig{AllowPlaintext: true})
	if e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go s.Serve(l)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	defer s.Shutdown(ctx)
	c, e := NewClient(ClientConfig{Target: l.Addr().String(), AllowPlaintext: true})
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	res, e := healthpb.NewHealthClient(c).Check(ctx, &healthpb.HealthCheckRequest{})
	if e != nil || res.Status != healthpb.HealthCheckResponse_SERVING {
		t.Fatal(res, e)
	}
	if status.Code(Status(errors.New("private"))) != codes.Internal {
		t.Fatal("classification")
	}
}

func TestCustomTLSVerifier(t *testing.T) {
	c, e := NewClient(ClientConfig{Target: "localhost:1", TLS: &tls.Config{InsecureSkipVerify: true, VerifyPeerCertificate: func([][]byte, [][]*x509.Certificate) error { return nil }}})
	if e != nil {
		t.Fatal("custom verifier rejected", e)
	}
	c.Close()
	if _, e := NewClient(ClientConfig{Target: "localhost:1", TLS: &tls.Config{InsecureSkipVerify: true}}); e == nil {
		t.Fatal("unverified TLS accepted")
	}
}
