package observability

import (
	"context"
	metricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOTLPExportOverTLS(t *testing.T) {
	var traces, metrics atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-SDK-Test") != "export" {
			t.Error("missing export header")
		}
		data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		var response proto.Message
		switch r.URL.Path {
		case "/v1/traces":
			request := new(tracepb.ExportTraceServiceRequest)
			if err := proto.Unmarshal(data, request); err != nil {
				t.Error(err)
			}
			for _, rs := range request.ResourceSpans {
				for _, ss := range rs.ScopeSpans {
					for _, span := range ss.Spans {
						if span.Name == "sdk.integration" {
							traces.Add(1)
						}
					}
				}
			}
			response = &tracepb.ExportTraceServiceResponse{}
		case "/v1/metrics":
			request := new(metricpb.ExportMetricsServiceRequest)
			if err := proto.Unmarshal(data, request); err != nil {
				t.Error(err)
			}
			for _, rm := range request.ResourceMetrics {
				for _, sm := range rm.ScopeMetrics {
					for _, m := range sm.Metrics {
						if m.Name == "sdk.test.count" {
							metrics.Add(1)
						}
					}
				}
			}
			response = &metricpb.ExportMetricsServiceResponse{}
		default:
			t.Errorf("unexpected OTLP path %q", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		body, err := proto.Marshal(response)
		if err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write(body)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p, err := New(ctx, Config{Service: "sdk-export-test", SampleRatio: 1, OTLP: true, EndpointURL: server.URL, OTLPHTTPClient: server.Client(), Headers: map[string]string{"X-SDK-Test": "export"}})
	if err != nil {
		t.Fatal(err)
	}
	_, span := p.Traces.Tracer("test").Start(ctx, "sdk.integration")
	span.End()
	counter, err := p.Metrics.Meter("test").Int64Counter("sdk.test.count")
	if err != nil {
		t.Fatal(err)
	}
	counter.Add(ctx, 1)
	if err := p.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if traces.Load() != 1 || metrics.Load() == 0 {
		t.Fatalf("exported traces=%d metrics=%d", traces.Load(), metrics.Load())
	}
	if err := p.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
}
