package grpcx_test

import (
	"encoding/json"
	"errors"
	"fmt"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/grpcx"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"os"
	"reflect"
	"testing"
)

func TestPublicStatusContract(t *testing.T) {
	categories := []app.Category{app.Validation, app.Authentication, app.Authorization, app.Business, app.NotFound, app.Conflict, app.RateLimit, app.Database, app.Cache, app.Messaging, app.Network, app.Infrastructure, app.Internal, app.Timeout, app.Unavailable}
	actual := map[string]map[string]any{}
	for _, category := range categories {
		e := app.Wrap(status.Error(codes.Internal, "private cause"), "Approved public message", app.WithCode("APP_CODE"), app.WithCategory(category), app.WithDebug("private stack"))
		s := status.Convert(grpcx.Status(fmt.Errorf("private wrapper: %w", e)))
		public := map[string]any{"code": s.Code().String(), "message": s.Message()}
		for _, detail := range s.Details() {
			if info, ok := detail.(*errdetails.ErrorInfo); ok {
				public["reason"] = info.Reason
				public["domain"] = info.Domain
			}
		}
		actual[string(category)] = public
	}
	retry := app.New("Approved public message", app.WithCategory(app.Database), app.WithRetryable(true))
	actual["DATABASE_RETRYABLE"] = map[string]any{"code": status.Code(grpcx.Status(retry)).String()}
	for _, code := range []codes.Code{codes.Canceled, codes.InvalidArgument, codes.DeadlineExceeded, codes.NotFound, codes.AlreadyExists, codes.PermissionDenied, codes.ResourceExhausted, codes.FailedPrecondition, codes.Aborted, codes.OutOfRange, codes.Unimplemented, codes.Internal, codes.Unavailable, codes.DataLoss, codes.Unauthenticated, codes.Unknown} {
		var local *app.AppError
		if !errors.As(grpcx.Classify(status.Error(code, "private remote diagnostic")), &local) {
			t.Fatalf("unclassified status %s", code)
		}
		actual["REMOTE_"+code.String()] = map[string]any{"code": local.Code, "category": string(local.Category), "http_status": local.HTTPStatus, "retryable": local.Retryable, "message": local.Message}
	}
	payload, err := json.Marshal(actual)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/status-v0.json")
	if err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("status wire contract changed\n got: %s\nwant: %s", payload, expected)
	}
}
