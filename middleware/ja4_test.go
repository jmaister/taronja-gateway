package middleware

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
)

func TestJA4Middleware(t *testing.T) {
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
			name: "HTTPS request",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/secure", nil)
				req.TLS = &tls.ConnectionState{
					Version:     tls.VersionTLS12,
					CipherSuite: tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					ServerName:  "example.com",
				}
				return req
			},
			expectEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test handler that checks the context
			var contextFingerprint string
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				contextFingerprint = fingerprint.GetJA4FromContext(r.Context())
				w.WriteHeader(http.StatusOK)
			})

			// Apply the JA4 middleware
			middleware := JA4Middleware(handler)

			// Create a test request
			req := tt.setupRequest()

			// Create a response recorder
			rr := httptest.NewRecorder()

			// Execute the request
			middleware.ServeHTTP(rr, req)

			// Check the results
			if tt.expectEmpty {
				assert.Empty(t, contextFingerprint, "Expected empty fingerprint")
			} else {
				assert.NotEmpty(t, contextFingerprint, "Expected non-empty JA4H fingerprint")
			}
		})
	}
}

func TestGetJA4FromContext(t *testing.T) {
	tests := []struct {
		name     string
		setup    func() *http.Request
		expected string
	}{
		{
			name: "Context with JA4H fingerprint",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				ctx := req.Context()
				ctx = context.WithValue(ctx, fingerprint.JA4HKey, "test-ja4h-fingerprint")
				return req.WithContext(ctx)
			},
			expected: "test-ja4h-fingerprint",
		},
		{
			name: "Context without JA4H fingerprint",
			setup: func() *http.Request {
				return httptest.NewRequest("GET", "/test", nil)
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()
			result := fingerprint.GetJA4FromContext(req.Context())
			assert.Equal(t, tt.expected, result)
		})
	}
}
