# OpenTelemetry

New creates owned trace/metric providers and a W3C TraceContext+Baggage propagator. It never installs global providers. Set Service and SampleRatio explicitly; ratio zero samples no root spans, while parent decisions remain honored. Version and Environment become resource attributes. Supply a TraceExporter or MetricReader, or enable OTLP. EndpointURL must use HTTPS. OTLP environment configuration is also honored by the official exporters; review endpoint/header environment variables in deployment.

Shutdown closes providers and exporters/readers passed into New, so do not pass shared exporters unless ownership transfer is intended. Flush forces pending data out. Applications with existing providers may use HTTP, Transport, Hook and Kafka wrappers directly without calling New.

HTTP requires a developer-defined route template, never a raw user path, and records duration without request-ID labels. Transport clones requests and injects propagation headers. Hook supplies fixed-name adapter spans/metrics for PostgreSQL and Redis and wrapper operations for MySQL. Producer/Consumer wrappers propagate Kafka trace headers. Correlate adds active trace/span IDs to logger context. Arbitrary error strings and SQL are excluded from adapter spans. Native driver escape hatches can bypass instrumentation.

Official gRPC instrumentation can be supplied through grpcx Options/interceptors. A Prometheus-compatible reader can be provided via Config.MetricReader; the SDK does not own its HTTP listener. Avoid double instrumentation and global registration by the application and SDK simultaneously.

GRPC builds a caller-owned gRPC stats handler for unary and streaming RPCs. Supply it through grpcx's StatsHandler field. It propagates W3C headers and records RPC durations by bounded gRPC status labels; raw method paths are excluded from metric labels. A real client/server test verifies matching trace IDs.
