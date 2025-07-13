package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
)

func TestJA4MiddlewareIntegration(t *testing.T) {
	tests := []struct {
		name         string
		setupRequest func() *http.Request
		expectEmpty  bool
	}{
		{
			name: "Basic HTTP request",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/test", nil)
			},
			expectEmpty: false, // JA4H should work with any HTTP request
		},
		{
			name: "POST request with headers",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/api/users", nil)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				return req
			},
			expectEmpty: false,
		},
		{
			name: "Realistic browser request",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/api/login", nil)
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36")
				req.Header.Set("Accept", "application/json, text/plain, */*")
				req.Header.Set("Accept-Language", "en-US,en;q=0.9")
				req.Header.Set("Accept-Encoding", "gzip, deflate, br")
				req.Header.Set("Cache-Control", "no-cache")
				req.Header.Set("Connection", "keep-alive")
				return req
			},
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that gets the fingerprint from headers
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Get the fingerprint from the header (like session.NewClientInfo would do)
				ja4hFingerprint := r.Header.Get(fingerprint.JA4HHeaderName)

				// Return it in response for verification
				w.Header().Set("Test-JA4H", ja4hFingerprint)
				w.WriteHeader(http.StatusOK)
			})

			middlewareChain := JA4Middleware(handler)
			req := tt.setupRequest()
			rr := httptest.NewRecorder()

			middlewareChain.ServeHTTP(rr, req)

			// Verify middleware set the header
			testJA4H := rr.Header().Get("Test-JA4H")

			if tt.expectEmpty {
				assert.Empty(t, testJA4H, "Expected empty fingerprint")
			} else {
				assert.NotEmpty(t, testJA4H, "JA4H fingerprint should be set by middleware")
				// Verify the fingerprint has the expected format
				assert.True(t, len(testJA4H) > 10, "JA4H fingerprint should be a substantial string")
				assert.Contains(t, testJA4H, "_", "JA4H fingerprint should contain underscores as separators")
				t.Logf("JA4H Fingerprint: %s", testJA4H)
			}
		})
	}
}
