package observability

import (
	"context"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	pb "google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"testing"
	"time"
)

func TestGRPCPropagation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	exp := tracetest.NewInMemoryExporter()
	p, e := New(ctx, Config{Service: "test", SampleRatio: 1, TraceExporter: exp})
	if e != nil {
		t.Fatal(e)
	}
	defer p.Shutdown(context.Background())
	serverHandler, e := GRPC(p.Traces, p.Metrics, p.Propagation, false)
	if e != nil {
		t.Fatal(e)
	}
	clientHandler, e := GRPC(p.Traces, p.Metrics, p.Propagation, true)
	if e != nil {
		t.Fatal(e)
	}
	s := grpc.NewServer(grpc.StatsHandler(serverHandler))
	pb.RegisterHealthServer(s, health.NewServer())
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go s.Serve(l)
	defer s.Stop()
	c, e := grpc.NewClient(l.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithStatsHandler(clientHandler))
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	if _, e = pb.NewHealthClient(c).Check(ctx, &pb.HealthCheckRequest{}); e != nil {
		t.Fatal(e)
	}
	if e = p.Flush(ctx); e != nil {
		t.Fatal(e)
	}
	spans := exp.GetSpans()
	if len(spans) != 2 {
		t.Fatal("expected client and server spans", len(spans))
	}
	if spans[0].SpanContext.TraceID() != spans[1].SpanContext.TraceID() {
		t.Fatal("trace context lost")
	}
}
