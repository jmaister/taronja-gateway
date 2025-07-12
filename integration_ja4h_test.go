package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/middleware"
	fp "github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
)

// TestJA4HIntegration demonstrates the complete flow from middleware to database
func TestJA4HIntegration(t *testing.T) {
	// Create a test handler that simulates what the gateway does
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate creating a session (like what happens during login)
		sessionRepo := db.NewMemorySessionRepository()
		userRepo := db.NewMemoryUserRepository()

		// Create a test user
		testUser := &db.User{
			ID:       "test-user-id",
			Username: "testuser",
			Email:    "test@example.com",
		}
		err := userRepo.CreateUser(testUser)
		if err != nil {
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		// Create client info from request (this will include JA4H fingerprint)
		clientInfo := session.NewClientInfo(r)

		// Create a session with the client info
		sessionData := &db.Session{
			Token:           "test-session-token",
			UserID:          testUser.ID,
			Username:        testUser.Username,
			Email:           testUser.Email,
			IsAuthenticated: true,
			ValidUntil:      time.Now().Add(time.Hour),
			ClientInfo:      *clientInfo, // This includes the JA4H fingerprint
		}

		err = sessionRepo.CreateSession(sessionData.Token, sessionData)
		if err != nil {
			http.Error(w, "Failed to create session", http.StatusInternalServerError)
			return
		}

		// Also test traffic metrics
		trafficMetricRepo := db.NewMemoryTrafficMetricRepository(userRepo)
		trafficMetric := session.NewTrafficMetric(r)
		trafficMetric.HttpStatus = 200
		trafficMetric.ResponseTimeNs = 1000000 // 1ms
		trafficMetric.UserID = testUser.ID
		trafficMetric.SessionID = sessionData.Token

		err = trafficMetricRepo.Create(trafficMetric)
		if err != nil {
			http.Error(w, "Failed to create traffic metric", http.StatusInternalServerError)
			return
		}

		// Verify the JA4H fingerprint was stored
		retrievedSession, err := sessionRepo.FindSessionByToken(sessionData.Token)
		if err != nil {
			http.Error(w, "Failed to retrieve session", http.StatusInternalServerError)
			return
		}

		// Store results in response headers for testing
		w.Header().Set("Session-JA4H", retrievedSession.JA4Fingerprint)
		w.Header().Set("Metric-JA4H", trafficMetric.JA4Fingerprint)

		// Also store the fingerprint from the request context for verification
		if contextFingerprint := fp.GetJA4FromContext(r.Context()); contextFingerprint != "" {
			w.Header().Set("Context-JA4H", contextFingerprint)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Integration test successful"))
	})

	// Apply the JA4 middleware
	middlewareChain := middleware.JA4Middleware(handler)

	// Create a test request with more headers and features to make the JA4H more complete
	req := httptest.NewRequest("POST", "/api/login", nil)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,es;q=0.8")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Pragma", "no-cache")
	req.Header.Set("Sec-Ch-Ua", `"Not A(Brand";v="99", "Google Chrome";v="121", "Chromium";v="121"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Sec-Fetch-Mode", "cors")
	req.Header.Set("Sec-Fetch-Dest", "empty")
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Referer", "https://example.com/login")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	// Add some cookies to make the fingerprint more interesting
	req.Header.Set("Cookie", "sessionid=abc123; csrftoken=xyz789; preferences=theme:dark,lang:en")

	// Add custom headers that might affect fingerprinting
	req.Header.Set("X-Custom-Header", "custom-value")
	req.Header.Set("Authorization", "Bearer test-token-123")

	// Create response recorder
	rr := httptest.NewRecorder()

	// Execute the request
	middlewareChain.ServeHTTP(rr, req)

	// Verify the response
	assert.Equal(t, http.StatusOK, rr.Code, "Expected successful response")

	// Verify that JA4H fingerprints were calculated and stored
	sessionJA4H := rr.Header().Get("Session-JA4H")
	metricJA4H := rr.Header().Get("Metric-JA4H")
	contextJA4H := rr.Header().Get("Context-JA4H")

	// Expected JA4H fingerprint for this specific request configuration
	expectedJA4H := "po11cr18enus_d2abf14a9a0c_3286d15c92e4_3286d15c92e4"

	assert.NotEmpty(t, sessionJA4H, "Session should have JA4H fingerprint")
	assert.NotEmpty(t, metricJA4H, "Traffic metric should have JA4H fingerprint")
	assert.NotEmpty(t, contextJA4H, "Request context should contain JA4H fingerprint")

	// Check that all fingerprints match the expected value
	assert.Equal(t, expectedJA4H, sessionJA4H, "Session JA4H should match expected value")
	assert.Equal(t, expectedJA4H, metricJA4H, "Metric JA4H should match expected value")
	assert.Equal(t, expectedJA4H, contextJA4H, "Context JA4H should match expected value")

	// Verify all sources have the same fingerprint
	assert.Equal(t, sessionJA4H, metricJA4H, "Both session and metric should have the same JA4H fingerprint")
	assert.Equal(t, sessionJA4H, contextJA4H, "Session and context should have the same JA4H fingerprint")

	t.Logf("JA4H Integration Test Results:")
	t.Logf("  - Context JA4H: %s", contextJA4H)
	t.Logf("  - Session JA4H: %s", sessionJA4H)
	t.Logf("  - Metric JA4H: %s", metricJA4H)
}

// TestJA4HWithoutMiddleware verifies that without the middleware, no fingerprint is generated
func TestJA4HWithoutMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientInfo := session.NewClientInfo(r)
		w.Header().Set("JA4H-Fingerprint", clientInfo.JA4Fingerprint)
		w.WriteHeader(http.StatusOK)
	})

	// Create a request WITHOUT applying the JA4 middleware
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	// Execute the request directly (no middleware)
	handler.ServeHTTP(rr, req)

	// Verify that no JA4H fingerprint was generated
	fingerprint := rr.Header().Get("JA4H-Fingerprint")
	assert.Empty(t, fingerprint, "Without middleware, JA4H fingerprint should be empty")

	// Verify context doesn't contain fingerprint
	contextFingerprint := fp.GetJA4FromContext(req.Context())
	assert.Empty(t, contextFingerprint, "Without middleware, context should not contain JA4H fingerprint")
}
