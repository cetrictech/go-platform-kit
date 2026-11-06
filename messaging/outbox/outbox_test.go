package outbox

import (
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"strings"
	"testing"
)

func TestInsert(t *testing.T) {
	r, e := New(kafka.Message{Topic: "events", Value: []byte("test")})
	if e != nil {
		t.Fatal(e)
	}
	q, args, e := InsertSQL("postgres", r)
	if e != nil || len(args) != 4 || !strings.Contains(q, "$4") {
		t.Fatal(q, args, e)
	}
	if _, _, e = InsertSQL("bad", r); e == nil {
		t.Fatal("dialect")
	}
}
