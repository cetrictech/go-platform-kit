package logger

import (
	"bytes"
	"context"
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
