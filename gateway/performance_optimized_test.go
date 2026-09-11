package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/jmaister/taronja-gateway/static"
)

// BenchmarkStaticRequestAnalyticsIncludingStatic benchmarks a static-asset
// request through the real full middleware chain with
// management.excludeStaticAssets left at its default (false): traffic
// metrics are still recorded for every request, static assets included.
// Compare against BenchmarkStaticRequestAnalyticsExcludingStatic below —
// same request, same chain, only that one flag differs.
func BenchmarkStaticRequestAnalyticsIncludingStatic(b *testing.B) {
	cfg := createTestConfig()
	cfg.Management.ExcludeStaticAssets = false
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	req := httptest.NewRequest("GET", "/_/static/style.css", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.handler.ServeHTTP(rr, req)
	}
}

// BenchmarkStaticRequestAnalyticsExcludingStatic is
// BenchmarkStaticRequestAnalyticsIncludingStatic with
// management.excludeStaticAssets: true — TrafficMetricMiddleware skips its
// response-writer wrapping, TrafficMetric construction, and async DB write
// for this request because session.IsStaticAssetPath("/_/static/style.css")
// is true. See PERFORMANCE_ANALYSIS.md for the measured before/after.
func BenchmarkStaticRequestAnalyticsExcludingStatic(b *testing.B) {
	cfg := createTestConfig()
	cfg.Management.ExcludeStaticAssets = true
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	req := httptest.NewRequest("GET", "/_/static/style.css", nil)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.handler.ServeHTTP(rr, req)
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

// TestStaticAssetDetection tests session.IsStaticAssetPath, the heuristic
// management.excludeStaticAssets and the request-details report filter both
// rely on to decide whether a path is a static asset.
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
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := session.IsStaticAssetPath(tc.path)
			if result != tc.expected {
				t.Errorf("IsStaticAssetPath(%q) = %v, expected %v", tc.path, result, tc.expected)
			}
		})
	}
}
