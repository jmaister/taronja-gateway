package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	fp "github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
)

// TestFingerprintIntegration verifies that a client fingerprint flows
// correctly from the JA4 middleware to session/metrics via the same
// consolidated Fingerprint/FingerprintType fields — see
// fingerprint.SelectFingerprint. With a real User-Agent set (as here), the
// stable fingerprint outranks JA4H in that selection, so that's what
// actually ends up stored — not literally the JA4H value, even though the
// middleware that computes it is still named "JA4 middleware" (it computes
// all three signals, JA4H included; see ja4.go).
func TestFingerprintIntegration(t *testing.T) {
	// Simple handler that creates session and metric like the real gateway
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Test session creation (what happens during login)
		clientInfo := session.NewClientInfo(r)

		// Test traffic metric creation (what happens on every request)
		trafficMetric := session.NewTrafficMetric(r)

		// Return fingerprints for verification
		w.Header().Set("Session-Fingerprint", clientInfo.Fingerprint)
		w.Header().Set("Session-Fingerprint-Type", clientInfo.FingerprintType)
		w.Header().Set("Metric-Fingerprint", trafficMetric.Fingerprint)
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
	sessionFingerprint := rr.Header().Get("Session-Fingerprint")
	sessionType := rr.Header().Get("Session-Fingerprint-Type")
	metricFingerprint := rr.Header().Get("Metric-Fingerprint")

	assert.NotEmpty(t, sessionFingerprint, "Session should have a fingerprint")
	assert.NotEmpty(t, metricFingerprint, "Traffic metric should have a fingerprint")
	assert.Equal(t, sessionFingerprint, metricFingerprint, "Session and metric should have the same fingerprint")
	// A real User-Agent plus no TLS means "stable" wins over JA4H — see
	// fingerprint.SelectFingerprint's priority order.
	assert.Equal(t, fp.TypeStable, sessionType)

	t.Logf("Fingerprint Integration: %s (%s)", sessionFingerprint, sessionType)
}

// TestFingerprintWithoutMiddleware verifies that without the middleware, no
// fingerprint is generated at all.
func TestFingerprintWithoutMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientInfo := session.NewClientInfo(r)
		w.Header().Set(fp.JA4HHeaderName, clientInfo.Fingerprint)
		w.WriteHeader(http.StatusOK)
	})

	// Create a request WITHOUT applying the JA4 middleware
	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	// Execute the request directly (no middleware)
	handler.ServeHTTP(rr, req)

	// Verify that no fingerprint was generated
	fingerprintValue := rr.Header().Get(fp.JA4HHeaderName)
	assert.Empty(t, fingerprintValue, "Without middleware, no fingerprint should be generated")

	// Verify none of the underlying headers were set either
	assert.Empty(t, req.Header.Get(fp.JA4HHeaderName))
	assert.Empty(t, req.Header.Get(fp.StableFingerprintHeaderName))
	assert.Empty(t, req.Header.Get(fp.JA4TLSHeaderName))
}
