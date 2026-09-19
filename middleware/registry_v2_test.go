package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
)

func TestRegistryBuildsMiddleware(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	if err := registry.RegisterFactory(NewRateLimiterFactory(nil)); err != nil {
		t.Fatalf("Failed to register rate_limiter factory: %v", err)
	}
	if err := registry.RegisterFactory(NewLoggingFactory()); err != nil {
		t.Fatalf("Failed to register logging factory: %v", err)
	}

	specs := []MiddlewareSpec{
		{Name: "rate_limiter", Config: config.RateLimiterConfig{RequestsPerMinute: 100, MaxErrors: 10, BlockMinutes: 5}},
		{Name: "logging"},
	}

	chain, err := registry.BuildChain(specs)
	if err != nil {
		t.Fatalf("Failed to build chain: %v", err)
	}
	if chain == nil {
		t.Fatal("Chain should not be nil")
	}

	// The built chain should actually be usable: wrap a simple handler and invoke it.
	called := false
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("Expected wrapped handler to be called")
	}
	if rw.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rw.Code)
	}
}

func TestRegistryRegisterFactoryRejectsDuplicates(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	if err := registry.RegisterFactory(NewLoggingFactory()); err != nil {
		t.Fatalf("First registration should succeed: %v", err)
	}
	if err := registry.RegisterFactory(NewLoggingFactory()); err == nil {
		t.Fatal("Expected error when registering duplicate factory name")
	}
}

func TestRegistryBuildChainUnknownMiddleware(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewLoggingFactory())

	specs := []MiddlewareSpec{
		{Name: "does_not_exist"},
	}

	_, err := registry.BuildChain(specs)
	if err == nil {
		t.Fatal("Should fail when middleware is not registered")
	}
}

func TestRegistryValidatesDependencies(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewSessionExtractionFactory(nil, nil))

	// Try to build session extraction without ja4_fingerprint being enabled first.
	specs := []MiddlewareSpec{
		{Name: "session_extraction"},
	}

	_, err := registry.BuildChain(specs)
	if err == nil {
		t.Fatal("Should fail when dependency is missing")
	}
}

func TestRegistryBuildChainSatisfiedDependencies(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewJA4Factory())
	registry.RegisterFactory(NewSessionExtractionFactory(nil, nil))
	registry.RegisterFactory(NewTrafficMetricsFactory(nil))

	specs := []MiddlewareSpec{
		{Name: "ja4_fingerprint"},
		{Name: "session_extraction"},
		{Name: "traffic_metrics"},
	}

	chain, err := registry.BuildChain(specs)
	if err != nil {
		t.Fatalf("Expected dependencies to be satisfied in order, got error: %v", err)
	}
	if chain == nil {
		t.Fatal("Chain should not be nil")
	}
}

func TestRegistryReportsStatus(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewRateLimiterFactory(nil))
	registry.RegisterFactory(NewLoggingFactory())

	// Nothing built yet: both should report as "available", not "active".
	status := registry.GetStatus()
	if len(status) != 2 {
		t.Fatalf("Expected 2 middleware, got %d", len(status))
	}
	rl, ok := status["rate_limiter"]
	if !ok {
		t.Fatal("rate_limiter should be in status")
	}
	if rl.Enabled || rl.Status != "available" {
		t.Fatalf("Expected rate_limiter to be available/disabled before building, got %+v", rl)
	}

	// Build a chain that only includes logging.
	if _, err := registry.BuildChain([]MiddlewareSpec{{Name: "logging"}}); err != nil {
		t.Fatalf("Failed to build chain: %v", err)
	}

	status = registry.GetStatus()
	logging := status["logging"]
	if !logging.Enabled || logging.Status != "active" {
		t.Fatalf("Expected logging to be active after building, got %+v", logging)
	}
	rl = status["rate_limiter"]
	if rl.Enabled || rl.Status != "available" {
		t.Fatalf("Expected rate_limiter to remain available/disabled, got %+v", rl)
	}
}

