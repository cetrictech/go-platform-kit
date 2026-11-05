package pagination

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCursor(t *testing.T) {
	now := time.Unix(100, 0)
	c := Codec{ActiveKey: "a", Keys: map[string][]byte{"a": bytes.Repeat([]byte{1}, 32)}, TTL: time.Minute, Now: func() time.Time { return now }}
	token, e := c.EncodeCursor(Cursor{Direction: "next", Query: "users", Keys: []json.RawMessage{json.RawMessage(`42`)}})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = c.DecodeCursor(token, "users", ""); e != nil {
		t.Fatal(e)
	}
	if _, e = c.DecodeCursor(token, "other", ""); e == nil {
		t.Fatal("binding")
	}
	p := strings.Split(token, ".")
	p[0] = "e30"
	if _, e = c.DecodeCursor(strings.Join(p, "."), "users", ""); e == nil {
		t.Fatal("tamper")
	}
	now = now.Add(time.Hour)
	if _, e = c.DecodeCursor(token, "users", ""); e == nil {
		t.Fatal("expiration")
	}
}
func TestOffset(t *testing.T) {
	if (Offset{0, 20}).Validate() == nil {
		t.Fatal("zero page")
	}
	m, e := NewOffsetMeta(Offset{1, 20}, 21)
	if e != nil || !m.HasNext || m.Pages != 2 {
		t.Fatal(m, e)
	}
}
func BenchmarkCursor(b *testing.B) {
	c := Codec{ActiveKey: "a", Keys: map[string][]byte{"a": bytes.Repeat([]byte{1}, 32)}}
	v := Cursor{Direction: "next", Query: "users", Keys: []json.RawMessage{json.RawMessage(`42`)}}
	for b.Loop() {
		_, _ = c.EncodeCursor(v)
	}
}
