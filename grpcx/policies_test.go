package grpcx

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	pb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/status"
	"net"
	"sync"
	"testing"
	"time"
)

func servePolicy(t *testing.T, cfg ServerConfig, clientCfg ClientConfig) (*Server, *grpc.ClientConn) {
	t.Helper()
	cfg.AllowPlaintext = true
	s, e := NewServer(cfg)
	if e != nil {
		t.Fatal(e)
	}
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go s.Serve(l)
	t.Cleanup(s.GRPC.Stop)
	clientCfg.Target = l.Addr().String()
	clientCfg.AllowPlaintext = true
	c, e := NewClient(clientCfg)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { c.Close() })
	return s, c
}
func TestAuthenticationRunsInsideServerDeadline(t *testing.T) {
	_, client := servePolicy(t, ServerConfig{DefaultTimeout: 20 * time.Millisecond, Authenticate: func(ctx context.Context, _ string) (context.Context, error) { <-ctx.Done(); return ctx, ctx.Err() }}, ClientConfig{})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, e := pb.NewHealthClient(client).Check(ctx, &pb.HealthCheckRequest{})
	if status.Code(e) != codes.DeadlineExceeded {
		t.Fatal(e)
	}
}
func TestStreamingTransportDeadline(t *testing.T) {
	_, client := servePolicy(t, ServerConfig{}, ClientConfig{StreamTimeout: 30 * time.Millisecond})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	stream, e := pb.NewHealthClient(client).Watch(ctx, &pb.HealthCheckRequest{})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = stream.Recv(); e != nil {
		t.Fatal(e)
	}
	if _, e = stream.Recv(); status.Code(e) != codes.DeadlineExceeded {
		t.Fatal(e)
	}
}
func TestForcedShutdownIsBounded(t *testing.T) {
	entered := make(chan struct{})
	block := make(chan struct{})
	var release sync.Once
	t.Cleanup(func() { release.Do(func() { close(block) }) })
	server, client := servePolicy(t, ServerConfig{Unary: []grpc.UnaryServerInterceptor{func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, h grpc.UnaryHandler) (any, error) {
		close(entered)
		<-block
		return h(ctx, req)
	}}}, ClientConfig{})
	done := make(chan error, 1)
	go func() {
		_, e := pb.NewHealthClient(client).Check(context.Background(), &pb.HealthCheckRequest{})
		done <- e
	}()
	<-entered
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	if e := server.Shutdown(ctx); !errors.Is(e, context.DeadlineExceeded) || time.Since(start) > 500*time.Millisecond {
		t.Fatal("unbounded shutdown", e, time.Since(start))
	}
	release.Do(func() { close(block) })
	cleanup, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if e := server.Shutdown(cleanup); e != nil {
		t.Fatal(e)
	}
	select {
	case <-done:
	case <-cleanup.Done():
		t.Fatal("client call leaked")
	}
}
