package gateway

import (
	"context"
	"fmt"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware/builtin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// InitTracing registers an OTLP/HTTP TracerProvider and the W3C tracecontext
// propagator when the tracing middleware is enabled; otherwise it is a no-op. The
// returned shutdown func flushes pending spans and must be called on graceful
// shutdown.
func InitTracing(ctx context.Context, gatewayConfig *config.GatewayConfig) (shutdown func(context.Context) error, err error) {
	noopShutdown := func(context.Context) error { return nil }
	cfg, enabled, err := builtin.MiddlewareSettings[builtin.TracingConfig](gatewayConfig, builtin.NameTracing)
	if err != nil || !enabled {
		return noopShutdown, err
	}
	serviceName := gatewayConfig.Name

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(cfg.Endpoint)}
	if cfg.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exporter, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return noopShutdown, fmt.Errorf("tracing: failed to create OTLP/HTTP exporter for endpoint %q: %w", cfg.Endpoint, err)
	}

	// Merged second so the configured name overrides the default service.name.
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
