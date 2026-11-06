package grpcx

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	pb "google.golang.org/grpc/health/grpc_health_v1"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestMutualTLS(t *testing.T) {
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &key.PublicKey, key)
	if e != nil {
		t.Fatal(e)
	}
	ca, e = x509.ParseCertificate(der)
	if e != nil {
		t.Fatal(e)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca)
	issue := func(serial int64) tls.Certificate {
		k, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			t.Fatal(e)
		}
		leaf := &x509.Certificate{SerialNumber: big.NewInt(serial), NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}}
		b, e := x509.CreateCertificate(rand.Reader, leaf, ca, &k.PublicKey, key)
		if e != nil {
			t.Fatal(e)
		}
		return tls.Certificate{Certificate: [][]byte{b}, PrivateKey: k}
	}
	server, e := NewServer(ServerConfig{TLS: &tls.Config{Certificates: []tls.Certificate{issue(2)}, ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: pool}, MaxConnections: 2})
	if e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go server.Serve(l)
	defer server.GRPC.Stop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, e := NewClient(ClientConfig{Target: l.Addr().String(), TLS: &tls.Config{RootCAs: pool, Certificates: []tls.Certificate{issue(3)}}})
	if e != nil {
		t.Fatal(e)
	}
	defer client.Close()
	if _, e = pb.NewHealthClient(client).Check(ctx, &pb.HealthCheckRequest{}); e != nil {
		t.Fatal(e)
	}
	untrusted, e := NewClient(ClientConfig{Target: l.Addr().String(), TLS: &tls.Config{RootCAs: pool}})
	if e != nil {
		t.Fatal(e)
	}
	defer untrusted.Close()
	if _, e = pb.NewHealthClient(untrusted).Check(ctx, &pb.HealthCheckRequest{}); e == nil {
		t.Fatal("missing client identity accepted")
	}
}
