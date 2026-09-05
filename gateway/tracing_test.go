package gateway

import (
	"context"
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
