package mysql

import (
	"context"
	"strings"
	"testing"
)

func TestPoolRejectsImpossibleIdleConfiguration(t *testing.T) {
	for _, configuration := range []Config{
		{DSN: "test:test@tcp(127.0.0.1:1)/test", AllowPlaintext: true, MaxIdleConnections: 33},
		{DSN: "test:test@tcp(127.0.0.1:1)/test", AllowPlaintext: true, MaxOpenConnections: 2, MaxIdleConnections: 3},
	} {
		client, err := New(context.Background(), configuration)
		if client != nil {
			client.Close()
			t.Fatal("accepted impossible idle bound")
		}
		if err == nil || !strings.Contains(err.Error(), "idle connections exceed") {
			t.Fatalf("validation did not reject before connecting: %v", err)
		}
	}
}
