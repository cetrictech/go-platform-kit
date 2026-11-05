package observability

import (
	"context"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/stats"
	"google.golang.org/grpc/status"
	"time"
)

type metadataCarrier metadata.MD

// Get reads one value into the requested representation.
func (m metadataCarrier) Get(k string) string {
	a := metadata.MD(m).Get(k)
	if len(a) > 0 {
		return a[0]
	}
	return ""
}

// Set stores a value with the requested expiration.
func (m metadataCarrier) Set(k, v string) { metadata.MD(m).Set(k, v) }
func (m metadataCarrier) Keys() []string {
	a := make([]string, 0, len(m))
	for k := range m {
		a = append(a, k)
	}
	return a
}

type rpcKey struct{}
type rpcSpan struct {
	span  trace.Span
	start time.Time
}
type grpcHandler struct {
	client bool
	tracer trace.Tracer
	hist   metric.Float64Histogram
	prop   propagation.TextMapPropagator
}

// GRPC instruments RPCs with W3C propagation. Set client=true for client connections, false for servers.
func GRPC(tp trace.TracerProvider, mp metric.MeterProvider, prop propagation.TextMapPropagator, client bool) (stats.Handler, error) {
	h, e := mp.Meter("go-platform-kit/grpc").Float64Histogram("rpc.duration", metric.WithUnit("s"))
	if e != nil {
		return nil, e
	}
	return &grpcHandler{client, tp.Tracer("go-platform-kit/grpc"), h, prop}, nil
}
func (h *grpcHandler) TagRPC(ctx context.Context, i *stats.RPCTagInfo) context.Context {
	kind := trace.SpanKindServer
	if h.client {
		kind = trace.SpanKindClient
	} else {
		md, _ := metadata.FromIncomingContext(ctx)
		ctx = h.prop.Extract(ctx, metadataCarrier(md))
	}
	ctx, s := h.tracer.Start(ctx, i.FullMethodName, trace.WithSpanKind(kind))
	if h.client {
		md, _ := metadata.FromOutgoingContext(ctx)
		md = md.Copy()
		if md == nil {
			md = metadata.MD{}
		}
		h.prop.Inject(ctx, metadataCarrier(md))
		ctx = metadata.NewOutgoingContext(ctx, md)
	}
	return context.WithValue(Correlate(ctx), rpcKey{}, rpcSpan{span: s, start: time.Now()})
}
func (h *grpcHandler) HandleRPC(ctx context.Context, event stats.RPCStats) {
	if end, ok := event.(*stats.End); ok {
		v, ok := ctx.Value(rpcKey{}).(rpcSpan)
		if !ok {
			return
		}
		if end.Error != nil {
			v.span.SetStatus(codes.Error, "RPC failed")
		}
		h.hist.Record(ctx, time.Since(v.start).Seconds(), metric.WithAttributes(attribute.String("rpc.grpc.status_code", status.Code(end.Error).String())))
		v.span.End()
	}
}
func (h *grpcHandler) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context { return ctx }
func (h *grpcHandler) HandleConn(context.Context, stats.ConnStats)                       {}
