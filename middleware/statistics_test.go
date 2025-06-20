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

func TestStatisticsMiddleware(t *testing.T) {
	statsRepo := db.NewMemoryTrafficMetricRepository()

	t.Run("records basic request statistics", func(t *testing.T) {
		middleware := TrafficMetricMiddleware(statsRepo)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, World!"))
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		req.Header.Set("User-Agent", "test-agent/1.0")

		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		// Wait a bit for the async operation to complete
		time.Sleep(10 * time.Millisecond)

		// Verify statistics were recorded
		stats, err := statsRepo.FindByPath("/test", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)
		stat := stats[0]
		assert.Equal(t, "GET", stat.HttpMethod)
		assert.Equal(t, "/test", stat.Path)
		assert.Equal(t, 200, stat.HttpStatus)
		assert.GreaterOrEqual(t, stat.ResponseTimeMs, int64(0))
		assert.Equal(t, "192.168.1.1", stat.IPAddress)
		assert.Equal(t, "test-agent/1.0", stat.UserAgent)
		assert.Equal(t, int64(13), stat.ResponseSize) // "Hello, World!" is 13 bytes
		assert.Equal(t, "", stat.Error)
		assert.Equal(t, "Other", stat.DeviceFamily)
		assert.Empty(t, stat.UserID)
		assert.Empty(t, stat.SessionID)
	})

	t.Run("records error responses", func(t *testing.T) {
		middleware := TrafficMetricMiddleware(statsRepo)

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("Internal Server Error"))
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("POST", "/error", nil)
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		// Wait a bit for the async operation to complete
		time.Sleep(10 * time.Millisecond)

		// Verify error statistics were recorded
		stats, err := statsRepo.FindByPath("/error", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "POST", stat.HttpMethod)
		assert.Equal(t, "/error", stat.Path)
		assert.Equal(t, 500, stat.HttpStatus)
		assert.Equal(t, "Internal Server Error", stat.Error)
	})

	t.Run("records session information when available", func(t *testing.T) {
		middleware := TrafficMetricMiddleware(statsRepo)

		// Create a mock session
		sessionData := &db.Session{
			Token:  "test-session-token",
			UserID: "test-user-id",
		}

		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		})

		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/authenticated", nil)

		// Add session to context
		ctx := context.WithValue(req.Context(), session.SessionKey, sessionData)
		req = req.WithContext(ctx)

		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		// Wait a bit for the async operation to complete
		time.Sleep(10 * time.Millisecond)

		// Verify session information was recorded
		stats, err := statsRepo.FindByPath("/authenticated", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "test-user-id", stat.UserID)
		assert.Equal(t, "test-session-token", stat.SessionID)
	})
}

func TestDetermineDeviceType(t *testing.T) {
	testCases := []struct {
		userAgent    string
		expectedType string
	}{
		{
			userAgent:    "Mozilla/5.0 (iPhone; CPU iPhone OS 14_0 like Mac OS X)",
			expectedType: "iPhone",
		},
		{
			userAgent:    "Mozilla/5.0 (iPad; CPU OS 14_0 like Mac OS X)",
			expectedType: "iPad",
		},
		{
			userAgent:    "Mozilla/5.0 (Linux; Android 10; SM-G973F)",
			expectedType: "Samsung SM-G973F",
		},
		{
			userAgent:    "Mozilla/5.0 (Linux; Android 10; SM-T870 Build/QP1A) tablet",
			expectedType: "Samsung SM-T870",
		},
		{
			userAgent:    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
			expectedType: "Other",
		},
		{
			userAgent:    "Googlebot/2.1 (+http://www.google.com/bot.html)",
			expectedType: "Spider",
		},
		{
			userAgent:    "curl/7.68.0",
			expectedType: "Other",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.userAgent, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			req.Header.Set("User-Agent", tc.userAgent)
			clientInfo := session.NewClientInfo(req)
			assert.Equal(t, tc.expectedType, clientInfo.DeviceFamily)
		})
	}
}

func TestGetClientIP(t *testing.T) {
	t.Run("extracts IP from X-Forwarded-For header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 192.168.1.1")
		req.RemoteAddr = "10.0.0.1:12345"

		ip := session.GetClientIP(req)
		assert.Equal(t, "203.0.113.1", ip)
	})

	t.Run("extracts IP from X-Real-IP header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("X-Real-IP", "203.0.113.2")
		req.RemoteAddr = "10.0.0.1:12345"

		ip := session.GetClientIP(req)
		assert.Equal(t, "203.0.113.2", ip)
	})

	t.Run("falls back to RemoteAddr", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"

		ip := session.GetClientIP(req)
		assert.Equal(t, "192.168.1.1", ip)
	})
}

func TestConditionalStatisticsMiddleware(t *testing.T) {
	statsRepo := db.NewMemoryTrafficMetricRepository()
	middleware := ConditionalStatisticsMiddleware(statsRepo)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	t.Run("excludes health check endpoint", func(t *testing.T) {
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		// Wait a bit for any potential async operation
		time.Sleep(10 * time.Millisecond)

		// Verify no statistics were recorded
		stats, err := statsRepo.FindByPath("/health", 10)
		require.NoError(t, err)
		assert.Len(t, stats, 0)
	})

	t.Run("includes regular endpoints", func(t *testing.T) {
		wrappedHandler := middleware(handler)

		req := httptest.NewRequest("GET", "/api/users", nil)
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		// Wait a bit for the async operation to complete
		time.Sleep(10 * time.Millisecond)

		// Verify statistics were recorded
		stats, err := statsRepo.FindByPath("/api/users", 10)
		require.NoError(t, err)
		assert.Len(t, stats, 1)
	})
}

func TestShouldExcludeFromStats(t *testing.T) {
	testCases := []struct {
		path     string
		excluded bool
	}{
		{"/health", true},
		{"/favicon.ico", true},
		{"/robots.txt", true},
		{"/sitemap.xml", true},
		{"/_/static/style.css", true},
		{"/_/static/js/app.js", true},
		{"/api/users", false},
		{"/login", false},
		{"/", false},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			result := shouldExcludeFromStats(tc.path)
			assert.Equal(t, tc.excluded, result)
		})
	}
}

func TestResponseWriterWithStats(t *testing.T) {
	t.Run("captures status code and response size", func(t *testing.T) {
		w := httptest.NewRecorder()
		rw := NewResponseWriterWithStats(w)

		rw.WriteHeader(http.StatusCreated)
		data := []byte("Hello, World!")
		n, err := rw.Write(data)

		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		assert.Equal(t, http.StatusCreated, rw.Status())
		assert.Equal(t, int64(len(data)), rw.Size())
	})

	t.Run("captures error response body", func(t *testing.T) {
		w := httptest.NewRecorder()
		rw := NewResponseWriterWithStats(w)

		rw.WriteHeader(http.StatusBadRequest)
		errorMsg := "Bad Request Error"
		rw.Write([]byte(errorMsg))

		assert.Equal(t, http.StatusBadRequest, rw.Status())
		assert.Equal(t, errorMsg, rw.Body())
	})

	t.Run("does not capture body for successful responses", func(t *testing.T) {
		w := httptest.NewRecorder()
		rw := NewResponseWriterWithStats(w)

		rw.WriteHeader(http.StatusOK)
		rw.Write([]byte("Success message"))

		assert.Equal(t, http.StatusOK, rw.Status())
		assert.Equal(t, "", rw.Body()) // Body should be empty for successful responses
	})
}
