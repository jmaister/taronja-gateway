package session

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// resetTrustedProxiesAfterTest clears the package-level trusted-proxy list
// once the calling test finishes, so one test's SetTrustedProxies call
// can't leak into whichever test runs next in this package.
func resetTrustedProxiesAfterTest(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { SetTrustedProxies(nil) })
}

func TestParseIPOrCIDR(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    bool
		wantOnesIn int // ones,bits from ipNet.Mask.Size(), checked when wantErr is false
		wantBits   int
	}{
		{name: "bare IPv4 widens to /32", input: "203.0.113.5", wantOnesIn: 32, wantBits: 32},
		{name: "bare IPv6 widens to /128", input: "::1", wantOnesIn: 128, wantBits: 128},
		{name: "IPv4 CIDR passes through", input: "10.0.0.0/8", wantOnesIn: 8, wantBits: 32},
		{name: "IPv6 CIDR passes through", input: "fd00::/8", wantOnesIn: 8, wantBits: 128},
		{name: "garbage is an error", input: "not-an-ip", wantErr: true},
		{name: "empty string is an error", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, ipNet, err := parseIPOrCIDR(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			ones, bits := ipNet.Mask.Size()
			assert.Equal(t, tt.wantOnesIn, ones)
			assert.Equal(t, tt.wantBits, bits)
		})
	}
}

func TestGetClientIP_TrustedProxyBehavior(t *testing.T) {
	t.Run("untrusted peer's forwarded headers are ignored entirely", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies(nil)
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "203.0.113.9:12345"
		req.Header.Set("X-Forwarded-For", "1.2.3.4")
		req.Header.Set("X-Real-IP", "5.6.7.8")
		req.Header.Set("X-Client-IP", "9.10.11.12")

		assert.Equal(t, "203.0.113.9", GetClientIP(req), "none of the spoofed headers should be trusted")
	})

	t.Run("trusted CIDR range honors X-Forwarded-For, first entry wins", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies([]string{"10.0.0.0/8"})
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")

		assert.Equal(t, "203.0.113.1", GetClientIP(req))
	})

	t.Run("trusted bare-IP entry matches exactly", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies([]string{"10.1.2.3"})
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:12345"
		req.Header.Set("X-Real-IP", "203.0.113.2")

		assert.Equal(t, "203.0.113.2", GetClientIP(req))
	})

	t.Run("X-Forwarded-For takes precedence over X-Real-IP and X-Client-IP", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies([]string{"10.0.0.0/8"})
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1")
		req.Header.Set("X-Real-IP", "198.51.100.1")
		req.Header.Set("X-Client-IP", "192.0.2.1")

		assert.Equal(t, "203.0.113.1", GetClientIP(req))
	})

	t.Run("a peer just outside the trusted range is not trusted", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies([]string{"10.0.0.0/24"}) // only 10.0.0.0-10.0.0.255
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.1.1:12345" // outside the /24
		req.Header.Set("X-Forwarded-For", "203.0.113.1")

		assert.Equal(t, "10.0.1.1", GetClientIP(req), "a peer outside the configured range must not be trusted")
	})

	t.Run("an invalid configured entry is skipped, valid ones still apply", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies([]string{"not-a-valid-entry", "10.0.0.0/8"})
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.1.2.3:12345"
		req.Header.Set("X-Forwarded-For", "203.0.113.1")

		assert.Equal(t, "203.0.113.1", GetClientIP(req), "a malformed entry must not prevent the valid ones from working")
	})

	t.Run("IPv6 peer and IPv6 trusted range", func(t *testing.T) {
		resetTrustedProxiesAfterTest(t)
		SetTrustedProxies([]string{"fd00::/8"})
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "[fd00::1]:12345"
		req.Header.Set("X-Real-IP", "2001:db8::1")

		assert.Equal(t, "2001:db8::1", GetClientIP(req))
	})
}