// TestRegistryStatusReflectsOnlyMostRecentBuild guards against a bug where
// r.built/r.metrics accumulated across multiple BuildChain calls on the same
// registry instead of being reset, so a middleware dropped from a later
// build kept reporting "active" from an earlier one — contradicting
// GetStatus's documented "included in the most recent BuildChain call"
// contract.
func TestRegistryStatusReflectsOnlyMostRecentBuild(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewRateLimiterFactory(nil))
	registry.RegisterFactory(NewLoggingFactory())

	if _, err := registry.BuildChain([]MiddlewareSpec{
		{Name: "rate_limiter", Config: config.RateLimiterConfig{RequestsPerMinute: 10}},
		{Name: "logging"},
	}); err != nil {
		t.Fatalf("first BuildChain failed: %v", err)
	}

	// Rebuild without rate_limiter (e.g. a config reload that dropped it).
	if _, err := registry.BuildChain([]MiddlewareSpec{{Name: "logging"}}); err != nil {
		t.Fatalf("second BuildChain failed: %v", err)
	}

	status := registry.GetStatus()
	rl := status["rate_limiter"]
	if rl.Enabled || rl.Status != "available" {
		t.Fatalf("Expected rate_limiter to report available/disabled after being dropped from the most recent build, got %+v", rl)
	}
	logging := status["logging"]
	if !logging.Enabled || logging.Status != "active" {
		t.Fatalf("Expected logging to remain active, got %+v", logging)
	}

	// Metrics for the dropped middleware should also not still be reported.
	if _, ok := registry.GetAllMetrics()["rate_limiter"]; ok {
		t.Fatal("Expected rate_limiter metrics to be cleared after being dropped from the most recent build")
	}
}

func TestBuildGlobalChainFromConfigV2MatchesConfig(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	registry.RegisterFactory(NewRateLimiterFactory(nil))
	registry.RegisterFactory(NewJA4Factory())
	registry.RegisterFactory(NewSessionExtractionFactory(nil, nil))
	registry.RegisterFactory(NewTrafficMetricsFactory(nil))
	registry.RegisterFactory(NewLoggingFactory())

	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Analytics = true
	gatewayConfig.Management.Logging = true

	chain, err := BuildGlobalChainFromConfigV2(registry, gatewayConfig)
	if err != nil {
		t.Fatalf("Failed to build chain from config: %v", err)
	}
	if chain == nil {
		t.Fatal("Chain should not be nil")
	}

	status := registry.GetStatus()
	for _, name := range []string{"ja4_fingerprint", "session_extraction", "traffic_metrics", "logging"} {
		if !status[name].Enabled {
			t.Fatalf("Expected %s to be active, got %+v", name, status[name])
		}
	}
	if status["rate_limiter"].Enabled {
		t.Fatalf("Expected rate_limiter to be disabled (not configured), got %+v", status["rate_limiter"])
	}
}

func TestBuildGlobalChainV2ProducesWorkingChain(t *testing.T) {
	gatewayConfig := &config.GatewayConfig{}
	gatewayConfig.Management.Logging = true

	chain, err := BuildGlobalChainV2(gatewayConfig, nil, nil, nil, nil)
	if err != nil {
		t.Fatalf("BuildGlobalChainV2 failed: %v", err)
	}

	called := false
	handler := chain.Build(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	if !called {
		t.Fatal("Expected wrapped handler to be called")
	}
}

// TestRegistry_ConcurrentBuildChainAndReads is the regression test for the
// registry's own documented contract: BuildChain's doc comment describes
// calling it again on an already-live registry (e.g. re-building one
// chain's specs in place) as supported, legitimate reuse — which means a
// concurrent GetStatus/GetMetrics/GetAllMetrics/GetHealth/Factories call
// (the admin dashboard's middleware-status/metrics endpoints) can
// genuinely race a BuildChain call on the very same registry, in
// production, not just in a synthetic test. Before factories/built/metrics
// were guarded by a mutex, this raced on Go's own unsynchronized
// concurrent-map-access detector — run with -race, this is exactly the
// scenario that would trip it; even without -race, concurrent map writes
// can panic outright ("fatal error: concurrent map read and map write").
func TestRegistry_ConcurrentBuildChainAndReads(t *testing.T) {
	registry := NewMiddlewareRegistryV2()
	if err := registry.RegisterFactory(NewRateLimiterFactory(nil)); err != nil {
		t.Fatalf("Failed to register rate_limiter factory: %v", err)
	}
	if err := registry.RegisterFactory(NewLoggingFactory()); err != nil {
		t.Fatalf("Failed to register logging factory: %v", err)
	}
	specs := []MiddlewareSpec{
		{Name: "rate_limiter", Config: config.RateLimiterConfig{RequestsPerMinute: 100, MaxErrors: 10, BlockMinutes: 5}},
		{Name: "logging"},
	}

	const goroutinesPerOp = 10
	var wg sync.WaitGroup
	start := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	for i := 0; i < goroutinesPerOp; i++ {
		start(func() {
			if _, err := registry.BuildChain(specs); err != nil {
				t.Errorf("BuildChain: %v", err)
			}
		})
		start(func() { registry.GetStatus() })
		start(func() { registry.GetAllMetrics() })
		start(func() { _, _ = registry.GetMetrics("logging") })
		start(func() { _, _ = registry.GetHealth("rate_limiter") })
		start(func() { registry.Factories() })
		start(func() { _ = registry.ValidateSpecs(specs) })
	}
	wg.Wait()
}
