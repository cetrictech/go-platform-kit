package logger

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileRotationCompressionAndRetention(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "service.log")
	old := filepath.Join(dir, "service-2000-01-01T00-00-00.000.log")
	if err := os.WriteFile(old, []byte("expired"), 0600); err != nil {
		t.Fatal(err)
	}
	l, err := Open(Config{JSON: true, File: file, MaxSizeMB: 1, MaxBackups: 1, MaxAgeDays: 1, Compress: true})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	payload := strings.Repeat("x", 8192)
	for range 300 {
		l.Info("rotation test", "payload", payload, "password", "must-not-appear")
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var archives []string
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".gz") {
				archives = append(archives, filepath.Join(dir, entry.Name()))
			}
		}
		_, oldErr := os.Stat(old)
		if len(archives) == 1 && os.IsNotExist(oldErr) {
			f, err := os.Open(archives[0])
			if err != nil {
				t.Fatal(err)
			}
			reader, err := gzip.NewReader(f)
			if err != nil {
				f.Close()
				t.Fatal(err)
			}
			content, readErr := io.ReadAll(reader)
			reader.Close()
			f.Close()
			// Compression runs asynchronously; a not-yet-complete archive may be retried.
			if readErr == nil {
				if strings.Contains(string(content), "must-not-appear") || !strings.Contains(string(content), "[REDACTED]") {
					t.Fatal("rotation lost redaction")
				}
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("rotation/compression/retention did not complete")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
