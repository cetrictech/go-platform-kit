package pagination

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPageSizeAndOffsetLimits(t *testing.T) {
	r := httptest.NewRequest("GET", "/?page=3&page_size=10", nil)
	p, e := ParseOffset(r, OffsetConfig{DefaultLimit: 10, MaxLimit: 20, MaxOffset: 20})
	if e != nil || p.Skip() != 20 {
		t.Fatal(p, e)
	}
	if _, e = ParseOffset(httptest.NewRequest("GET", "/?page=4&page_size=10", nil), OffsetConfig{DefaultLimit: 10, MaxLimit: 20, MaxOffset: 20}); e == nil {
		t.Fatal("offset cap")
	}
	if _, e = ParseOffset(httptest.NewRequest("GET", "/?page_size=10&limit=11", nil)); e == nil {
		t.Fatal("ambiguous parameters")
	}
	m, e := NewOffsetMeta(Offset{Page: 1, Limit: 10}, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(m)
	for _, key := range []string{`"type":"offset"`, `"page_size":10`, `"total_records":0`, `"total_pages":0`} {
		if !bytes.Contains(b, []byte(key)) {
			t.Fatal(string(b))
		}
	}
}
func TestKeyRingRotationAndConcurrentSigning(t *testing.T) {
	keys := map[string][]byte{"a": bytes.Repeat([]byte{1}, 32), "b": bytes.Repeat([]byte{2}, 32)}
	ring, e := NewKeyRing("a", keys)
	if e != nil {
		t.Fatal(e)
	}
	codec := Codec{Provider: ring, TTL: time.Minute}
	value := Cursor{Direction: "next", Query: "query", Keys: []json.RawMessage{json.RawMessage(`9007199254740993`)}}
	old, e := codec.EncodeCursor(value)
	if e != nil {
		t.Fatal(e)
	}
	if e = ring.Rotate("b", keys); e != nil {
		t.Fatal(e)
	}
	if _, e = codec.DecodeCursor(old, "query", ""); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 30; i++ {
				if e := ring.Rotate([]string{"a", "b"}[i%2], keys); e != nil {
					t.Error(e)
				}
				token, e := codec.EncodeCursor(value)
				if e != nil {
					t.Error(e)
					return
				}
				if _, e = codec.DecodeCursor(token, "query", ""); e != nil {
					t.Error(e)
				}
			}
		}()
	}
	wg.Wait()
	if e = ring.Rotate("b", map[string][]byte{"b": keys["b"]}); e != nil {
		t.Fatal(e)
	}
	if _, e = codec.DecodeCursor(old, "query", ""); e == nil {
		t.Fatal("retired key accepted")
	}
}
func TestCursorKeyTypesAndNoQueryOnTamper(t *testing.T) {
	raw := []json.RawMessage{json.RawMessage(`9007199254740993`), json.RawMessage(`"2026-11-05T00:00:00Z"`)}
	values, e := decodeKeys(raw, []KeyType{KeyInt64, KeyTime})
	if e != nil || values[0] != int64(9007199254740993) {
		t.Fatal(values, e)
	}
	if _, e = decodeKeys([]json.RawMessage{json.RawMessage(`"bad"`)}, []KeyType{KeyInt64}); e == nil {
		t.Fatal("bad type")
	}
	q := Query{Dialect: "postgres", Template: "SELECT id WHERE {{where}} ORDER BY {{order}} LIMIT {{limit}}", Identity: "query", Columns: map[string]string{"id": "id"}, Sort: []Sort{{Name: "id", Unique: true}}, KeyTypes: []KeyType{KeyInt64}}
	_, e = FetchCursor[int64](context.Background(), func(context.Context, any, string, ...any) error { t.Fatal("invalid cursor reached SQL"); return nil }, q, CursorRequest{Cursor: "invalid", Limit: 10}, Codec{}, func(v int64) ([]any, error) { return []any{v}, nil }, nil)
	if e == nil {
		t.Fatal("tamper accepted")
	}
}
