package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	fp "github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
)

// TestJA4HIntegration verifies that JA4H fingerprints flow correctly from middleware to session/metrics
func TestJA4HIntegration(t *testing.T) {
	// Simple handler that creates session and metric like the real gateway
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test session creation (what happens during login)
		clientInfo := session.NewClientInfo(r)

		// Test traffic metric creation (what happens on every request)
		trafficMetric := session.NewTrafficMetric(r)

		// Return fingerprints for verification
		w.Header().Set("Session-JA4H", clientInfo.JA4Fingerprint)
		w.Header().Set("Metric-JA4H", trafficMetric.JA4Fingerprint)
		w.WriteHeader(http.StatusOK)
	})

	// Apply the JA4 middleware
	middlewareChain := JA4Middleware(handler)

	// Create a test request with realistic headers
	req := httptest.NewRequest("POST", "/api/login", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	rr := httptest.NewRecorder()
	middlewareChain.ServeHTTP(rr, req)

	// Verify the fingerprints were populated
	sessionJA4H := rr.Header().Get("Session-JA4H")
	metricJA4H := rr.Header().Get("Metric-JA4H")

	assert.NotEmpty(t, sessionJA4H, "Session should have JA4H fingerprint")
	assert.NotEmpty(t, metricJA4H, "Traffic metric should have JA4H fingerprint")
	assert.Equal(t, sessionJA4H, metricJA4H, "Session and metric should have the same JA4H fingerprint")

	t.Logf("JA4H Integration: %s", sessionJA4H)
}

// TestJA4HWithoutMiddleware verifies that without the middleware, no fingerprint is generated
func TestJA4HWithoutMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientInfo := session.NewClientInfo(r)
		w.Header().Set(fp.JA4HHeaderName, clientInfo.JA4Fingerprint)
		w.WriteHeader(http.StatusOK)
	})

	// Create a request WITHOUT applying the JA4 middleware
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	// Execute the request directly (no middleware)
	handler.ServeHTTP(rr, req)

	// Verify that no JA4H fingerprint was generated
	fingerprintValue := rr.Header().Get(fp.JA4HHeaderName)
	assert.Empty(t, fingerprintValue, "Without middleware, JA4H fingerprint should be empty")

	// Verify header doesn't contain fingerprint
	headerFingerprint := req.Header.Get(fp.JA4HHeaderName)
	assert.Empty(t, headerFingerprint, "Without middleware, header should not contain JA4H fingerprint")
}
