package session

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRequestIsSecure is the regression test for the needs-validation item
// this closes: a deployment that terminates TLS upstream of this gateway
// (a cloud load balancer, another reverse proxy) never sets req.TLS on the
// request this gateway itself receives, so a bare "req.TLS != nil" check
// (what every Secure-cookie call site and the reverse-proxy director used
// before this) would incorrectly treat an end-to-end-HTTPS connection as
// insecure — while trusting a client-supplied X-Forwarded-Proto claim
// unconditionally would swing too far the other way, letting a direct,
// untrusted client claim HTTPS for a genuinely plain-HTTP connection.
func TestRequestIsSecure(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xfProto    string
		tls        bool
		want       bool
	}{
		{"gateway's own TLS listener", "203.0.113.5:1234", "", true, true},
		{"trusted loopback proxy's https claim is honored", "127.0.0.1:1234", "https", false, true},
		{"trusted private-range proxy's https claim is honored", "10.0.0.5:1234", "https", false, true},
		{"untrusted public peer's https claim is ignored", "203.0.113.5:1234", "https", false, false},
		{"trusted proxy with no claim at all stays insecure", "127.0.0.1:1234", "", false, false},
		{"trusted proxy claiming a non-https value stays insecure", "127.0.0.1:1234", "http", false, false},
		{"plain request, no TLS, no header", "203.0.113.5:1234", "", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xfProto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.xfProto)
			}
			if tt.tls {
				req.TLS = &tls.ConnectionState{}
			}
			assert.Equal(t, tt.want, RequestIsSecure(req))
		})
	}
}
