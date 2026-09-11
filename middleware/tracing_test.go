package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// useInMemoryTracing registers an in-memory span exporter as the global
// OpenTelemetry TracerProvider for the calling test, and restores a no-op
// provider afterward so this test's spans can't leak into (or be
// interpreted by) whichever test runs next in this binary. WithSyncer
// (not WithBatcher) makes export synchronous — a span is in the exporter
// the instant Span.End() returns, no flush/wait needed — which is exactly
// what makes this a viable substitute for a real collector in a test: no
// network, no background batching delay, just a normal in-process
// function call recording what would otherwise have been serialized and
// sent over OTLP/HTTP.
func useInMemoryTracing(t *testing.T) *tracetest.InMemoryExporter {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() {
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
	return exporter
}

func TestTracingMiddleware_CreatesASpanPerRequest(t *testing.T) {
	exporter := useInMemoryTracing(t)

	handler := TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/widgets", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1, "exactly one span per request")
	assert.True(t, spans[0].SpanContext.IsValid(), "the span must have a real trace/span ID, not a zero value")
	assert.False(t, spans[0].Parent.IsValid(), "no incoming traceparent header, so this must start a new trace, not continue one")
}

func TestTracingMiddleware_ContinuesAnIncomingTrace(t *testing.T) {
	// The core distributed-tracing correctness property: a request
	// arriving with a W3C traceparent header must produce a span that's a
	// *child* of that trace, not a new, disconnected one — otherwise
	// there's no distributed trace at all, just isolated per-hop spans
	// that happen to share a timestamp.
	exporter := useInMemoryTracing(t)

	handler := TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	const incomingTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	const incomingSpanID = "00f067aa0ba902b7"
	req := httptest.NewRequest(http.MethodGet, "/widgets", nil)
	req.Header.Set("traceparent", "00-"+incomingTraceID+"-"+incomingSpanID+"-01")
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	assert.Equal(t, incomingTraceID, spans[0].SpanContext.TraceID().String(), "the span must belong to the caller's trace")
	assert.Equal(t, incomingSpanID, spans[0].Parent.SpanID().String(), "the span's parent must be the caller's span")
}

func TestTracingMiddleware_RecordsResponseStatus(t *testing.T) {
	exporter := useInMemoryTracing(t)

	handler := TracingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	spans := exporter.GetSpans()
	require.Len(t, spans, 1)
	// "http.response.status_code" is the current HTTP semantic-convention
	// attribute name otelhttp uses (confirmed against the actual span,
	// not assumed) — it has been renamed across semconv versions before,
	// so a future otelhttp/semconv bump changing it again should fail
	// this test loudly rather than have a fallback silently paper over
	// which name is really in use.
	var sawStatusCode bool
	for _, attr := range spans[0].Attributes {
		if string(attr.Key) == "http.response.status_code" {
			sawStatusCode = true
			assert.EqualValues(t, http.StatusNotFound, attr.Value.AsInt64())
		}
	}
	assert.True(t, sawStatusCode, "the span must record the actual response status code")
}
