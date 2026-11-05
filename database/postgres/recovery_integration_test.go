//go:build integration

package postgres

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestReplicaSelectionAndBackendRecovery(t *testing.T) {
	url := os.Getenv("POSTGRES_URL")
	if url == "" {
		t.Skip("POSTGRES_URL unset")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := Config{URL: url, AllowPlaintext: true, MaxConnections: 1}
	cluster, err := NewCluster(ctx, ClusterConfig{Primary: cfg, Replicas: []Config{cfg, cfg}})
	if err != nil {
		t.Fatal(err)
	}
	defer cluster.Close()
	strong, err := cluster.Reader(Strong)
	if err != nil || strong != cluster.Writer() {
		t.Fatal(err)
	}
	first, err := cluster.Reader(Eventual)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cluster.Reader(Eventual)
	if err != nil || first == second || first == strong {
		t.Fatal("replicas not selected", err)
	}
	var readonly string
	if err := first.QueryRow(ctx, "SHOW default_transaction_read_only").Scan(&readonly); err != nil || readonly != "on" {
		t.Fatal(readonly, err)
	}
	var pid int
	if err := strong.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid); err != nil {
		t.Fatal(err)
	}
	killer, err := New(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer killer.Close()
	var terminated bool
	if err := killer.QueryRow(ctx, "SELECT pg_terminate_backend($1)", pid).Scan(&terminated); err != nil || !terminated {
		t.Fatal(terminated, err)
	}
	// A killed idle connection may fail its next use; subsequent operations recover.
	var value int
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := strong.QueryRow(ctx, "SELECT 1").Scan(&value); err == nil && value == 1 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}
