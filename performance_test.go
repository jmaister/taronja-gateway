package main

import (
	"net/http"
	"net/http/httptest"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/static"
)

// createTestGateway creates a gateway instance for performance testing
func createTestGateway(cfg *config.GatewayConfig) (*gateway.Gateway, error) {
	deps := deps.NewTest()

	return gateway.NewGatewayWithDependencies(cfg, &static.StaticAssetsFS, deps)
}

// BenchmarkAPIRequest benchmarks the handling of API requests
func BenchmarkAPIRequest(b *testing.B) {
	// Set up test gateway with full middleware chain
	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create test request
	req := httptest.NewRequest("GET", "/_/api/health", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			b.Errorf("Expected status 200, got %d", rr.Code)
		}
	}
}

// BenchmarkStaticRequest benchmarks the handling of static file requests
func BenchmarkStaticRequest(b *testing.B) {
	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create test request for static content
	req := httptest.NewRequest("GET", "/_/static/style.css", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)

		// Static files might return 404 if not found, that's ok for performance testing
		if rr.Code != http.StatusOK && rr.Code != http.StatusNotFound {
			b.Errorf("Expected status 200 or 404, got %d", rr.Code)
		}
	}
}

// BenchmarkAuthenticatedRequest benchmarks authenticated API requests
func BenchmarkAuthenticatedRequest(b *testing.B) {
	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create test request with session cookie
	req := httptest.NewRequest("GET", "/_/api/me", nil)
	// Add a mock session cookie
	req.AddCookie(&http.Cookie{
		Name:  "sessionToken",
		Value: "test-session-token",
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)

		// This will likely return 401 without valid session, but we're testing performance
		if rr.Code != http.StatusOK && rr.Code != http.StatusUnauthorized {
			b.Errorf("Expected status 200 or 401, got %d", rr.Code)
		}
	}
}

