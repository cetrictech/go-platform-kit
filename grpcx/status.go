package grpcx

import (
	"context"
	"encoding/json"
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"github.com/cetrictech/go-platform-kit/security"
	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"strings"
)

// Status prioritizes safe AppError fields over underlying gRPC diagnostics. Wrapped native statuses discard outer debug text.
func Status(e error) error {
	if e == nil {
		return nil
	}
	var a *app.AppError
	if errors.As(e, &a) && a != nil {
		return applicationStatus(a)
	}
	if errors.Is(e, context.Canceled) {
		return status.Error(codes.Canceled, "Request canceled")
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Request timed out")
	}
	var native interface{ GRPCStatus() *status.Status }
	if errors.As(e, &native) && native.GRPCStatus() != nil {
		return native.GRPCStatus().Err()
	}
	return status.Error(codes.Internal, "Internal server error")
}
func applicationStatus(a *app.AppError) error {
	code := codes.Internal
	switch a.Category {
	case app.Validation:
		code = codes.InvalidArgument
	case app.Authentication:
		code = codes.Unauthenticated
	case app.Authorization:
		code = codes.PermissionDenied
	case app.NotFound:
		code = codes.NotFound
	case app.Conflict:
		code = codes.AlreadyExists
	case app.Business:
		code = codes.FailedPrecondition
	case app.RateLimit:
		code = codes.ResourceExhausted
	case app.Timeout:
		code = codes.DeadlineExceeded
	case app.Unavailable, app.Network:
		code = codes.Unavailable
	case app.Database:
		if a.Retryable {
			code = codes.Aborted
		}
	}
	s := status.New(code, a.Message)
	reason := a.Code
	if len(reason) > 128 {
		reason = "APPLICATION_ERROR"
	}
	info := &errdetails.ErrorInfo{Reason: reason, Domain: "go-platform-kit", Metadata: map[string]string{}}
	if a.TraceID != "" {
		info.Metadata["trace_id"] = a.TraceID
	}
	if a.CorrelationID != "" {
		info.Metadata["correlation_id"] = a.CorrelationID
	}
	if next, e := s.WithDetails(info); e == nil {
		s = next
	}
	if a.RetryAfter > 0 {
		if next, e := s.WithDetails(&errdetails.RetryInfo{RetryDelay: durationpb.New(a.RetryAfter)}); e == nil {
			s = next
		}
	}
	if len(a.Details) > 0 {
		safe := security.Redact(a.Details)
		raw, e := json.Marshal(safe)
		if e == nil && len(raw) <= 16<<10 {
			var normalized map[string]any
			if json.Unmarshal(raw, &normalized) == nil {
				if detail, e := structpb.NewStruct(normalized); e == nil {
					if next, e := s.WithDetails(detail); e == nil {
						s = next
					}
				}
			}
		}
	}
	return s.Err()
}

// Classify maps remote statuses to safe local errors, preserving the original error without copying remote diagnostic text into the public message.
func Classify(e error) error {
	if e == nil {
		return nil
	}
	s, ok := status.FromError(e)
	if !ok {
		return app.Classify(e)
	}
	category, httpStatus, retry := app.Internal, 500, false
	switch s.Code() {
	case codes.InvalidArgument, codes.OutOfRange:
		category, httpStatus = app.Validation, 400
	case codes.Unauthenticated:
		category, httpStatus = app.Authentication, 401
	case codes.PermissionDenied:
		category, httpStatus = app.Authorization, 403
	case codes.NotFound:
		category, httpStatus = app.NotFound, 404
	case codes.AlreadyExists:
		category, httpStatus = app.Conflict, 409
	case codes.Aborted:
		category, httpStatus, retry = app.Conflict, 409, true
	case codes.ResourceExhausted:
		category, httpStatus, retry = app.RateLimit, 429, true
	case codes.DeadlineExceeded:
		category, httpStatus, retry = app.Timeout, 504, true
	case codes.Unavailable:
		category, httpStatus, retry = app.Unavailable, 503, true
	case codes.FailedPrecondition:
		category, httpStatus = app.Business, 422
	case codes.Canceled:
		category, httpStatus = app.Network, 408
	}
	a := app.Wrap(e, "The RPC could not be completed.", app.WithCode("RPC_"+strings.ToUpper(s.Code().String())), app.WithCategory(category), app.WithHTTPStatus(httpStatus), app.WithRetryable(retry))
	for _, detail := range s.Details() {
		if r, ok := detail.(*errdetails.RetryInfo); ok && r.RetryDelay.CheckValid() == nil {
			a.RetryAfter = r.RetryDelay.AsDuration()
		}
	}
	return a
}
