// Command fixtures creates ephemeral test certificates. It is test infrastructure, not a published SDK tool.
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

func main() {
	directory := flag.String("dir", "", "dedicated generated fixture directory")
	flag.Parse()
	if *directory == "" {
		fail(fmt.Errorf("-dir required"))
	}
	if e := os.MkdirAll(*directory, 0755); e != nil {
		fail(e)
	}
	if e := os.Chmod(*directory, 0755); e != nil {
		fail(e)
	}
	caKey, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		fail(e)
	}
	now := time.Now()
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "go-platform-kit ephemeral test CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign}
	der, e := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if e != nil {
		fail(e)
	}
	write(*directory, "ca.crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	for i, name := range []string{"server", "client"} {
		key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if e != nil {
			fail(e)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: "sdk-test-" + name}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost", "redpanda-secure", "postgres-secure", "mysql-secure", "redis-secure"}}
		der, e := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
		if e != nil {
			fail(e)
		}
		pk, e := x509.MarshalPKCS8PrivateKey(key)
		if e != nil {
			fail(e)
		}
		write(*directory, name+".crt", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
		write(*directory, name+".key", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}))
	}
}

// Test keys are deliberately readable by container service UIDs through a read-only bind mount. Never use fixture keys outside isolated tests.
func write(dir, name string, data []byte) {
	if e := os.WriteFile(filepath.Join(dir, name), data, 0644); e != nil {
		fail(e)
	}
}
func fail(e error) { fmt.Fprintln(os.Stderr, e); os.Exit(1) }
