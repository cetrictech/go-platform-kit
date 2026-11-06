package encodingx

import "testing"

func TestLimits(t *testing.T) {
	j := JSON{MaxBytes: 5}
	if _, e := j.Encode("abcdef"); e == nil {
		t.Fatal("size")
	}
	var s string
	if e := j.Decode([]byte(`"ok"`), &s); e != nil || s != "ok" {
		t.Fatal(s, e)
	}
	if j.Decode([]byte(`broken`), &s) == nil {
		t.Fatal("invalid")
	}
}
func BenchmarkJSON(b *testing.B) {
	j := JSON{}
	v := map[string]any{"key": "value"}
	for b.Loop() {
		_, _ = j.Encode(v)
	}
}
