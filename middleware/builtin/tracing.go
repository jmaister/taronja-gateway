package builtin

import (
	"fmt"
	"net/http"

	"github.com/jmaister/taronja-gateway/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	yaml "gopkg.in/yaml.v3"
)

// TracingMiddleware wraps next with OpenTelemetry HTTP server
// instrumentation (go.opentelemetry.io/contrib's otelhttp, the standard,
// upstream-maintained package for exactly this — not a hand-rolled span
// wrapper): it extracts any incoming W3C "traceparent" header to continue
// a caller's trace (or starts a new one if there isn't one), creates a
// server span tagged with standard HTTP semantic-convention attributes
// (method, route, status code, request/response size, ...), and ends it
// once the response completes.
//
// The span this creates dispatches through whatever *sdktrace.
// TracerProvider gateway.InitTracing registered globally — this
// middleware itself holds no state and doesn't know or care whether
// tracing is actually configured. Like every filter, "disabled" means "not
// in the chain", not a per-request check.
func TracingMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "gateway.request")
}

// FilterTracing creates an OpenTelemetry span per request and exports it over
// OTLP/HTTP. The exporter is set up once at startup by gateway.InitTracing.
const FilterTracing = "tracing"

// TracingConfig holds the tracing filter options.
type TracingConfig struct {
	// Endpoint is the OTLP/HTTP collector's host:port, no scheme or path
	// (e.g. "localhost:4318"). Required.
	Endpoint string `yaml:"endpoint"`
	// Insecure sends spans over plain HTTP. Default: false (HTTPS). Local
	// collectors without TLS need true.
	Insecure bool `yaml:"insecure,omitempty"`
}

// Validate requires an endpoint.
func (c TracingConfig) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}
	return nil
}

type tracingFactory struct{ middleware.ConcreteFactory }

func newTracingFactory() *tracingFactory {
	return &tracingFactory{ConcreteFactory: middleware.NewConcreteFactory(FilterTracing, "Creates an OpenTelemetry span per request and propagates trace context to proxied backends")}
}

func (f *tracingFactory) DecodeConfig(raw *yaml.Node) (interface{}, error) {
	return middleware.DecodeConfigInto(raw, TracingConfig{})
}

func (f *tracingFactory) Create(interface{}) (middleware.Middleware, error) {
	return TracingMiddleware, nil
}
