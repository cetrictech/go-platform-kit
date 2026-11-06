package grpcx

import (
	"fmt"
	app "github.com/cetrictech/go-platform-kit/errors"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"strings"
	"testing"
	"time"
)

func TestWrappedDiagnosticsNeverBecomeStatus(t *testing.T) {
	e := app.Wrap(status.Error(codes.Internal, "secret cause"), "Safe message", app.WithDebug("private debug"), app.WithCode("SAFE"), app.WithCategory(app.Validation), app.WithRetryAfter(time.Second), app.WithDetails(map[string]any{"nested": struct{ Password string }{"secret"}}))
	s := status.Convert(Status(e))
	if s.Code() != codes.InvalidArgument || s.Message() != "Safe message" || strings.Contains(fmt.Sprint(s.Details()), "secret") || strings.Contains(fmt.Sprint(s.Details()), "private") {
		t.Fatal(s)
	}
	wrapped := fmt.Errorf("private wrapper: %w", status.Error(codes.NotFound, "safe native"))
	if s := status.Convert(Status(wrapped)); s.Message() != "safe native" {
		t.Fatal(s)
	}
	classified := Classify(status.Error(codes.Unavailable, "private remote"))
	if a := app.Classify(classified); a.HTTPStatus != 503 || !a.Retryable || strings.Contains(a.Message, "private") {
		t.Fatal(a)
	}
}
