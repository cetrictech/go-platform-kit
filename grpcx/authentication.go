package grpcx

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"time"
)

// Authenticator authenticates a workload/end-user identity before authorization. It may attach verified identity values to a derived context.
type Authenticator func(context.Context, string) (context.Context, error)

// BearerAuthentication adapts a JWT or other bearer verifier without coupling gRPC to an optional JWT module.
func BearerAuthentication(verify func(context.Context, string) (context.Context, error)) Authenticator {
	return func(ctx context.Context, _ string) (context.Context, error) {
		if verify == nil {
			return ctx, errors.New("grpcx: bearer verifier required")
		}
		md, _ := metadata.FromIncomingContext(ctx)
		values := md.Get("authorization")
		if len(values) != 1 {
			return ctx, errors.New("grpcx: one authorization value required")
		}
		return verify(ctx, values[0])
	}
}

// Authentication rejects failures with a safe Unauthenticated status. Cancellation keeps its standard gRPC meaning.
func Authentication(check Authenticator) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if check != nil {
			next, e := check(ctx, info.FullMethod)
			if e != nil || next == nil {
				if ctx.Err() != nil {
					return nil, Status(ctx.Err())
				}
				return nil, status.Error(codes.Unauthenticated, "Authentication required")
			}
			ctx = next
		}
		return h(ctx, req)
	}
}

// StreamAuthentication applies the same identity policy to an entire stream.
func StreamAuthentication(check Authenticator) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, info *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if check == nil {
			return h(srv, ss)
		}
		ctx, e := check(ss.Context(), info.FullMethod)
		if e != nil || ctx == nil {
			if ss.Context().Err() != nil {
				return Status(ss.Context().Err())
			}
			return status.Error(codes.Unauthenticated, "Authentication required")
		}
		return h(srv, &contextStream{ServerStream: ss, ctx: ctx})
	}
}

// StreamDeadline bounds the context visible to the stream handler. Handlers must honor Context cancellation.
// Clients can impose a transport deadline with ClientConfig.StreamTimeout, including on blocking Recv/Send operations.
func StreamDeadline(timeout time.Duration) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if timeout <= 0 {
			return h(srv, ss)
		}
		ctx, cancel := context.WithTimeout(ss.Context(), timeout)
		defer cancel()
		e := h(srv, &contextStream{ServerStream: ss, ctx: ctx})
		if ctx.Err() != nil {
			return Status(ctx.Err())
		}
		return e
	}
}

type deadlineClientStream struct {
	grpc.ClientStream
	cancel context.CancelFunc
}

func (s *deadlineClientStream) RecvMsg(m any) error {
	e := s.ClientStream.RecvMsg(m)
	if e != nil {
		s.cancel()
	}
	return e
}
func (s *deadlineClientStream) SendMsg(m any) error {
	e := s.ClientStream.SendMsg(m)
	if e != nil {
		s.cancel()
	}
	return e
}

// StreamClientDeadline installs an actual transport deadline and releases its timer on terminal receive/send errors.
func StreamClientDeadline(timeout time.Duration) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if timeout <= 0 {
			return streamer(ctx, desc, cc, method, opts...)
		}
		ctx, cancel := context.WithTimeout(ctx, timeout)
		stream, e := streamer(ctx, desc, cc, method, opts...)
		if e != nil {
			cancel()
			return nil, e
		}
		return &deadlineClientStream{ClientStream: stream, cancel: cancel}, nil
	}
}
