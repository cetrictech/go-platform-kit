package observability

import (
	"context"
	sdk "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"testing"
)

func TestProviders(t *testing.T) {
	exp := tracetest.NewInMemoryExporter()
	p, e := New(context.Background(), Config{Service: "test", SampleRatio: 1, TraceExporter: exp})
	if e != nil {
		t.Fatal(e)
	}
	ctx, s := p.Traces.Tracer("test").Start(context.Background(), "operation")
	if ctx == nil {
		t.Fatal("context")
	}
	s.End()
	if e = p.Flush(context.Background()); e != nil {
		t.Fatal(e)
	}
	if len(exp.GetSpans()) != 1 {
		t.Fatal(exp.GetSpans())
	}
	if e = p.Shutdown(context.Background()); e != nil {
		t.Fatal(e)
	}
	_ = sdk.AlwaysSample()
}
