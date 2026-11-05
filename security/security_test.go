package security

import (
	"bytes"
	"testing"
)

func TestCrypto(t *testing.T) {
	k := bytes.Repeat([]byte{1}, 32)
	s, e := Sign(k, []byte("data"))
	if e != nil || !Verify(k, []byte("data"), s) || Verify(k, []byte("other"), s) {
		t.Fatal("HMAC")
	}
	if _, e = RandomBytes(0); e == nil {
		t.Fatal("invalid size")
	}
	h, e := HashPassword([]byte("correct horse"), 10)
	if e != nil || CheckPassword(h, []byte("correct horse")) != nil || CheckPassword(h, []byte("wrong")) == nil {
		t.Fatal("password")
	}
}
