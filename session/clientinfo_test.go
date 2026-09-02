package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
)

func TestNewClientInfo_Fingerprint(t *testing.T) {
	tests := []struct {
		name                string
		setupRequest        func() *http.Request
		expectedFingerprint string
		expectedType        string
	}{
		{
			name: "no fingerprint headers at all",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/test", nil)
			},
			expectedFingerprint: "",
			expectedType:        "",
		},
		{
			name: "only JA4H present falls back to it",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set(fingerprint.JA4HHeaderName, "test-ja4h-fingerprint")
				return req
			},
			expectedFingerprint: "test-ja4h-fingerprint",
			expectedType:        fingerprint.TypeJA4H,
		},
		{
			name: "realistic JA4H-only value",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/api/login", nil)
				req.Header.Set(fingerprint.JA4HHeaderName, "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac")
				return req
			},
			expectedFingerprint: "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac",
			expectedType:        fingerprint.TypeJA4H,
		},
		{
			// This is the scenario that matters most in practice: the
			// ja4_fingerprint middleware sets both headers on essentially
			// every real request (StableFingerprint just needs a
			// User-Agent, which JA4H itself already assumes), so the
			// stable fingerprint — not JA4H — is what actually ends up
			// stored day to day whenever TLS isn't involved.
			name: "stable fingerprint present outranks JA4H",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set(fingerprint.JA4HHeaderName, "ja4h-value")
				req.Header.Set(fingerprint.StableFingerprintHeaderName, "stable-value")
				return req
			},
			expectedFingerprint: "stable-value",
			expectedType:        fingerprint.TypeStable,
		},
		{
			// TLS JA4 outranks everything, including a present stable
			// fingerprint — this is what a TLS-enabled gateway's requests
			// actually look like (gateway/ja4tls.go sets this header
			// before the ja4_fingerprint middleware's stable/JA4H headers
			// are even read here).
			name: "TLS JA4 present outranks both stable and JA4H",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("GET", "/test", nil)
				req.Header.Set(fingerprint.JA4HHeaderName, "ja4h-value")
				req.Header.Set(fingerprint.StableFingerprintHeaderName, "stable-value")
				req.Header.Set(fingerprint.JA4TLSHeaderName, "t13i1311h2_f57a46bbacb6_e5728521abd4")
				return req
			},
			expectedFingerprint: "t13i1311h2_f57a46bbacb6_e5728521abd4",
			expectedType:        fingerprint.TypeJA4TLS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			clientInfo := NewClientInfo(req)

			assert.NotNil(t, clientInfo, "ClientInfo should not be nil")
			assert.Equal(t, tt.expectedFingerprint, clientInfo.Fingerprint)
			assert.Equal(t, tt.expectedType, clientInfo.FingerprintType)

			// Verify other fields are still populated regardless.
			assert.NotNil(t, clientInfo.UserAgent, "UserAgent should be set")
			assert.NotNil(t, clientInfo.BrowserFamily, "BrowserFamily should be set")
		})
	}
}
