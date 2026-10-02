package gateway

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// TestInitTracing_DisabledIsNoOp confirms the documented contract:
// cfg.Enabled == false must not touch the global TracerProvider at all,
// so anything else in the process that happens to call otel.Tracer(...)
// keeps using whatever was already registered (OpenTelemetry's own
// default no-op provider, absent any other Init call).
func TestInitTracing_DisabledIsNoOp(t *testing.T) {
	before := otel.GetTracerProvider()

	shutdown, err := InitTracing(context.Background(), config.TracingConfig{Enabled: false}, "test-service")
	require.NoError(t, err)

	assert.Same(t, before, otel.GetTracerProvider(), "a disabled config must not install a TracerProvider")
	assert.NoError(t, shutdown(context.Background()))
}

// TestInitTracing_EnabledExportsRealSpansOverOTLP is the "does this
// actually work end-to-end" check the in-memory-exporter unit tests in
// middleware/tracing_test.go can't cover: it verifies real spans travel
// over a real OTLP/HTTP request to a real (if fake) collector, using an
// httptest.Server standing in for one — no Jaeger/OTel Collector/Docker
// needed to confirm the exporter itself is wired correctly.
func TestInitTracing_EnabledExportsRealSpansOverOTLP(t *testing.T) {
	var (
		mu           sync.Mutex
		requestCount int
		lastBody     []byte
		lastPath     string
	)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		requestCount++
		lastBody = body
		lastPath = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()

	endpoint := collector.Listener.Addr().String() // host:port, no scheme — see config.TracingConfig.Endpoint
	shutdown, err := InitTracing(context.Background(), config.TracingConfig{
		Enabled:  true,
		Endpoint: endpoint,
		Insecure: true, // the fake collector above is plain HTTP
	}, "test-service")
	require.NoError(t, err)

	tracer := otel.Tracer("test")
	_, span := tracer.Start(context.Background(), "test-span")
	span.End()

	// TracerProvider.Shutdown flushes every pending span through the
	// exporter before returning — this is what makes the batched export
	// (sdktrace.WithBatcher, the production-appropriate option InitTracing
	// actually uses) observable synchronously in a test, instead of racing
	// a background timer.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, shutdown(shutdownCtx))

	mu.Lock()
	defer mu.Unlock()
	assert.GreaterOrEqual(t, requestCount, 1, "the exporter should have sent at least one OTLP export request to the collector")
	assert.Equal(t, "/v1/traces", lastPath, "otlptracehttp appends the standard OTLP traces path itself")
	assert.NotEmpty(t, lastBody, "the export request must carry the actual span data, not an empty body")
}

// TestGatewayPropagatesTraceContextToBackend is the actually-distributed
// half of distributed tracing: a real request through a real Gateway,
// with a real proxy route, must carry a "traceparent" header to the
// backend it proxies to — otherwise every hop gets its own disconnected
// span and nothing is actually being traced *across* the request's
// journey. This only works because InitTracing (called for real here,
// against a fake collector — see TestInitTracing_EnabledExportsRealSpansOverOTLP
// for why that's enough) registers the W3C propagator globally;
// otelhttp.NewTransport (wired in by createProxyHandlerFunc when tracing
// is enabled) uses whatever's globally registered, the same way
// TracingMiddleware does.
func TestGatewayPropagatesTraceContextToBackend(t *testing.T) {
	// t.Cleanup, not defer, and in this order: cleanups run LIFO, so
	// registering the collector's shutdown first and the tracer's flush
	// last means the flush (which needs the collector still listening)
	// actually runs before the collector stops, instead of racing it.
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(collector.Close)
	shutdown, err := InitTracing(context.Background(), config.TracingConfig{
		Enabled:  true,
		Endpoint: collector.Listener.Addr().String(),
		Insecure: true,
	}, "test-service")
	require.NoError(t, err)
	t.Cleanup(func() { _ = shutdown(context.Background()) })

	var receivedTraceparent string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gwConfig := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin"},
		Tracing:    config.TracingConfig{Enabled: true, Endpoint: collector.Listener.Addr().String(), Insecure: true},
		Routes: []config.RouteConfig{
			{
				Name:           "TracedProxy",
				From:           "/proxy",
				To:             []string{backend.URL},
				Authentication: config.AuthenticationConfig{Enabled: false},
			},
		},
	}

	gw, err := NewGatewayWithDependencies(gwConfig, nil, deps.NewTest())
	require.NoError(t, err)

	listener, err := net.Listen("tcp", gw.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	go func() { _ = gw.Server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = gw.Server.Shutdown(ctx)
	})
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL + "/proxy")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.NotEmpty(t, receivedTraceparent, "the proxied request must carry a traceparent header to the backend")
}

// receivedSpan is the subset of an OTLP-exported span this test cares
// about, decoded from the real protobuf export — hex-encoded IDs so they
// compare and print the same way the "traceparent" header's own hex
// encoding does.
type receivedSpan struct {
	name         string
	kind         tracepb.Span_SpanKind
	traceID      string
	spanID       string
	parentSpanID string
}