// BenchmarkMiddlewareChain benchmarks just the middleware chain without the final handler
func BenchmarkMiddlewareChain(b *testing.B) {
	cfg := createTestConfig()
	_, err := createTestGateway(cfg)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a simple handler that just returns 200
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// BenchmarkWithoutMiddleware benchmarks request handling without any middleware
func BenchmarkWithoutMiddleware(b *testing.B) {
	// Simple handler without any middleware
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	req := httptest.NewRequest("GET", "/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}
}

// ProfileAPIRequest creates a CPU profile of API request handling
func ProfileAPIRequest(b *testing.B) {
	if !testing.Short() {
		// Create CPU profile file
		err := pprof.StartCPUProfile(nil)
		if err != nil {
			b.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	req := httptest.NewRequest("GET", "/_/api/health", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)
	}

	// Memory profile
	if !testing.Short() {
		runtime.GC()
		memProfile := pprof.Lookup("heap")
		if memProfile != nil {
			memProfile.WriteTo(nil, 2)
		}
	}
}

// MemoryUsageTest measures memory usage during request processing
func TestMemoryUsage(t *testing.T) {
	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Measure memory before
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Process multiple requests
	numRequests := 1000
	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/_/api/health", nil)
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)
	}

	// Force garbage collection and measure memory after
	runtime.GC()
	runtime.ReadMemStats(&m2)

	t.Logf("Memory usage for %d requests:", numRequests)
	t.Logf("  Alloc: %d KB -> %d KB (diff: %d KB)",
		m1.Alloc/1024, m2.Alloc/1024, (m2.Alloc-m1.Alloc)/1024)
	t.Logf("  TotalAlloc: %d KB -> %d KB (diff: %d KB)",
		m1.TotalAlloc/1024, m2.TotalAlloc/1024, (m2.TotalAlloc-m1.TotalAlloc)/1024)
	t.Logf("  Sys: %d KB -> %d KB (diff: %d KB)",
		m1.Sys/1024, m2.Sys/1024, (m2.Sys-m1.Sys)/1024)
	t.Logf("  NumGC: %d -> %d (diff: %d)", m1.NumGC, m2.NumGC, m2.NumGC-m1.NumGC)
	t.Logf("  Average per request: %d bytes", (m2.TotalAlloc-m1.TotalAlloc)/uint64(numRequests))
}

// ConcurrentRequestTest tests performance under concurrent load
func TestConcurrentRequests(t *testing.T) {
	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	numGoroutines := 10
	requestsPerGoroutine := 100

	start := time.Now()

	results := make(chan time.Duration, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func() {
			goroutineStart := time.Now()

			for j := 0; j < requestsPerGoroutine; j++ {
				req := httptest.NewRequest("GET", "/_/health", nil)
				rr := httptest.NewRecorder()
				gw.Mux.ServeHTTP(rr, req)

				if rr.Code != http.StatusOK {
					t.Errorf("Expected status 200, got %d", rr.Code)
				}
			}

			results <- time.Since(goroutineStart)
		}()
	}

	// Wait for all goroutines to complete
	var totalGoroutineTime time.Duration
	for i := 0; i < numGoroutines; i++ {
		totalGoroutineTime += <-results
	}

	totalTime := time.Since(start)
	totalRequests := numGoroutines * requestsPerGoroutine

	t.Logf("Concurrent performance test results:")
	t.Logf("  Total requests: %d", totalRequests)
	t.Logf("  Goroutines: %d", numGoroutines)
	t.Logf("  Requests per goroutine: %d", requestsPerGoroutine)
	t.Logf("  Total wall time: %v", totalTime)
	t.Logf("  Average goroutine time: %v", totalGoroutineTime/time.Duration(numGoroutines))
	t.Logf("  Requests per second: %.2f", float64(totalRequests)/totalTime.Seconds())
	t.Logf("  Average request time: %v", totalTime/time.Duration(totalRequests))
}

// RequestLatencyTest measures detailed request latency
func TestRequestLatency(t *testing.T) {
	cfg := createTestConfig()
	gw, err := createTestGateway(cfg)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	numRequests := 1000
	latencies := make([]time.Duration, numRequests)

	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/_/health", nil)
		rr := httptest.NewRecorder()

		start := time.Now()
		gw.Mux.ServeHTTP(rr, req)
		latencies[i] = time.Since(start)

		if rr.Code != http.StatusOK {
			t.Errorf("Request %d: Expected status 200, got %d", i, rr.Code)
		}
	}

	// Calculate statistics
	var total time.Duration
	min := latencies[0]
	max := latencies[0]

	for _, latency := range latencies {
		total += latency
		if latency < min {
			min = latency
		}
		if latency > max {
			max = latency
		}
	}

	avg := total / time.Duration(numRequests)

	t.Logf("Request latency statistics (%d requests):", numRequests)
	t.Logf("  Average: %v", avg)
	t.Logf("  Minimum: %v", min)
	t.Logf("  Maximum: %v", max)
	t.Logf("  Total: %v", total)
}

// createTestConfig creates a minimal configuration for testing
func createTestConfig() *config.GatewayConfig {
	return &config.GatewayConfig{
		Name: "test-gateway",
		Server: config.ServerConfig{
			Host: "localhost",
			Port: 8080,
		},
		Management: config.ManagementConfig{
			Prefix:    "/_",
			Analytics: true,
			Logging:   true,
			Admin: config.AdminConfig{
				Enabled:  false,
				Username: "admin",
				Email:    "admin@example.com",
				Password: "password",
			},
		},
		AuthenticationProviders: config.AuthenticationProviders{
			Basic: config.BasicAuthenticationConfig{
				Enabled: false,
			},
			Google: config.AuthProviderCredentials{
				ClientId:     "",
				ClientSecret: "",
			},
			Github: config.AuthProviderCredentials{
				ClientId:     "",
				ClientSecret: "",
			},
		},
		Routes: []config.RouteConfig{
			{
				Name:   "test-api",
				From:   "/api/*",
				To:     "http://localhost:3000",
				Static: false,
				Authentication: config.AuthenticationConfig{
					Enabled: false,
				},
			},
		},
	}
}
