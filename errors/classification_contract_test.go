package errors_test

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	app "github.com/cetrictech/go-platform-kit/errors"
	"net"
	"os"
	"reflect"
	"testing"
	"time"
)

// Released codes, categories, HTTP mappings and public fields form a wire
// contract. Changes to this reviewed fixture require compatibility review.
func TestClassificationContract(t *testing.T) {
	inputs := map[string]error{
		"not_found":       sql.ErrNoRows,
		"timeout":         fmt.Errorf("private wrapper: %w", context.DeadlineExceeded),
		"canceled":        context.Canceled,
		"network_timeout": &net.DNSError{Err: "private resolver diagnostic", Name: "private.internal", IsTimeout: true},
		"network_failure": &net.DNSError{Err: "private resolver diagnostic", Name: "private.internal"},
		"unknown":         stderrors.New("private database password"),
		"application":     app.Wrap(stderrors.New("private cause"), "Approved public message", app.WithCode("ORDER_CONFLICT"), app.WithCategory(app.Conflict), app.WithHTTPStatus(409), app.WithSeverity(app.Warning), app.WithRetryable(true), app.WithRetryAfter(time.Second), app.WithDebug("private stack"), app.WithDetails(map[string]any{"field": "version"})),
	}
	raw, err := os.ReadFile("testdata/classification-v0.json")
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]json.RawMessage
	if err := json.Unmarshal(raw, &expected); err != nil {
		t.Fatal(err)
	}
	if len(inputs) != len(expected) {
		t.Fatal("fixture and classification inventory differ")
	}
	for name, input := range inputs {
		t.Run(name, func(t *testing.T) {
			classified := app.Classify(input)
			payload, err := json.Marshal(struct {
				HTTPStatus int           `json:"http_status"`
				Public     *app.AppError `json:"public"`
			}{classified.HTTPStatus, classified})
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if err := json.Unmarshal(payload, &got); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(expected[name], &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("public contract changed\n got: %s\nwant: %s", payload, expected[name])
			}
			if !stderrors.Is(classified, input) {
				t.Fatal("private cause identity lost")
			}
		})
	}
	if app.Classify(nil) != nil {
		t.Fatal("nil errors must remain nil")
	}
}