// decodingOTLPCollector is an httptest.Server standing in for a real OTLP/
// HTTP collector, except it actually decodes every export it receives
// (real protobuf, go.opentelemetry.io/proto/otlp's own generated types —
// the same wire format otlptracehttp really sends) into spans, instead of
// just counting requests the way TestInitTracing_EnabledExportsRealSpansOverOTLP's
// collector does. Safe for concurrent export batches: every access to
// spans is mutex-guarded.
type decodingOTLPCollector struct {
	*httptest.Server
	mu    sync.Mutex
	spans []receivedSpan
}

func newDecodingOTLPCollector(t *testing.T) *decodingOTLPCollector {
	t.Helper()
	c := &decodingOTLPCollector{}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		var req coltracepb.ExportTraceServiceRequest
		if err := proto.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		c.mu.Lock()
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, span := range ss.Spans {
					c.spans = append(c.spans, receivedSpan{
						name:         span.Name,
						kind:         span.Kind,
						traceID:      hex.EncodeToString(span.TraceId),
						spanID:       hex.EncodeToString(span.SpanId),
						parentSpanID: hex.EncodeToString(span.ParentSpanId),
					})
				}
			}
		}
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.Server.Close)
	return c
}

// spansByKind returns every decoded span received so far, snapshotted
// under the lock so a caller can range over the result safely after the
// collector has stopped receiving new exports (e.g. after
// InitTracing's shutdown has flushed the batch exporter).
func (c *decodingOTLPCollector) spansByKind() []receivedSpan {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]receivedSpan(nil), c.spans...)
}

// TestGatewayTracing_DistributedSpanLinkageOverRealOTLP is the repeatable
// version of the manual, ad hoc verification this feature got when it was
// first asked "is otel actually passing the right headers to routes and
// back": a real gateway, a real proxy route, a real backend, and a real
// (if fake) OTLP collector that decodes the actual protobuf export — not
// just "a traceparent header showed up" (see
// TestGatewayPropagatesTraceContextToBackend for that half alone), but
// that the two spans a proxied request produces are genuinely linked into
// one distributed trace, and that the specific header the backend received
// names exactly the span that link depends on.
//
// Run it directly to see this for yourself:
//
//	go test ./gateway/... -run TestGatewayTracing_DistributedSpanLinkageOverRealOTLP -v
func TestGatewayTracing_DistributedSpanLinkageOverRealOTLP(t *testing.T) {
	collector := newDecodingOTLPCollector(t)

	shutdown, err := InitTracing(context.Background(), config.TracingConfig{
		Enabled:  true,
		Endpoint: collector.Listener.Addr().String(),
		Insecure: true,
	}, "test-service")
	require.NoError(t, err)

	var receivedTraceparent string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedTraceparent = r.Header.Get("traceparent")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(backend.Close)

	gwConfig := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "localhost", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin"},
		Tracing:    config.TracingConfig{Enabled: true, Endpoint: collector.Listener.Addr().String(), Insecure: true},
		Routes: []config.RouteConfig{
			{
				Name:           "TracedProxy",
				From:           "/proxy",
				To:             []string{backend.URL},
				Authentication: config.AuthenticationConfig{Enabled: false},
			},
		},
	}

	gw, err := NewGatewayWithDependencies(gwConfig, nil, deps.NewTest())
	require.NoError(t, err)

	listener, err := net.Listen("tcp", gw.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	serverURL := fmt.Sprintf("http://localhost:%d", port)

	go func() { _ = gw.Server.Serve(listener) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = gw.Server.Shutdown(ctx)
	})
	time.Sleep(50 * time.Millisecond)

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(serverURL + "/proxy")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NotEmpty(t, receivedTraceparent, "the proxied request must carry a traceparent header to the backend")

	// Flush every pending span through the batch exporter before
	// inspecting what the collector actually decoded — see
	// TestInitTracing_EnabledExportsRealSpansOverOTLP's comment on why
	// Shutdown, not a sleep, is what makes this deterministic rather than
	// racing a background export timer.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, shutdown(shutdownCtx))

	spans := collector.spansByKind()
	var serverSpan, clientSpan *receivedSpan
	for i := range spans {
		switch spans[i].kind {
		case tracepb.Span_SPAN_KIND_SERVER:
			serverSpan = &spans[i]
		case tracepb.Span_SPAN_KIND_CLIENT:
			clientSpan = &spans[i]
		}
	}
	require.NotNil(t, serverSpan, "expected a SERVER span for the inbound gateway request, got: %+v", spans)
	require.NotNil(t, clientSpan, "expected a CLIENT span for the outbound proxy call, got: %+v", spans)

	assert.Equal(t, serverSpan.traceID, clientSpan.traceID,
		"the inbound request and the outbound proxy call must share one trace ID, not two disconnected traces")
	assert.Equal(t, serverSpan.spanID, clientSpan.parentSpanID,
		"the outbound proxy call's span must be a CHILD of the inbound request's span")

	// The header that actually reached the backend must name exactly the
	// client span — proving the propagation isn't just "some traceparent
	// value," it's specifically the one for the span this trace just
	// linked as a child of the inbound request.
	wantTraceparent := fmt.Sprintf("00-%s-%s-01", clientSpan.traceID, clientSpan.spanID)
	assert.Equal(t, wantTraceparent, receivedTraceparent)
}
