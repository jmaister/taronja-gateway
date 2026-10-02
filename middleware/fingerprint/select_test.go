package fingerprint

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSelectFingerprint(t *testing.T) {
	tests := []struct {
		name         string
		header       string
		expectedVal  string
		expectedType string
	}{
		{"missing", "", "", ""},
		{"ja4h", "ja4h:ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac", "ge11nn05_9c68f7ca5aaf_d4bd6ad6f3ac", TypeJA4H},
		{"tls", "ja4_tls:t13i1311h2_f57a46bbacb6_e5728521abd4", "t13i1311h2_f57a46bbacb6_e5728521abd4", TypeJA4TLS},
		{"value keeps later colons", "stable:a:b", "a:b", TypeStable},
		{"no separator", "garbage", "", ""},
		{"empty value", "ja4h:", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.header != "" {
				req.Header.Set(HeaderName, tt.header)
			}
			val, typ := SelectFingerprint(req)
			assert.Equal(t, tt.expectedVal, val)
			assert.Equal(t, tt.expectedType, typ)
		})
	}
}

func TestAssign_PrefersStableOverJA4H(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("Accept-Language", "en-US")
	req.Header.Set(HeaderName, "ja4_tls:spoofed")

	Assign(req, "ja4h-value")

	val, typ := SelectFingerprint(req)
	assert.Equal(t, TypeStable, typ)
	assert.Equal(t, StableFingerprint(req), val)
}

func TestAssign_FallsBackToJA4H(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Del("User-Agent")

	Assign(req, "ja4h-value")

	val, typ := SelectFingerprint(req)
	assert.Equal(t, TypeJA4H, typ)
	assert.Equal(t, "ja4h-value", val)
}

func TestAssign_ClearsHeaderWhenNothingAvailable(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(HeaderName, "ja4_tls:spoofed")

	Assign(req, "")

	assert.Empty(t, req.Header.Get(HeaderName))
}
