package grpcx

import (
	"context"
	"github.com/cetrictech/go-platform-kit/id"
	"github.com/cetrictech/go-platform-kit/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"log/slog"
	"strings"
)

// RequestIDs generates a server-owned request/correlation ID. Forwarded correlation is accepted only with explicit opt-in and bounded syntax.
func RequestIDs(trustCorrelation bool) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		ctx, e := requestContext(ctx, trustCorrelation)
		if e != nil {
			return nil, e
		}
		return h(ctx, req)
	}
}
func requestContext(ctx context.Context, trust bool) (context.Context, error) {
	u, e := id.NewV7()
	if e != nil {
		return ctx, status.Error(codes.Internal, "Request identity unavailable")
	}
	requestID := u.String()
	correlation := requestID
	if trust {
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("x-correlation-id")
		if len(values) == 1 && validCorrelation(values[0]) {
			correlation = values[0]
		}
	}
	_ = grpc.SetHeader(ctx, metadata.Pairs("x-request-id", requestID, "x-correlation-id", correlation))
	return logger.Context(ctx, slog.String("request_id", requestID), slog.String("correlation_id", correlation)), nil
}
func validCorrelation(s string) bool {
	return len(s) > 0 && len(s) <= 128 && strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.')
	}) < 0
}

type contextStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *contextStream) Context() context.Context { return s.ctx }

// StreamRequestIDs applies the same identity policy to streaming RPCs.
func StreamRequestIDs(trustCorrelation bool) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		ctx, e := requestContext(ss.Context(), trustCorrelation)
		if e != nil {
			return e
		}
		return h(srv, &contextStream{ss, ctx})
	}
}
