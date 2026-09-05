package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

// --- Health checks ---

func TestGetHealth_UnknownMiddlewareReturnsFalse(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	if _, ok := registry.GetHealth("does_not_exist"); ok {
		t.Fatal("expected ok=false for an unregistered middleware name")
	}
}

func TestGetHealth_FactoryWithoutHealthCheckerReturnsUnknown(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewLoggingFactory())

	health, ok := registry.GetHealth(config.MiddlewareNameLogging)
	if !ok {
		t.Fatal("expected ok=true for a registered middleware")
	}
	if health.Status != "unknown" {
		t.Fatalf("expected status 'unknown' for a factory with no HealthChecker, got %+v", health)
	}
}

func TestRateLimiterFactory_HealthCheck_NoInstance(t *testing.T) {
	f := NewRateLimiterFactory(nil)
	health := f.HealthCheck()
	if health.Status != "unknown" {
		t.Fatalf("expected 'unknown' with no rate limiter instance, got %+v", health)
	}
}

func TestRateLimiterFactory_HealthCheck_WithInstance(t *testing.T) {
	rl := NewRateLimiter(config.RateLimiterConfig{RequestsPerMinute: 100, MaxErrors: 5, BlockMinutes: 5}, nil)
	f := NewRateLimiterFactory(rl)
	health := f.HealthCheck()
	if health.Status != "healthy" {
		t.Fatalf("expected 'healthy' with a real rate limiter instance, got %+v", health)
	}
	if health.Message == "" {
		t.Fatal("expected a non-empty message describing rate limiter state")
	}
}

func TestGetStatus_IncludesHealthWhenImplemented(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewRateLimiterFactory(nil))
	registry.RegisterFactory(NewLoggingFactory())

	status := registry.GetStatus()

	rl := status[config.MiddlewareNameRateLimiter]
	if rl.Health == nil {
		t.Fatal("expected rate_limiter status to include Health (it implements HealthChecker)")
	}

	logging := status[config.MiddlewareNameLogging]
	if logging.Health != nil {
		t.Fatalf("expected logging status to have nil Health (no HealthChecker), got %+v", logging.Health)
	}
}

// --- Metrics ---

func TestGetMetrics_UnknownMiddlewareErrors(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	if _, err := registry.GetMetrics("does_not_exist"); err == nil {
		t.Fatal("expected an error for an unregistered middleware name")
	}
}

func TestGetMetrics_KnownButNeverBuiltReturnsZeroSnapshot(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewLoggingFactory())

	snap, err := registry.GetMetrics(config.MiddlewareNameLogging)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.RequestCount != 0 || snap.LastInvokedAt != nil {
		t.Fatalf("expected a zero-valued snapshot, got %+v", snap)
	}
}

func TestMetrics_RecordedAcrossRequests(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewLoggingFactory())

	chain, err := registry.BuildChain([]MiddlewareSpec{{Name: config.MiddlewareNameLogging}})
	if err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/ping", nil)
		rw := httptest.NewRecorder()
		handler.ServeHTTP(rw, req)
	}

	snap, err := registry.GetMetrics(config.MiddlewareNameLogging)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.RequestCount != 3 {
		t.Fatalf("expected 3 recorded requests, got %d", snap.RequestCount)
	}
	if snap.ErrorCount != 0 {
		t.Fatalf("expected 0 errors for 200 responses, got %d", snap.ErrorCount)
	}
	if snap.LastInvokedAt == nil {
		t.Fatal("expected LastInvokedAt to be set after requests were served")
	}
}

func TestMetrics_CountsServerErrors(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewLoggingFactory())

	chain, err := registry.BuildChain([]MiddlewareSpec{{Name: config.MiddlewareNameLogging}})
	if err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	snap, err := registry.GetMetrics(config.MiddlewareNameLogging)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if snap.RequestCount != 1 || snap.ErrorCount != 1 {
		t.Fatalf("expected 1 request and 1 error, got %+v", snap)
	}
}

func TestGetAllMetrics_OnlyIncludesBuiltMiddleware(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewLoggingFactory())
	registry.RegisterFactory(NewRateLimiterFactory(nil))

	if _, err := registry.BuildChain([]MiddlewareSpec{{Name: config.MiddlewareNameLogging}}); err != nil {
		t.Fatalf("BuildChain failed: %v", err)
	}

	all := registry.GetAllMetrics()
	if _, ok := all[config.MiddlewareNameLogging]; !ok {
		t.Fatal("expected logging to be present (it was built)")
	}
	if _, ok := all[config.MiddlewareNameRateLimiter]; ok {
		t.Fatal("expected rate_limiter to be absent (it was never built into a chain)")
	}
}

func TestInstrumentMiddleware_PassesRequestThrough(t *testing.T) {
	counter := &middlewareMetricsCounter{}
	var identity Middleware = func(next http.Handler) http.Handler { return next }
	wrapped := instrumentMiddleware(identity, counter)

	called := false
	handler := wrapped(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("expected the wrapped handler to run")
	}
	if rw.Code != http.StatusTeapot {
		t.Fatalf("expected status to pass through unchanged, got %d", rw.Code)
	}

	snap := counter.snapshot("test")
	if snap.RequestCount != 1 {
		t.Fatalf("expected 1 recorded request, got %d", snap.RequestCount)
	}
}

func TestMiddlewareMetricsCounter_AverageDuration(t *testing.T) {
	c := &middlewareMetricsCounter{}
	c.record(http.StatusOK, 10*time.Millisecond)
	c.record(http.StatusOK, 30*time.Millisecond)

	snap := c.snapshot("test")
	if snap.RequestCount != 2 {
		t.Fatalf("expected 2 requests, got %d", snap.RequestCount)
	}
	if snap.AverageDurationMs < 19.9 || snap.AverageDurationMs > 20.1 {
		t.Fatalf("expected average duration ~20ms, got %f", snap.AverageDurationMs)
	}
}
