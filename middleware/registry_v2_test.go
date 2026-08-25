package middleware

import (
	"net/http"
	"net/http/httptest"
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
