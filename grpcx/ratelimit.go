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

// StreamRateLimit applies admission limits using the authenticated streaming context.
func StreamRateLimit(l ratelimit.Limiter, key func(context.Context, string) string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if l == nil || key == nil {
			return status.Error(codes.Internal, "Rate limiter unavailable")
		}
		d, e := l.Allow(ss.Context(), key(ss.Context(), info.FullMethod))
		if e != nil {
			return status.Error(codes.Unavailable, "Rate limiter unavailable")
		}
		if !d.Allowed {
			ss.SetTrailer(metadata.Pairs("retry-after-ms", strconv.FormatInt(d.RetryAfter.Milliseconds(), 10)))
			return status.Error(codes.ResourceExhausted, "Rate limit exceeded")
		}
		return h(srv, ss)
	}
}

// StreamMessageRateLimit limits received messages before the application can process them, rather than only limiting stream admission.
func StreamMessageRateLimit(l ratelimit.Limiter, key func(context.Context, string) string) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if l == nil || key == nil {
			return status.Error(codes.Internal, "Rate limiter unavailable")
		}
		return h(srv, &limitedServerStream{ServerStream: ss, limiter: l, key: key(ss.Context(), info.FullMethod)})
	}
}

type limitedServerStream struct {
	grpc.ServerStream
	limiter ratelimit.Limiter
	key     string
}

func (s *limitedServerStream) RecvMsg(m any) error {
	if e := s.ServerStream.RecvMsg(m); e != nil {
		return e
	}
	d, e := s.limiter.Allow(s.Context(), s.key)
	if e != nil {
		return status.Error(codes.Unavailable, "Rate limiter unavailable")
	}
	if !d.Allowed {
		s.SetTrailer(metadata.Pairs("retry-after-ms", strconv.FormatInt(d.RetryAfter.Milliseconds(), 10)))
		return status.Error(codes.ResourceExhausted, "Rate limit exceeded")
	}
	return nil
}
