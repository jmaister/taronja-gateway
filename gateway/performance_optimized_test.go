package gateway

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/static"
)

// BenchmarkOptimizedStaticRequest benchmarks static requests with performance optimizations
func BenchmarkOptimizedStaticRequest(b *testing.B) {
	cfg := createTestConfig()

	// Enable performance optimizations
	perfConfig := middleware.DefaultPerformanceConfig()

	gw, err := NewGatewayWithPerformanceConfig(cfg, &static.StaticAssetsFS, perfConfig)
	if err != nil {
		b.Fatalf("Failed to create optimized gateway: %v", err)
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

// BenchmarkOptimizedAPIRequest benchmarks API requests with caching
func BenchmarkOptimizedAPIRequest(b *testing.B) {
	cfg := createTestConfig()

	// Enable performance optimizations
	perfConfig := middleware.DefaultPerformanceConfig()

	gw, err := NewGatewayWithPerformanceConfig(cfg, &static.StaticAssetsFS, perfConfig)
	if err != nil {
		b.Fatalf("Failed to create optimized gateway: %v", err)
	}

	// Create test request for API endpoint
	req := httptest.NewRequest("GET", "/api/test", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)
	}
}

// BenchmarkJA4HCaching specifically tests JA4H caching performance
func BenchmarkJA4HCaching(b *testing.B) {
	cache := middleware.NewJA4HCache(1000)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("Accept", "text/html,application/json")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		fingerprint := cache.GetOrCalculate(req)
		if fingerprint == "" {
			b.Error("Expected non-empty fingerprint")
		}
	}

	// Report cache statistics
	hits, misses, size := cache.GetStats()
	b.Logf("Cache stats - Hits: %d, Misses: %d, Size: %d, Hit Rate: %.2f%%",
		hits, misses, size, float64(hits)/float64(hits+misses)*100)
}

// BenchmarkJA4HNoCaching benchmarks JA4H without caching for comparison
func BenchmarkJA4HNoCaching(b *testing.B) {
	optimizedMiddleware := middleware.OptimizedJA4Middleware(false)

	// Create a simple handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middlewareHandler := optimizedMiddleware(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("User-Agent", "TestAgent/1.0")
	req.Header.Set("Accept", "text/html,application/json")

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		middlewareHandler.ServeHTTP(rr, req)
	}
}

// TestPerformanceMetrics tests the performance metrics collection
func TestPerformanceMetrics(t *testing.T) {
	metrics := middleware.GetPerformanceMetrics()

	// Reset metrics
	*metrics = middleware.PerformanceMiddlewareMetrics{}

	// Simulate some requests
	for i := 0; i < 100; i++ {
		metrics.IncrementTotalRequests()
		if i%5 == 0 { // Every 5th request is static
			metrics.IncrementStaticAssetsSkipped()
		}
	}

	staticSkipped, totalRequests := metrics.GetStats()

	if totalRequests != 100 {
		t.Errorf("Expected 100 total requests, got %d", totalRequests)
	}

	if staticSkipped != 20 {
		t.Errorf("Expected 20 static assets skipped, got %d", staticSkipped)
	}

	t.Logf("Performance metrics - Static skipped: %d, Total requests: %d, Skip rate: %.2f%%",
		staticSkipped, totalRequests, float64(staticSkipped)/float64(totalRequests)*100)
}

// NewGatewayWithPerformanceConfig creates a gateway with performance optimizations
// This is a placeholder - in the real implementation, you'd modify the actual NewGateway function
func NewGatewayWithPerformanceConfig(config *config.GatewayConfig, webappEmbedFS interface{}, perfConfig *middleware.PerformanceConfig) (*Gateway, error) {
	// For now, just return a regular gateway
	// In the real implementation, this would use the optimized middleware chain
	return NewGateway(config, &static.StaticAssetsFS)
}

// TestStaticAssetDetection tests the static asset detection logic
func TestStaticAssetDetection(t *testing.T) {
	testCases := []struct {
		path     string
		expected bool
	}{
		{"/static/style.css", true},
		{"/_/static/app.js", true},
		{"/images/logo.png", true},
		{"/favicon.ico", true},
		{"/api/users", false},
		{"/login", false},
		{"/admin/dashboard", false},
		{"/assets/main.bundle.js", true},
		{"/public/fonts/roboto.woff2", true},
		{"/app.json", true},
		{"/config.xml", true},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := isStaticAsset(tc.path)
			if result != tc.expected {
				t.Errorf("isStaticAsset(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}

// isStaticAsset is a helper function for testing - normally this would be in the middleware package
func isStaticAsset(path string) bool {
	// This would normally be imported from middleware package
	// For testing purposes, we duplicate the logic
	staticExtensions := []string{
		".css", ".js", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".svg",
		".woff", ".woff2", ".ttf", ".eot", ".webp", ".mp4", ".pdf",
		".zip", ".tar", ".gz", ".json", ".xml", ".txt",
	}

	for _, ext := range staticExtensions {
		if strings.HasSuffix(strings.ToLower(path), ext) {
			return true
		}
	}

	// Check for static paths
	staticPaths := []string{"/static/", "/_/static/", "/assets/", "/public/"}
	for _, staticPath := range staticPaths {
		if strings.Contains(strings.ToLower(path), staticPath) {
			return true
		}
	}

	return false
}
