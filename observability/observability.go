// Package observability constructs caller-owned OpenTelemetry providers without changing globals.
package observability

import (
	"context"
	"errors"
	"github.com/cetrictech/go-platform-kit/instrument"
	"github.com/cetrictech/go-platform-kit/logger"
	"github.com/cetrictech/go-platform-kit/messaging/kafka"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Config contains explicit constructor settings; see package documentation for defaults.
type Config struct {
	Service, Version, Environment string
	SampleRatio                   float64
	TraceExporter                 sdktrace.SpanExporter
	MetricReader                  sdkmetric.Reader
	OTLP                          bool
	EndpointURL                   string
	Headers                       map[string]string
}

// Providers owns OpenTelemetry trace and metric providers without global registration.
type Providers struct {
	Traces      *sdktrace.TracerProvider
	Metrics     *sdkmetric.MeterProvider
	Propagation propagation.TextMapPropagator
}

// New constructs and validates an explicitly owned instance. See package documentation for defaults and cleanup.
func New(ctx context.Context, c Config) (*Providers, error) {
	if c.Service == "" || c.SampleRatio < 0 || c.SampleRatio > 1 {
		return nil, errors.New("observability: service and valid sampling ratio required")
	}
	if c.EndpointURL != "" && !strings.HasPrefix(c.EndpointURL, "https://") {
		return nil, errors.New("observability: HTTPS OTLP endpoint required")
	}
	res, e := resource.New(ctx, resource.WithAttributes(attribute.String("service.name", c.Service), attribute.String("service.version", c.Version), attribute.String("deployment.environment.name", c.Environment)))
	if e != nil {
		return nil, e
	}
	exp := c.TraceExporter
	reader := c.MetricReader
	if c.OTLP {
		if exp == nil {
			o := []otlptracehttp.Option{}
			if c.EndpointURL != "" {
				o = append(o, otlptracehttp.WithEndpointURL(c.EndpointURL))
			}
			if len(c.Headers) > 0 {
				o = append(o, otlptracehttp.WithHeaders(c.Headers))
			}
			exp, e = otlptracehttp.New(ctx, o...)
			if e != nil {
				return nil, e
			}
		}
		if reader == nil {
			o := []otlpmetrichttp.Option{}
			if c.EndpointURL != "" {
				o = append(o, otlpmetrichttp.WithEndpointURL(c.EndpointURL))
			}
			if len(c.Headers) > 0 {
				o = append(o, otlpmetrichttp.WithHeaders(c.Headers))
			}
			x, err := otlpmetrichttp.New(ctx, o...)
			if err != nil {
				if exp != nil {
					_ = exp.Shutdown(ctx)
				}
				return nil, err
			}
			reader = sdkmetric.NewPeriodicReader(x)
		}
	}
	to := []sdktrace.TracerProviderOption{sdktrace.WithResource(res), sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(c.SampleRatio)))}
	if exp != nil {
		to = append(to, sdktrace.WithBatcher(exp))
	}
	mo := []sdkmetric.Option{sdkmetric.WithResource(res)}
	if reader != nil {
		mo = append(mo, sdkmetric.WithReader(reader))
	}
	return &Providers{sdktrace.NewTracerProvider(to...), sdkmetric.NewMeterProvider(mo...), propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{})}, nil
}

// Shutdown drains owned work until the context deadline, using documented cancellation fallback.
func (p *Providers) Shutdown(ctx context.Context) error {
	return errors.Join(p.Traces.Shutdown(ctx), p.Metrics.Shutdown(ctx))
}

// Flush waits for pending publication outcomes, bounded by context cancellation.
func (p *Providers) Flush(ctx context.Context) error {
	return errors.Join(p.Traces.ForceFlush(ctx), p.Metrics.ForceFlush(ctx))
}

// Correlate adds active span IDs to slog context.
func Correlate(ctx context.Context) context.Context {
	s := trace.SpanContextFromContext(ctx)
	if !s.IsValid() {
		return ctx
	}
	return logger.Context(ctx, slog.String("trace_id", s.TraceID().String()), slog.String("span_id", s.SpanID().String()))
}

