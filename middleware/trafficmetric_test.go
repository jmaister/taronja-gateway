package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrafficMetricMiddleware(t *testing.T) {
	t.Run("records successful request metrics", func(t *testing.T) {
		// Create memory repository for testing
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		// Create a simple handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Success response"))
		})

		// Wrap handler with middleware
		wrappedHandler := middleware(handler)

		// Create test request
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:8080"
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		// Execute request
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation to complete
		time.Sleep(15 * time.Millisecond)

		// Verify response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Success response", w.Body.String())

		// Verify metrics were recorded
		stats, err := statsRepo.FindByPath("/api/test", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "GET", stat.HttpMethod)
		assert.Equal(t, "/api/test", stat.Path)
		assert.Equal(t, 200, stat.HttpStatus)
		assert.GreaterOrEqual(t, stat.ResponseTimeNs, int64(0))
		assert.Equal(t, int64(16), stat.ResponseSize) // "Success response" is 16 bytes
		assert.Equal(t, "192.168.1.100", stat.IPAddress)
		assert.Equal(t, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36", stat.UserAgent)
		assert.Equal(t, "Other", stat.DeviceFamily) // Desktop browsers are classified as "Other"
		assert.Equal(t, "Chrome", stat.BrowserFamily)
		assert.Equal(t, "Windows", stat.OSFamily)
		assert.NotEmpty(t, stat.BrowserVersion) // Should have a version
		assert.NotEmpty(t, stat.OSVersion)      // Should have a version
		assert.Empty(t, stat.Error)
		assert.Empty(t, stat.UserID)
		assert.Empty(t, stat.SessionID)
	})

	t.Run("records error request metrics", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		// Create a handler that returns an error
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("Invalid request format"))
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("POST", "/api/create", nil)
		req.RemoteAddr = "10.0.0.1:9090"
		req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36")

		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify error response
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Verify error metrics were recorded
		stats, err := statsRepo.FindByPath("/api/create", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "POST", stat.HttpMethod)
		assert.Equal(t, "/api/create", stat.Path)
		assert.Equal(t, 400, stat.HttpStatus)
		assert.Equal(t, "Invalid request format", stat.Error)
		assert.Equal(t, "10.0.0.1", stat.IPAddress)
		// Device info should be populated even for error requests
		assert.NotEmpty(t, stat.DeviceFamily)
		assert.NotEmpty(t, stat.BrowserFamily)
		assert.NotEmpty(t, stat.OSFamily)
	})

	t.Run("records session information when available", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		// Create handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Authenticated response"))
		})

		wrappedHandler := middleware(handler)

		// Create request with session context
		req := httptest.NewRequest("GET", "/api/profile", nil)
		sessionData := &db.Session{
			Token:  "session-token-123",
			UserID: "user-456",
		}
		ctx := context.WithValue(req.Context(), session.SessionKey, sessionData)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify session info was recorded
		stats, err := statsRepo.FindByPath("/api/profile", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "user-456", stat.UserID)
		assert.Equal(t, "session-token-123", stat.SessionID)
	})

	t.Run("handles long error messages by truncating", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		// Create a handler that returns a very long error message
		longErrorMsg := make([]byte, 1200) // Longer than 1000 byte limit
		for i := range longErrorMsg {
			longErrorMsg[i] = 'x'
		}

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(longErrorMsg)
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/api/error", nil)
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify error message was truncated
		stats, err := statsRepo.FindByPath("/api/error", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, 500, stat.HttpStatus)
		assert.Equal(t, 1003, len(stat.Error)) // 1000 chars + "..."
		assert.Contains(t, stat.Error, "...")
	})

	t.Run("measures response time accurately", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		// Create a handler that sleeps to create measurable response time
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(5 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Delayed response"))
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/api/slow", nil)
		w := httptest.NewRecorder()

		startTime := time.Now()
		wrappedHandler.ServeHTTP(w, req)
		actualDuration := time.Since(startTime)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify response time was measured
		stats, err := statsRepo.FindByPath("/api/slow", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.GreaterOrEqual(t, stat.ResponseTimeNs, int64(4000000))                     // Should be at least 4ms (4000000 ns)
		assert.LessOrEqual(t, stat.ResponseTimeNs, actualDuration.Nanoseconds()+10000000) // Allow some margin (10ms in ns)
	})

	t.Run("handles nil session gracefully", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/api/public", nil)
		// Add nil session to context
		ctx := context.WithValue(req.Context(), session.SessionKey, (*db.Session)(nil))
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify metrics were recorded without session info
		stats, err := statsRepo.FindByPath("/api/public", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Empty(t, stat.UserID)
		assert.Empty(t, stat.SessionID)
	})

	t.Run("records mobile device information correctly", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Mobile response"))
		})

		wrappedHandler := middleware(handler)

		// Create request with mobile user agent
		req := httptest.NewRequest("GET", "/api/mobile", nil)
		req.RemoteAddr = "203.0.113.50:443"
		req.Header.Set("User-Agent", "Mozilla/5.0 (iPhone; CPU iPhone OS 14_7_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.2 Mobile/15E148 Safari/604.1")

		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify mobile device info was recorded
		stats, err := statsRepo.FindByPath("/api/mobile", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "GET", stat.HttpMethod)
		assert.Equal(t, "/api/mobile", stat.Path)
		assert.Equal(t, 200, stat.HttpStatus)
		assert.Equal(t, "203.0.113.50", stat.IPAddress)
		assert.Equal(t, "Mozilla/5.0 (iPhone; CPU iPhone OS 14_7_1 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/14.1.2 Mobile/15E148 Safari/604.1", stat.UserAgent)
		assert.Equal(t, "iPhone", stat.DeviceFamily) // Should detect iPhone
		assert.Equal(t, "Mobile Safari", stat.BrowserFamily)
		assert.Equal(t, "iOS", stat.OSFamily)
		assert.NotEmpty(t, stat.BrowserVersion) // Should have a version
		assert.NotEmpty(t, stat.OSVersion)      // Should have a version
	})

	t.Run("records Android device information correctly", func(t *testing.T) {
		statsRepo := db.NewMemoryTrafficMetricRepository()
		middleware := TrafficMetricMiddleware(statsRepo)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Android response"))
		})

		wrappedHandler := middleware(handler)

		// Create request with Android user agent (Samsung Galaxy)
		req := httptest.NewRequest("GET", "/api/android", nil)
		req.RemoteAddr = "203.0.113.75:443"
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 11; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36")

		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Wait for async operation
		time.Sleep(15 * time.Millisecond)

		// Verify Android device info was recorded
		stats, err := statsRepo.FindByPath("/api/android", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "GET", stat.HttpMethod)
		assert.Equal(t, "/api/android", stat.Path)
		assert.Equal(t, 200, stat.HttpStatus)
		assert.Equal(t, "203.0.113.75", stat.IPAddress)
		assert.Equal(t, "Mozilla/5.0 (Linux; Android 11; SM-G991B) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.120 Mobile Safari/537.36", stat.UserAgent)
		assert.Equal(t, "Samsung SM-G991B", stat.DeviceFamily)
		assert.Equal(t, "Chrome Mobile", stat.BrowserFamily)
		assert.Equal(t, "Android", stat.OSFamily)
		assert.Equal(t, "Samsung", stat.DeviceBrand)
		assert.Equal(t, "SM-G991B", stat.DeviceModel)
		assert.NotEmpty(t, stat.BrowserVersion) // Should have a version
		assert.NotEmpty(t, stat.OSVersion)      // Should have a version
	})
}
