package errors

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
)

func TestSafeAndWrap(t *testing.T) {
	e := Wrap(context.DeadlineExceeded, "safe", WithDebug("private"))
	if !Is(e, context.DeadlineExceeded) {
		t.Fatal("lost cause")
	}
	b, _ := json.Marshal(e)
	if strings.Contains(string(b), "private") {
		t.Fatal(string(b))
	}
	if Classify(context.DeadlineExceeded).Category != Timeout {
		t.Fatal("classification")
	}
}
func TestRecovery(t *testing.T) {
	var e error
	func() { defer Recover(&e, nil); panic("bad") }()
	if e == nil {
		t.Fatal("lost panic")
	}
}

func BenchmarkConstruction(b *testing.B) {
	for b.Loop() {
		benchmarkError = New("safe", WithCode("FAILURE"), WithCategory(Database))
	}
}

func TestNoRows(t *testing.T) {
	if e := Classify(sql.ErrNoRows); e.HTTPStatus != 404 || e.Category != NotFound {
		t.Fatal(e)
	}
}

var benchmarkError *AppError
