// Package grpcx provides explicit secure gRPC factories and ordered interceptors.
package grpcx

import (
	"context"
	"crypto/tls"
	"errors"
	app "github.com/cetrictech/go-platform-kit/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"log/slog"
	"net"
	"time"
)

// ServerConfig controls explicitly secure gRPC server construction.
type ServerConfig struct {
	MaxConnections                                   int
	TrustCorrelation                                 bool
	StatsHandler                                     stats.Handler
	Credentials                                      credentials.TransportCredentials
	TLS                                              *tls.Config
	AllowPlaintext, Reflection                       bool
	Logger                                           *slog.Logger
	Authorize                                        func(context.Context, string) error
	DefaultTimeout                                   time.Duration
	MaxRecvBytes, MaxSendBytes, MaxConcurrentStreams int
	Unary                                            []grpc.UnaryServerInterceptor
	Stream                                           []grpc.StreamServerInterceptor
	Options                                          []grpc.ServerOption
}

// Server owns a gRPC server and its registered health service.
type Server struct {
	maxConnections int
	GRPC           *grpc.Server
	Health         *health.Server
}

// NewServer constructs a gRPC server; verified transport or explicit plaintext opt-in is required.
func NewServer(c ServerConfig) (*Server, error) {
	if c.MaxConnections < 0 || c.MaxConcurrentStreams < 0 || c.MaxRecvBytes < 0 || c.MaxSendBytes < 0 || c.DefaultTimeout < 0 {
		return nil, errors.New("grpcx: invalid limits")
	}
	creds := c.Credentials
	if creds == nil && c.TLS != nil {
		t := c.TLS.Clone()
		if t.MinVersion < tls.VersionTLS12 {
			t.MinVersion = tls.VersionTLS12
		}
		creds = credentials.NewTLS(t)
	}
	if creds == nil && !c.AllowPlaintext {
		return nil, errors.New("grpcx: transport credentials required")
	}
	opts := []grpc.ServerOption{}
	if c.StatsHandler != nil {
		opts = append(opts, grpc.StatsHandler(c.StatsHandler))
	}
	if creds != nil {
		opts = append(opts, grpc.Creds(creds))
	}
	unary := []grpc.UnaryServerInterceptor{Recovery(c.Logger), Translate(), RequestIDs(c.TrustCorrelation), Logging(c.Logger), Authorize(c.Authorize), Deadline(c.DefaultTimeout)}
	unary = append(unary, c.Unary...)
	streams := []grpc.StreamServerInterceptor{StreamRecovery(c.Logger), StreamRequestIDs(c.TrustCorrelation), StreamAuthorize(c.Authorize)}
	streams = append(streams, c.Stream...)
	opts = append(opts, grpc.ChainUnaryInterceptor(unary...), grpc.ChainStreamInterceptor(streams...), grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{MinTime: 5 * time.Second}))
	if c.MaxRecvBytes > 0 {
		opts = append(opts, grpc.MaxRecvMsgSize(c.MaxRecvBytes))
	}
	if c.MaxSendBytes > 0 {
		opts = append(opts, grpc.MaxSendMsgSize(c.MaxSendBytes))
	}
	if c.MaxConcurrentStreams > 0 {
		opts = append(opts, grpc.MaxConcurrentStreams(uint32(c.MaxConcurrentStreams)))
	}
	opts = append(opts, c.Options...)
	s := grpc.NewServer(opts...)
	h := health.NewServer()
	healthpb.RegisterHealthServer(s, h)
	if c.Reflection {
		reflection.Register(s)
	}
	return &Server{GRPC: s, Health: h, maxConnections: c.MaxConnections}, nil
}

// Serve serves requests on the caller-supplied listener.
func (s *Server) Serve(l net.Listener) error {
	if s.maxConnections > 0 {
		limited, e := LimitListener(l, s.maxConnections)
		if e != nil {
			return e
		}
		l = limited
	}
	return s.GRPC.Serve(l)
}

