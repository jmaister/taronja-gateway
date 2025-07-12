package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
)

func TestNewClientInfoWithJA4H(t *testing.T) {
	tests := []struct {
		name         string
		setupRequest func() *http.Request
		expectJA4    bool
		expectedJA4  string
	}{
		{
			name: "HTTP request without JA4H header",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/test", nil)
			},
			expectJA4:   false,
			expectedJA4: "",
		},
		{
			name: "Request with JA4H header set by middleware",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set(fingerprint.JA4HHeaderName, "test-ja4h-fingerprint")
				return req
			},
			expectJA4:   true,
			expectedJA4: "test-ja4h-fingerprint",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			clientInfo := NewClientInfo(req)

			assert.NotNil(t, clientInfo, "ClientInfo should not be nil")

			if tt.expectJA4 {
				assert.Equal(t, tt.expectedJA4, clientInfo.JA4Fingerprint, "JA4H fingerprint should match expected value")
			} else {
				assert.Empty(t, clientInfo.JA4Fingerprint, "JA4H fingerprint should be empty for requests without header")
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
			name: "Request with JA4H fingerprint in header",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				req.Header.Set(fingerprint.JA4HHeaderName, "fingerprint-123")
				return req
			},
			expectedJA4: "fingerprint-123",
		},
		{
			name: "Request without JA4H fingerprint header",
			setup: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				return req
			},
			expectedJA4: "",
		},
		{
			name: "Request with realistic JA4H fingerprint",
			setup: func() *http.Request {
				req := httptest.NewRequest("POST", "/api/login", nil)
				req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
				req.Header.Set(fingerprint.JA4HHeaderName, "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac")
				return req
			},
			expectedJA4: "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac",
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
