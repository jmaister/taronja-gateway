package session

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewClientInfoWithJA4H(t *testing.T) {
	tests := []struct {
		name         string
		setupRequest func() *http.Request
		expectJA4    bool
	}{
		{
			name: "HTTP request without JA4H",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/test", nil)
			},
			expectJA4: false,
		},
		{
			name: "HTTPS request with JA4H in context",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.TLS = &tls.ConnectionState{
					Version:     tls.VersionTLS12,
					CipherSuite: tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					ServerName:  "example.com",
				}

				// Simulate the JA4 middleware setting the context value
				ctx := req.Context()
				ctx = context.WithValue(ctx, "ja4h_fingerprint", "test-ja4h-fingerprint")
				return req.WithContext(ctx)
			},
			expectJA4: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			clientInfo := NewClientInfo(req)

			assert.NotNil(t, clientInfo, "ClientInfo should not be nil")

			if tt.expectJA4 {
				assert.Equal(t, "test-ja4h-fingerprint", clientInfo.JA4Fingerprint, "JA4H fingerprint should match expected value")
			} else {
				assert.Empty(t, clientInfo.JA4Fingerprint, "JA4H fingerprint should be empty for requests without context value")
			}

			// Verify other fields are populated
			assert.NotNil(t, clientInfo.UserAgent, "UserAgent should be set")
			assert.NotNil(t, clientInfo.BrowserFamily, "BrowserFamily should be set")
		})
	}
}

func TestNewClientInfoWithJA4Fingerprint(t *testing.T) {
	tests := []struct {
		name        string
		setup       func() *http.Request
		expectedJA4 string
	}{
		{
			name: "Request with JA4H fingerprint in context",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				ctx := req.Context()
				ctx = context.WithValue(ctx, "ja4h_fingerprint", "fingerprint-123")
				return req.WithContext(ctx)
			},
			expectedJA4: "fingerprint-123",
		},
		{
			name: "Request without JA4H fingerprint",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				return req
			},
			expectedJA4: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setup()
			clientInfo := NewClientInfo(req)
			assert.Equal(t, tt.expectedJA4, clientInfo.JA4Fingerprint)
		})
	}
}