// Shutdown drains owned work until the context deadline, using documented cancellation fallback.
func (s *Server) Shutdown(ctx context.Context) error {
	s.Health.Shutdown()
	done := make(chan struct{})
	go func() { s.GRPC.GracefulStop(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		s.GRPC.Stop()
		<-done
		return ctx.Err()
	}
}

// ClientConfig controls a caller-owned gRPC connection and its transport policy.
type ClientConfig struct {
	StatsHandler   stats.Handler
	Target         string
	Credentials    credentials.TransportCredentials
	TLS            *tls.Config
	AllowPlaintext bool
	DefaultTimeout time.Duration
	Unary          []grpc.UnaryClientInterceptor
	Stream         []grpc.StreamClientInterceptor
	Options        []grpc.DialOption
}

// NewClient constructs a caller-owned gRPC connection without performing an initial blocking dial.
func NewClient(c ClientConfig) (*grpc.ClientConn, error) {
	if c.Target == "" {
		return nil, errors.New("grpcx: target required")
	}
	creds := c.Credentials
	if creds == nil && c.TLS != nil {
		t := c.TLS.Clone()
		if t.InsecureSkipVerify && t.VerifyPeerCertificate == nil && t.VerifyConnection == nil {
			return nil, errors.New("grpcx: certificate verification required")
		}
		if t.MinVersion < tls.VersionTLS12 {
			t.MinVersion = tls.VersionTLS12
		}
		creds = credentials.NewTLS(t)
	}
	if creds == nil {
		if !c.AllowPlaintext {
			return nil, errors.New("grpcx: transport credentials required")
		}
		creds = insecure.NewCredentials()
	}
	unary := []grpc.UnaryClientInterceptor{func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if c.DefaultTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, c.DefaultTimeout)
			defer cancel()
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}}
	unary = append(unary, c.Unary...)
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds), grpc.WithChainUnaryInterceptor(unary...), grpc.WithChainStreamInterceptor(c.Stream...)}
	if c.StatsHandler != nil {
		opts = append(opts, grpc.WithStatsHandler(c.StatsHandler))
	}
	opts = append(opts, c.Options...)
	return grpc.NewClient(c.Target, opts...)
}

// Recovery returns a reporting panic-recovery interceptor or middleware.
func Recovery(l *slog.Logger) grpc.UnaryServerInterceptor {
	if l == nil {
		l = slog.Default()
	}
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, h grpc.UnaryHandler) (out any, err error) {
		defer func() {
			if p := recover(); p != nil {

				l.ErrorContext(ctx, "RPC panic", "method", info.FullMethod, "panic", p)
				err = status.Error(codes.Internal, "Internal server error")
			}
		}()
		return h(ctx, req)
	}
}

// Translate returns a safe application-error translation interceptor.
func Translate() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		out, e := h(ctx, req)
		return out, Status(e)
	}
}

// Status translates errors into safe gRPC status values.
func Status(e error) error {
	if e == nil {
		return nil
	}
	if _, ok := status.FromError(e); ok {
		return e
	}
	if errors.Is(e, context.Canceled) {
		return status.Error(codes.Canceled, "Request canceled")
	}
	if errors.Is(e, context.DeadlineExceeded) {
		return status.Error(codes.DeadlineExceeded, "Request timed out")
	}
	var a *app.AppError
	if !errors.As(e, &a) {
		return status.Error(codes.Internal, "Internal server error")
	}
	c := codes.Internal
	switch a.Category {
	case app.Validation:
		c = codes.InvalidArgument
	case app.Authentication:
		c = codes.Unauthenticated
	case app.Authorization:
		c = codes.PermissionDenied
	case app.NotFound:
		c = codes.NotFound
	case app.Conflict:
		c = codes.AlreadyExists
	case app.RateLimit:
		c = codes.ResourceExhausted
	case app.Timeout:
		c = codes.DeadlineExceeded
	case app.Unavailable, app.Network:
		c = codes.Unavailable
	}
	return status.Error(c, a.Message)
}

// Logging returns structured request-logging instrumentation.
func Logging(l *slog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, i *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		start := time.Now()
		out, e := h(ctx, req)
		if l != nil {
			l.InfoContext(ctx, "RPC request", "method", i.FullMethod, "code", status.Code(Status(e)).String(), "duration", time.Since(start))
		}
		return out, e
	}
}

// Authorize applies an explicit caller-supplied authorization policy.
func Authorize(check func(context.Context, string) error) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, i *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if check != nil {
			if e := check(ctx, i.FullMethod); e != nil {
				return nil, Status(e)
			}
		}
		return h(ctx, req)
	}
}

// Deadline bounds unary calls with a configured context deadline.
func Deadline(d time.Duration) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		if d > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, d)
			defer cancel()
		}
		return h(ctx, req)
	}
}

// StreamRecovery recovers and reports stream handler panics.
func StreamRecovery(l *slog.Logger) grpc.StreamServerInterceptor {
	if l == nil {
		l = slog.Default()
	}
	return func(srv any, ss grpc.ServerStream, i *grpc.StreamServerInfo, h grpc.StreamHandler) (err error) {
		defer func() {
			if p := recover(); p != nil {

				l.ErrorContext(ss.Context(), "stream panic", "method", i.FullMethod, "panic", p)
				err = status.Error(codes.Internal, "Internal server error")
			}
		}()
		return Status(h(srv, ss))
	}
}

// StreamAuthorize applies authorization to streaming RPCs.
func StreamAuthorize(check func(context.Context, string) error) grpc.StreamServerInterceptor {
	return func(srv any, ss grpc.ServerStream, i *grpc.StreamServerInfo, h grpc.StreamHandler) error {
		if check != nil {
			if e := check(ss.Context(), i.FullMethod); e != nil {
				return Status(e)
			}
		}
		return h(srv, ss)
	}
}
