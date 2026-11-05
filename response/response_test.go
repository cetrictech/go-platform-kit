package response

import (
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAtomic(t *testing.T) {
	r := httptest.NewRecorder()
	if e := JSON(r, 200, make(chan int)); e == nil || r.Body.Len() != 0 {
		t.Fatal("encoding committed")
	}
	if e := JSON(r, 200, strings.Repeat("x", 100), Options{MaxBytes: 10}); e == nil || r.Body.Len() != 0 {
		t.Fatal("limit committed")
	}
	w := &Writer{ResponseWriter: r}
	if e := NoContent(w); e != nil || r.Body.Len() != 0 {
		t.Fatal(e)
	}
	if !errors.Is(OK(w, nil), ErrWritten) {
		t.Fatal("double write")
	}
}
func TestDebugHidden(t *testing.T) {
	w := httptest.NewRecorder()
	e := app.New("safe", app.WithDebug("private"))
	if err := Error(w, nil, e); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(w.Body.String(), "private") {
		t.Fatal("debug leaked")
	}
}

func BenchmarkSerialization(b *testing.B) {
	v := Envelope{Success: true, Code: "SUCCESS", Data: map[string]int{"count": 42}}
	for b.Loop() {
		_ = JSON(httptest.NewRecorder(), 200, v)
	}
}

func TestDebugAuthorization(t *testing.T) {
	e := app.New("safe", app.WithDebug("private"))
	for _, allowed := range []bool{false, true} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		err := Error(w, r, e, Options{Debug: AuthorizedInternal, AuthorizeDebug: func(*http.Request) bool { return allowed }, SanitizeDebug: func(string) string { return "sanitized" }})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "sanitized") != allowed {
			t.Fatal(w.Body.String())
		}
	}
}

func TestRetryAfterDoesNotOverflow(t *testing.T) {
	w := httptest.NewRecorder()
	e := app.New("wait", app.WithRetryAfter(time.Duration(1<<63-1)))
	if err := Error(w, nil, e); err != nil {
		t.Fatal(err)
	}
	if w.Header().Get("Retry-After") != "9223372037" {
		t.Fatal(w.Header())
	}
}
