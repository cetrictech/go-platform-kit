package logger

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

type ownedBuffer struct {
	bytes.Buffer
	closes  atomic.Int32
	flushes atomic.Int32
}

func (w *ownedBuffer) Close() error { w.closes.Add(1); return nil }
func (w *ownedBuffer) Flush() error { w.flushes.Add(1); return nil }
func TestOutputOwnershipAndDerivedClose(t *testing.T) {
	w := new(ownedBuffer)
	l, err := Open(Config{Output: w, OwnOutput: true, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	child := l.With("service", "test")
	if err := child.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := child.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if w.closes.Load() != 1 || w.flushes.Load() != 1 {
		t.Fatal("output lifecycle was duplicated")
	}
	external := new(ownedBuffer)
	New(Config{Output: external}).Close()
	if external.closes.Load() != 0 {
		t.Fatal("closed caller-owned writer")
	}
}
func TestOpenFailureAndSharedLevel(t *testing.T) {
	if _, err := Open(Config{File: filepath.Join(t.TempDir(), "missing", "log")}); err == nil {
		t.Fatal("expected eager open failure")
	}
	var out bytes.Buffer
	l, err := Open(Config{Output: &out, JSON: true})
	if err != nil {
		t.Fatal(err)
	}
	child := l.With("child", true)
	l.SetLevel(slog.LevelError)
	child.Info("hidden")
	child.Error("visible")
	if bytes.Contains(out.Bytes(), []byte("hidden")) || !bytes.Contains(out.Bytes(), []byte("visible")) {
		t.Fatal(out.String())
	}
}
