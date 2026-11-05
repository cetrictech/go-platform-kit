package logger

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestRedaction(t *testing.T) {
	var b bytes.Buffer
	l := New(Config{JSON: true, Output: &b})
	l.InfoContext(context.Background(), "test", "password", "hidden", "nested", map[string]any{"token": "hidden"})
	if strings.Contains(b.String(), "hidden") {
		t.Fatal(b.String())
	}
	l.SetLevel(slog.LevelError)
	n := b.Len()
	l.Info("ignored")
	if b.Len() != n {
		t.Fatal("level ignored")
	}
}
func BenchmarkJSON(b *testing.B) {
	l := New(Config{JSON: true, Output: &discard{}})
	for b.Loop() {
		l.Info("event", "count", 42)
	}
}

type discard struct{}

func (*discard) Write(p []byte) (int, error) { return len(p), nil }

func TestSecretTaggedStructuredValue(t *testing.T) {
	var b bytes.Buffer
	l := New(Config{JSON: true, Output: &b})
	value := struct {
		Material string `secret:"true"`
		Counter  int64
	}{"sensitive", 9007199254740993}
	l.Info("safe", "object", value)
	if strings.Contains(b.String(), "sensitive") || !strings.Contains(b.String(), "9007199254740993") {
		t.Fatal(b.String())
	}
}

func TestDiagnosticCredentialRedaction(t *testing.T) {
	var b bytes.Buffer
	l := New(Config{JSON: true, Output: &b})
	l.Info("password=hidden authorization: Bearer hidden https://user:hidden@host/path?api_key=hidden")
	l.WithError(fmt.Errorf("connection postgres://user:hidden@localhost/db failed")).Error("failure")
	if strings.Contains(b.String(), "hidden") {
		t.Fatal(b.String())
	}
}
