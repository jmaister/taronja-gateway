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
			assert.Equal(t, tt.want, IsTrustedProxy(tt.ip))
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

	t.Run("loopback peer honors X-Forwarded-For, rightmost untrusted entry wins", func(t *testing.T) {
		// Not "first entry wins": see rightmostUntrustedForwardedFor's own
		// doc comment for why the leftmost entry is the wrong one to trust
		// when a proxy appends rather than replaces this header — both
		// entries here are equally "untrusted" (public) addresses, so this
		// only pins down which end of the list wins, matching the real
		// spoofing scenario in TestGetClientIP_ForwardedForSpoofing below.
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")

		assert.Equal(t, "198.51.100.1", GetClientIP(req))
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

	t.Run("a chain of trusted proxies is peeled to find the real client", func(t *testing.T) {
		// Two hops between the client and this gateway, both private —
		// e.g. an internal load balancer, then nginx, both in the same
		// Docker network — each appending its own observed peer per RFC
		// 7239. The real client (rightmost untrusted entry) is neither
		// the leftmost entry nor simply "the last one," but the first one
		// walking from the right that isn't itself another trusted hop.
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.2:12345" // nginx, the immediate (trusted) peer
		req.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1")

		assert.Equal(t, "203.0.113.7", GetClientIP(req))
	})

	t.Run("an all-trusted chain falls through to X-Real-IP", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.2:12345"
		req.Header.Set("X-Forwarded-For", "10.0.0.1, 127.0.0.1")
		req.Header.Set("X-Real-IP", "203.0.113.8")

		assert.Equal(t, "203.0.113.8", GetClientIP(req), "an X-Forwarded-For with no untrusted entry at all must not be treated as the answer")
	})
}

// TestGetClientIP_ForwardedForSpoofing is the regression test for the
// vulnerability rightmostUntrustedForwardedFor fixes: a client sending its
// own X-Forwarded-For to a proxy that *appends* rather than replaces
// (nginx without a real_ip module, Traefik's default passthrough, and
// others — a common, not universal, topology) previously got its own fake
// leading entry trusted outright, letting it rotate a fresh "identity" per
// request to defeat IP-based rate limiting/blocklisting and poison
// recorded IP addresses. The gateway's own immediate peer here is the
// proxy (trusted); the proxy appended the real client's address as the
// last entry, exactly as an append-style proxy would.
func TestGetClientIP_ForwardedForSpoofing(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345" // the trusted proxy sitting in front of this gateway
	req.Header.Set("X-Forwarded-For", "6.6.6.6, 198.51.100.42")

	assert.Equal(t, "198.51.100.42", GetClientIP(req), "the real client (appended by the trusted proxy) must win over the attacker's own spoofed leading entry")
}
