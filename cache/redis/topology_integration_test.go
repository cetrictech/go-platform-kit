//go:build integration

package redis

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	r "github.com/redis/go-redis/v9"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestClusterRoutingAcrossSlots(t *testing.T) {
	address := os.Getenv("REDIS_CLUSTER_ADDRESS")
	if address == "" {
		t.Skip("REDIS_CLUSTER_ADDRESS unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	c, err := New(ctx, Config{Mode: Cluster, AllowPlaintext: true, Addresses: []string{address}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	prefix := fmt.Sprintf("gpk-cluster-%d-", time.Now().UnixNano())
	for i := range 60 {
		key := fmt.Sprintf("%s%d", prefix, i)
		if err := c.Set(ctx, key, i, time.Minute); err != nil {
			t.Fatal(err)
		}
		if got, err := c.Get(ctx, key); err != nil || got != fmt.Sprint(i) {
			t.Fatal(got, err)
		}
	}
	native, ok := c.Native().(*r.ClusterClient)
	if !ok {
		t.Fatal("not cluster client")
	}
	var masters atomic.Int32
	if err := native.ForEachMaster(ctx, func(ctx context.Context, node *r.Client) error { masters.Add(1); return node.Ping(ctx).Err() }); err != nil {
		t.Fatal(err)
	}
	if masters.Load() != 3 {
		t.Fatalf("masters=%d", masters.Load())
	}
}
func TestSentinelFailover(t *testing.T) {
	address := os.Getenv("REDIS_SENTINEL_ADDRESS")
	if address == "" {
		t.Skip("REDIS_SENTINEL_ADDRESS unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()
	sentinel := r.NewSentinelClient(&r.Options{Addr: address})
	defer sentinel.Close()
	before, err := sentinel.GetMasterAddrByName(ctx, "sdk-master").Result()
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(ctx, Config{Mode: Sentinel, AllowPlaintext: true, MasterName: "sdk-master", Addresses: []string{address}})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	key := fmt.Sprintf("gpk-failover-%d", time.Now().UnixNano())
	if err := c.Set(ctx, key, "survives", time.Minute); err != nil {
		t.Fatal(err)
	}
	if n, err := c.Native().Do(ctx, "WAIT", 1, 5000).Int(); err != nil || n != 1 {
		t.Fatal(n, err)
	}
	if err := sentinel.Failover(ctx, "sdk-master").Err(); err != nil {
		t.Fatal(err)
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		after, e := sentinel.GetMasterAddrByName(ctx, "sdk-master").Result()
		if e == nil && fmt.Sprint(after) != fmt.Sprint(before) {
			if got, e := c.Get(ctx, key); e == nil && got == "survives" {
				break
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
	if err := c.Set(ctx, key, "recovered", time.Minute); err != nil {
		t.Fatal(err)
	}
}
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
	cfg := Config{Addresses: []string{"127.0.0.1:17479"}, TLS: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots, Certificates: []tls.Certificate{cert}}}
	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Set(ctx, "gpk-tls", "verified", time.Minute); err != nil {
		t.Fatal(err)
	}
	cfg.TLS = cfg.TLS.Clone()
	cfg.TLS.Certificates = nil
	cfg.DialTimeout = time.Second
	cfg.MaxRetries = -1
	if bad, err := New(ctx, cfg); err == nil {
		bad.Close()
		t.Fatal("accepted missing client certificate")
	}
}
