package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTrafficMetricWithSessionExtractionIntegration tests the integration of SessionExtractionMiddleware
// followed by TrafficMetricMiddleware to ensure session information is captured in traffic metrics
func TestTrafficMetricWithSessionExtractionIntegration(t *testing.T) {
	t.Run("traffic metrics capture session information when session extraction is applied first", func(t *testing.T) {
		// Generate unique test name for database isolation
		testName := fmt.Sprintf("middleware_integration_test_%d", time.Now().UnixNano())

		// Use modern dependency injection approach
		dependencies := deps.NewTestWithName(testName)

		sessionStore := dependencies.SessionStore
		statsRepo := dependencies.TrafficMetricRepo
		tokenService := dependencies.TokenService

		// Create a user and session
		user := &db.User{
			ID:       "integration-user-123",
			Username: "integrationuser",
			Email:    "integration@example.com",
		}

		// Create a session with valid future expiration
		testSession := &db.Session{
			Token:        "integration-session-token",
			UserID:       user.ID,
			Username:     user.Username,
			IsAdmin:      false,
			ValidUntil:   time.Now().Add(24 * time.Hour), // Valid for 24 hours
			LastActivity: time.Now(),
		}
		dependencies.SessionRepo.CreateSession("integration-session-token", testSession)

		// Create middlewares in the same order as the gateway
		sessionExtractionMiddleware := SessionExtractionMiddleware(sessionStore, tokenService)
		trafficMetricMiddleware := TrafficMetricMiddleware(statsRepo)

		// Create a simple handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Integration test response"))
		})

		// Apply middlewares in correct order: SessionExtraction -> TrafficMetric -> Handler
		finalHandler := sessionExtractionMiddleware(trafficMetricMiddleware(handler))

		// Create request with session cookie
		req := httptest.NewRequest("GET", "/api/integration-test", nil)
		req.RemoteAddr = "192.168.1.200:9090"
		req.Header.Set("User-Agent", "Integration-Test-Agent/1.0")
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: "integration-session-token",
		})

		// Execute request
		w := httptest.NewRecorder()
		finalHandler.ServeHTTP(w, req)

		// Verify response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "Integration test response", w.Body.String())

		// Wait for async traffic metric operation to complete
		time.Sleep(20 * time.Millisecond)

		// Verify traffic metrics were recorded with session information
		stats, err := statsRepo.FindByPath("/api/integration-test", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "GET", stat.HttpMethod)
		assert.Equal(t, "/api/integration-test", stat.Path)
		assert.Equal(t, 200, stat.HttpStatus)
		assert.Equal(t, int64(25), stat.ResponseSize) // "Integration test response" length
		assert.Equal(t, "192.168.1.200", stat.IPAddress)
		assert.Equal(t, "Integration-Test-Agent/1.0", stat.UserAgent)

		// Most importantly: verify session information was captured
		assert.Equal(t, "integration-user-123", stat.UserID, "UserID should be captured from session")
		assert.Equal(t, "integration-session-token", stat.SessionID, "SessionID should be captured from session")

		assert.Empty(t, stat.Error)
	})

	t.Run("traffic metrics work without session when no session available", func(t *testing.T) {
		// Generate unique test name for database isolation
		testName := fmt.Sprintf("middleware_integration_test_2_%d", time.Now().UnixNano())

		// Use modern dependency injection approach
		dependencies := deps.NewTestWithName(testName)

		sessionStore := dependencies.SessionStore
		statsRepo := dependencies.TrafficMetricRepo
		tokenService := dependencies.TokenService

		// Create middlewares in the same order as the gateway
		sessionExtractionMiddleware := SessionExtractionMiddleware(sessionStore, tokenService)
		trafficMetricMiddleware := TrafficMetricMiddleware(statsRepo)

		// Create a simple handler
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("No session response"))
		})

		// Apply middlewares in correct order: SessionExtraction -> TrafficMetric -> Handler
		finalHandler := sessionExtractionMiddleware(trafficMetricMiddleware(handler))

		// Create request WITHOUT session cookie
		req := httptest.NewRequest("GET", "/api/no-session-test", nil)
		req.RemoteAddr = "10.0.0.1:8080"
		req.Header.Set("User-Agent", "No-Session-Agent/1.0")

		// Execute request
		w := httptest.NewRecorder()
		finalHandler.ServeHTTP(w, req)

		// Verify response
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "No session response", w.Body.String())

		// Wait for async traffic metric operation to complete
		time.Sleep(20 * time.Millisecond)

		// Verify traffic metrics were recorded without session information
		stats, err := statsRepo.FindByPath("/api/no-session-test", 10)
		require.NoError(t, err)
		require.Len(t, stats, 1)

		stat := stats[0]
		assert.Equal(t, "GET", stat.HttpMethod)
		assert.Equal(t, "/api/no-session-test", stat.Path)
		assert.Equal(t, 200, stat.HttpStatus)
		assert.Equal(t, int64(19), stat.ResponseSize) // "No session response" length
		assert.Equal(t, "10.0.0.1", stat.IPAddress)
		assert.Equal(t, "No-Session-Agent/1.0", stat.UserAgent)

		// Session information should be empty
		assert.Empty(t, stat.UserID, "UserID should be empty when no session")
		assert.Empty(t, stat.SessionID, "SessionID should be empty when no session")

		assert.Empty(t, stat.Error)
	})
}
