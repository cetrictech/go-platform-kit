package grpcx

import (
	"context"
	"github.com/cetrictech/go-platform-kit/ratelimit"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"strconv"
)

// RateLimit applies a caller-supplied identity policy; key must use authenticated context information.
func RateLimit(l ratelimit.Limiter, key func(context.Context, string) string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if l == nil || key == nil {
			return nil, status.Error(codes.Internal, "Rate limiter unavailable")
		}
		d, e := l.Allow(ctx, key(ctx, info.FullMethod))
		if e != nil {
			return nil, status.Error(codes.Unavailable, "Rate limiter unavailable")
		}
		if !d.Allowed {
			_ = grpc.SetHeader(ctx, metadata.Pairs("retry-after-ms", strconv.FormatInt(d.RetryAfter.Milliseconds(), 10)))
			return nil, status.Error(codes.ResourceExhausted, "Rate limit exceeded")
		}
		return h(ctx, req)
	}
}
