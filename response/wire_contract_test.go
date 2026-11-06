package response_test

import (
	"encoding/json"
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/response"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestResponseWireContract(t *testing.T) {
	helpers := map[string]func(http.ResponseWriter, string) error{
		"bad_request": response.BadRequest, "unauthorized": response.Unauthorized, "forbidden": response.Forbidden, "not_found": response.NotFound, "method_not_allowed": response.MethodNotAllowed, "conflict": response.Conflict, "gone": response.Gone, "unprocessable": response.UnprocessableEntity, "rate_limit": response.TooManyRequests, "internal": response.InternalServerError, "bad_gateway": response.BadGateway, "unavailable": response.ServiceUnavailable, "gateway_timeout": response.GatewayTimeout,
	}
	calls := map[string]func(http.ResponseWriter) error{
		"ok":         func(w http.ResponseWriter) error { return response.OK(w, map[string]any{"id": "record-1"}) },
		"created":    func(w http.ResponseWriter) error { return response.Created(w, map[string]any{"id": "record-1"}) },
		"accepted":   func(w http.ResponseWriter) error { return response.Accepted(w, nil) },
		"no_content": response.NoContent,
		"paginated": func(w http.ResponseWriter) error {
			return response.Paginated(w, []string{"record-1"}, map[string]any{"page": 1, "page_size": 20})
		},
		"application_error": func(w http.ResponseWriter) error {
			e := app.Wrap(errors.New("private cause must never leak"), "Approved public message", app.WithCode("ORDER_CONFLICT"), app.WithCategory(app.Conflict), app.WithHTTPStatus(409), app.WithDebug("private stack must never leak"), app.WithRetryAfter(1500*time.Millisecond), app.WithDetails(map[string]any{"field": "version", "password": "private credential must never leak"}))
			e.TraceID = "trace-1"
			e.CorrelationID = "request-1"
			return response.Error(w, nil, e)
		},
		"invalid_error_status": func(w http.ResponseWriter) error {
			return response.Error(w, nil, app.New("Approved public message", app.WithCode("CUSTOM"), app.WithHTTPStatus(200)))
		},
	}
	for name, helper := range helpers {
		calls[name] = func(w http.ResponseWriter) error { return helper(w, "") }
	}
	data, err := os.ReadFile("testdata/wire-v0.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(data, &expected); err != nil {
		t.Fatal(err)
	}
	if len(calls) != len(expected) {
		t.Fatal("fixture and response inventory differ")
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			w := httptest.NewRecorder()
			before := time.Now().UTC()
			if err := call(w); err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if w.Body.Len() > 0 {
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				timestamp, ok := body["timestamp"].(string)
				if !ok {
					t.Fatal("timestamp missing or changed type")
				}
				parsed, err := time.Parse(time.RFC3339Nano, timestamp)
				if err != nil || parsed.Before(before) || parsed.After(time.Now().UTC()) || !strings.HasSuffix(timestamp, "Z") {
					t.Fatalf("invalid UTC timestamp: %q", timestamp)
				}
				body["timestamp"] = "<timestamp>"
				for _, private := range []string{"private cause", "private stack", "private credential"} {
					if strings.Contains(w.Body.String(), private) {
						t.Fatalf("private diagnostic leaked: %s", w.Body.String())
					}
				}
			}
			actual := map[string]any{"status": w.Code, "content_type": w.Header().Get("Content-Type"), "retry_after": w.Header().Get("Retry-After"), "body": body}
			encoded, err := json.Marshal(actual)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(encoded, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(expected[name], &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("response contract changed\n got: %s\nwant: %s", encoded, expected[name])
			}
		})
	}
}
