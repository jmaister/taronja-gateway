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

// --- Trusted-proxy client IP resolution ------------------------------------

func TestIsTrustedProxy(t *testing.T) {
	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{"IPv4 loopback", "127.0.0.1", true},
		{"IPv6 loopback", "::1", true},
		{"RFC 1918 10/8", "10.1.2.3", true},
		{"RFC 1918 172.16/12", "172.20.5.6", true},
		{"RFC 1918 192.168/16", "192.168.1.1", true},
		{"RFC 4193 IPv6 ULA", "fd00::1", true},
		{"public IPv4", "203.0.113.9", false},
		{"public IPv6", "2001:db8::1", false},
		{"garbage is not trusted", "not-an-ip", false},
		{"empty is not trusted", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isTrustedProxy(tt.ip))
		})
	}
}

func TestGetClientIP_TrustedProxyBehavior(t *testing.T) {
	t.Run("public peer's forwarded headers are ignored entirely", func(t *testing.T) {
		// The security-relevant case: a direct external client can't
		// present a private-range address as its own real TCP peer
		// address (not routable from the public internet), so nothing
		// needs configuring for this to be safe by default.
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.9:12345"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		req.Header.Set("X-Real-IP", "5.6.7.8")
		req.Header.Set("X-Client-IP", "9.10.11.12")

		assert.Equal(t, "203.0.113.9", GetClientIP(req), "none of the spoofed headers should be trusted")
	})

	t.Run("loopback peer honors X-Forwarded-For, first entry wins", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")

		assert.Equal(t, "203.0.113.1", GetClientIP(req))
	})

	t.Run("private-range peer honors X-Real-IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:12345"
		req.Header.Set("X-Real-IP", "203.0.113.2")

		assert.Equal(t, "203.0.113.2", GetClientIP(req))
	})

	t.Run("X-Forwarded-For takes precedence over X-Real-IP and X-Client-IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.Header.Set("X-Real-IP", "198.51.100.1")
		req.Header.Set("X-Client-IP", "192.0.2.1")

		assert.Equal(t, "203.0.113.1", GetClientIP(req))
	})

	t.Run("IPv6 loopback/ULA peer honors X-Real-IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "[fd00::1]:12345"
		req.Header.Set("X-Real-IP", "2001:db8::1")

		assert.Equal(t, "2001:db8::1", GetClientIP(req))
	})
}
