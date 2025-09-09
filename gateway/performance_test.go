package gateway

import (
	"embed"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/static"
)

var testBackendServerURL string

// NewTestGateway creates a gateway instance for testing with silent database logging
func NewTestGateway(config *config.GatewayConfig, webappEmbedFS *embed.FS) (*Gateway, error) {
	// Initialize the database connection for tests (with silent logging)
	db.InitForTest()

	// Use the normal gateway creation process
	return NewGateway(config, webappEmbedFS)
}

// createTestSession creates a test session for authenticated benchmarks
func createTestSession(gw *Gateway) *db.Session {
	session := &db.Session{
		Token:           "test-session-token",
		UserID:          "1",
		Username:        "testuser",
		Email:           "test@example.com",
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(time.Hour),
		Provider:        "test-provider",
	}

	// Create a new session repository directly
	sessionRepo := db.NewSessionRepositoryDB()
	err := sessionRepo.CreateSession(session.Token, session)
	if err != nil {
		panic("Failed to create test session: " + err.Error())
	}

	return session
}

// BenchmarkAPIRequest benchmarks the handling of API requests
func BenchmarkAPIRequest(b *testing.B) {
	// Disable logging for cleaner benchmark output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	// Set up test gateway with full middleware chain
	cfg := createTestConfig()
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create test request
	req := httptest.NewRequest("GET", "/_/health", nil)

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
	// Disable logging for cleaner benchmark output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	cfg := createTestConfig()
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create test request for static content
	req := httptest.NewRequest("GET", "/_/login", nil)

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

// BenchmarkAuthenticatedRequest benchmarks requests with authentication middleware
func BenchmarkAuthenticatedRequest(b *testing.B) {
	// Disable logging for cleaner benchmark output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	cfg := createTestConfig()
	cfg.Routes[0].Authentication.Enabled = true // Enable authentication for this route
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	// Create a test session first
	session := createTestSession(gw)

	// Create test request with session cookie
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  "tg_session_token",
		Value: session.Token,
	})

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)

		// Should get 200 (mock backend returns OK)
		if rr.Code != http.StatusOK {
			b.Errorf("Expected status 200 (mock backend), got %d", rr.Code)
		}
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

// BenchmarkProfileAPIRequest creates a CPU profile of API request handling
func BenchmarkProfileAPIRequest(b *testing.B) {
	// Disable logging for cleaner benchmark output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	var profileFile *os.File
	if !testing.Short() {
		var err error
		profileFile, err = os.Create("api_request_profile.prof")
		if err != nil {
			b.Fatal(err)
		}
		defer profileFile.Close()

		err = pprof.StartCPUProfile(profileFile)
		if err != nil {
			b.Fatal(err)
		}
		defer pprof.StopCPUProfile()
	}

	cfg := createTestConfig()
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		b.Fatalf("Failed to create gateway: %v", err)
	}

	req := httptest.NewRequest("GET", "/_/health", nil)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rr := httptest.NewRecorder()
		gw.Mux.ServeHTTP(rr, req)
	}

	// Memory profile
	if !testing.Short() && profileFile != nil {
		runtime.GC()
		memProfile := pprof.Lookup("heap")
		if memProfile != nil {
			memProfile.WriteTo(profileFile, 2)
		}
	}
}

// TestMemoryUsage measures memory usage during request processing
func TestMemoryUsage(t *testing.T) {
	// Disable logging for cleaner test output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	cfg := createTestConfig()
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
	if err != nil {
		t.Fatalf("Failed to create gateway: %v", err)
	}

	// Measure memory before
	var m1, m2 runtime.MemStats
	runtime.ReadMemStats(&m1)

	// Process multiple requests
	numRequests := 1000
	for i := 0; i < numRequests; i++ {
		req := httptest.NewRequest("GET", "/_/health", nil)
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

// TestConcurrentRequests tests performance under concurrent load
func TestConcurrentRequests(t *testing.T) {
	// Disable logging for cleaner test output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	cfg := createTestConfig()
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
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

// TestRequestLatency measures detailed request latency
func TestRequestLatency(t *testing.T) {
	// Disable logging for cleaner test output
	originalOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(originalOutput)

	cfg := createTestConfig()
	gw, err := NewTestGateway(cfg, &static.StaticAssetsFS)
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

func TestMain(m *testing.M) {
	// Start a local backend server to mock responses
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))
	testBackendServerURL = backend.URL
	code := m.Run()
	backend.Close()
	os.Exit(code)
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
				To:     testBackendServerURL,
				Static: false,
				Authentication: config.AuthenticationConfig{
					Enabled: false,
				},
			},
		},
	}
}
