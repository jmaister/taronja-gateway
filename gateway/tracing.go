package gateway

import (
	"context"
	"fmt"

	"github.com/jmaister/taronja-gateway/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitTracing sets up OpenTelemetry distributed tracing from cfg: an OTLP/
// HTTP exporter sending spans to cfg.Endpoint, registered as the global
// TracerProvider (what every otel.Tracer(...) call anywhere in the process
// — including middleware.TracingMiddleware and the otelhttp-wrapped proxy
// transport — dispatches through), plus the W3C tracecontext propagator so
// an incoming "traceparent" header continues a caller's trace instead of
// always starting a new one, and so it gets forwarded to backends in turn.
//
// If cfg.Enabled is false (the default), this is a complete no-op: it
// never touches the global TracerProvider at all, leaving OpenTelemetry's
// own built-in no-op provider in place — every otel.Tracer(...).Start()
// call anywhere still works, just does nothing, at negligible cost. This
// project also never adds TracingMiddleware to the chain (or wraps the
// proxy transport) unless tracing is enabled — see registry_v2.go — so in
// practice a disabled config touches none of this machinery at all; this
// no-op path exists for the same reason as the disabled paths in
// gateway/tls.go and gateway/ja4tls.go: correct behavior if something ever
// does call otel directly regardless.
//
// Returns a shutdown func that flushes any pending, not-yet-exported spans
// and closes the exporter — call it during graceful shutdown (see
// main.go), the same reason gracefulShutdownTimeout exists for in-flight
// requests: a span for the very last requests handled before shutdown
// would otherwise be silently dropped, sitting in the batch exporter's
// internal queue when the process exits.
func InitTracing(ctx context.Context, cfg config.TracingConfig, serviceName string) (shutdown func(context.Context) error, err error) {
	noopShutdown := func(context.Context) error { return nil }
	if !cfg.Enabled {
		return noopShutdown, nil
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return noopShutdown, fmt.Errorf("tracing: failed to create OTLP/HTTP exporter for endpoint %q: %w", cfg.Endpoint, err)
	}

	// resource.Default() already sets a generic "unknown_service:<binary>"
	// service.name; merging in ours (as `b`) overwrites it with the
	// gateway's own configured Name — see resource.Merge's doc comment for
	// why the argument order here is the one that wins.
	res, err := resource.Merge(resource.Default(), resource.NewSchemaless(
		attribute.String("service.name", serviceName),
	))
	if err != nil {
		return noopShutdown, fmt.Errorf("tracing: failed to build resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	return tp.Shutdown, nil
}
