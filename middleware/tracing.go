package middleware

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
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
// tracing is actually configured. It's only ever added to the chain when
// it is (see registry_v2.go), the same convention as every other
// optional global middleware here (compression, CORS, ...): the
// "disabled" state is "not in the chain at all," not an internal
// enabled-check on every request.
func TracingMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "gateway.request")
}

// --- TracingFactory ----------------------------------------------------

// TracingFactory creates the OpenTelemetry tracing middleware. Has no
// runtime dependencies and no per-entry configuration of its own — the
// actual exporter/endpoint setup is top-level (config.TracingConfig,
// applied once via gateway.InitTracing at startup), not something this
// factory or a `middleware.global` entry can override per position.
type TracingFactory struct{ ConcreteFactory }

func NewTracingFactory() *TracingFactory {
	return &TracingFactory{
		ConcreteFactory: ConcreteFactory{
			name:        config.MiddlewareNameTracing,
			description: "Creates an OpenTelemetry span per request and propagates trace context to proxied backends",
		},
	}
}

func (f *TracingFactory) Create(cfg interface{}) (Middleware, error) {
	return TracingMiddleware, nil
}

func (f *TracingFactory) GetDefaultConfig() interface{} {
	return struct{}{}
}
