package redis

import (
	"context"
	"strings"
	"testing"
)

func TestPoolRejectsImpossibleIdleConfiguration(t *testing.T) {
	client, err := New(context.Background(), Config{Addresses: []string{"127.0.0.1:1"}, AllowPlaintext: true, PoolSize: 2, MinIdleConnections: 3})
	if client != nil {
		client.Close()
		t.Fatal("accepted impossible idle bound")
	}
	if err == nil || !strings.Contains(err.Error(), "idle connections exceed") {
		t.Fatalf("validation did not reject before connecting: %v", err)
	}
}
