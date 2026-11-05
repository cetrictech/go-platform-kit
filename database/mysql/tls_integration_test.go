//go:build integration

package mysql

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMutualTLS(t *testing.T) {
	dir := os.Getenv("GPK_TLS_DIR")
	if dir == "" {
		t.Skip("GPK_TLS_DIR unset")
	}
	pem, err := os.ReadFile(filepath.Join(dir, "ca.crt"))
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		t.Fatal("invalid CA")
	}
	cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "client.crt"), filepath.Join(dir, "client.key"))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := Config{DSN: "sdk:sdk-test-only@tcp(127.0.0.1:23306)/sdk", TLS: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{cert}}}
	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var name, cipher string
	if err := c.QueryRow(ctx, "SHOW SESSION STATUS LIKE 'Ssl_cipher'").Scan(&name, &cipher); err != nil || cipher == "" {
		t.Fatal(cipher, err)
	}
	cfg.TLS = cfg.TLS.Clone()
	cfg.TLS.Certificates = nil
	if bad, err := New(ctx, cfg); err == nil {
		bad.Close()
		t.Fatal("accepted missing client certificate")
	}
}