// HTTP instruments requests using a developer-supplied low-cardinality route name.
func HTTP(tp trace.TracerProvider, mp metric.MeterProvider, prop propagation.TextMapPropagator, route string) (func(http.Handler) http.Handler, error) {
	hist, e := mp.Meter("go-platform-kit/http").Float64Histogram("http.server.request.duration", metric.WithUnit("s"))
	if e != nil {
		return nil, e
	}
	tr := tp.Tracer("go-platform-kit/http")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := prop.Extract(r.Context(), propagation.HeaderCarrier(r.Header))
			ctx, span := tr.Start(ctx, route, trace.WithSpanKind(trace.SpanKindServer), trace.WithAttributes(attribute.String("http.route", route), attribute.String("http.request.method", r.Method)))
			defer span.End()
			start := time.Now()
			defer func() {
				hist.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attribute.String("http.route", route)))
			}()
			next.ServeHTTP(w, r.WithContext(Correlate(ctx)))
		})
	}, nil
}

// Transport propagates W3C headers on a cloned request. Its base transport remains caller-owned.
type Transport struct {
	Base        http.RoundTripper
	Tracer      trace.Tracer
	Propagation propagation.TextMapPropagator
}

// RoundTrip instruments an HTTP request and injects trace headers on a cloned request.
func (t Transport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx, s := t.Tracer.Start(r.Context(), r.Method, trace.WithSpanKind(trace.SpanKindClient))
	defer s.End()
	clone := r.Clone(ctx)
	t.Propagation.Inject(ctx, propagation.HeaderCarrier(clone.Header))
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}
	res, e := base.RoundTrip(clone)
	if e != nil {
		s.RecordError(e)
	}
	return res, e
}

// Hook instruments SDK adapters through a vendor-independent callback. It records no SQL, keys, or payloads.
func Hook(tp trace.TracerProvider, mp metric.MeterProvider) (instrument.Hook, error) {
	hist, e := mp.Meter("go-platform-kit").Float64Histogram("sdk.operation.duration", metric.WithUnit("s"))
	if e != nil {
		return nil, e
	}
	tr := tp.Tracer("go-platform-kit")
	return func(ctx context.Context, operation string) (context.Context, func(error)) {
		ctx, s := tr.Start(ctx, operation)
		start := time.Now()
		return ctx, func(e error) {
			if e != nil {
				s.SetStatus(codes.Error, "operation failed")
			}
			hist.Record(ctx, time.Since(start).Seconds(), metric.WithAttributes(attribute.String("operation", operation)))
			s.End()
		}
	}, nil
}

// Producer wraps Kafka publishing with trace propagation. Flush and Close retain their underlying ownership semantics.
type Producer struct {
	kafka.Producer
	Tracer      trace.Tracer
	Propagation propagation.TextMapPropagator
}

// Publish publishes with the configured acknowledgment policy.
func (p Producer) Publish(ctx context.Context, m kafka.Message) error {
	ctx, s := p.Tracer.Start(ctx, "kafka.publish", trace.WithSpanKind(trace.SpanKindProducer))
	defer s.End()
	headers := make(map[string][]byte, len(m.Headers)+2)
	for k, v := range m.Headers {
		headers[k] = v
	}
	carrier := propagation.MapCarrier{}
	p.Propagation.Inject(ctx, carrier)
	for k, v := range carrier {
		headers[k] = []byte(v)
	}
	m.Headers = headers
	e := p.Producer.Publish(ctx, m)
	if e != nil {
		s.SetStatus(codes.Error, "publish failed")
	}
	return e
}

// PublishBatch publishes every supplied message, returning a delivery error if any operation fails.
func (p Producer) PublishBatch(ctx context.Context, msgs []kafka.Message) error {
	for _, m := range msgs {
		if e := p.Publish(ctx, m); e != nil {
			return e
		}
	}
	return nil
}

// Consumer wraps a Kafka handler with trace extraction and duration instrumentation.
func Consumer(tr trace.Tracer, prop propagation.TextMapPropagator, h kafka.Handler) kafka.Handler {
	return func(ctx context.Context, m kafka.Message) error {
		carrier := propagation.MapCarrier{}
		for k, v := range m.Headers {
			carrier[k] = string(v)
		}
		ctx = prop.Extract(ctx, carrier)
		ctx, s := tr.Start(ctx, "kafka.consume", trace.WithSpanKind(trace.SpanKindConsumer))
		defer s.End()
		e := h(Correlate(ctx), m)
		if e != nil {
			s.SetStatus(codes.Error, "handler failed")
		}
		return e
	}
}
