package gateway

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatewayJA4TLS_RealHandshakeSetsFingerprintHeader(t *testing.T) {
	var receivedJA4TLS, receivedJA4H string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedJA4TLS = r.Header.Get(fingerprint.JA4TLSHeaderName)
		receivedJA4H = r.Header.Get(fingerprint.JA4HHeaderName)
		fmt.Fprint(w, "ok")
	}))
	t.Cleanup(backend.Close)

	certPath, keyPath := writeSelfSignedCert(t, t.TempDir(), 1)
	cfg := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin", Analytics: true},
		Routes: []config.RouteConfig{
			{Name: "Backend", From: "/*", To: []string{backend.URL}},
		},
	}
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = certPath
	cfg.Server.TLS.KeyFile = keyPath

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)
	require.NotNil(t, gw.tlsJA4, "TLS-enabled gateway must build a tlsJA4 capturer")

	listener, err := net.Listen("tcp", gw.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	go gw.Server.ServeTLS(listener, "", "") //nolint:errcheck

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec
		},
		Timeout: 5 * time.Second,
	}

	require.Eventually(t, func() bool {
		resp, reqErr := client.Get(fmt.Sprintf("https://127.0.0.1:%d/ping", port))
		if reqErr != nil {
			return false
		}
		defer resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 2*time.Second, 20*time.Millisecond, "server did not start accepting TLS connections in time")

	require.NotEmpty(t, receivedJA4TLS, "the backend must receive the TLS JA4 fingerprint header")
	assert.True(t, strings.Count(receivedJA4TLS, "_") == 2, "a real JA4 string has the form <a>_<b>_<c>, got %q", receivedJA4TLS)
	// Go's own http.Client over TLS 1.3 with HTTP/1.1 (no h2 negotiated
	// here, since backend/client don't set NextProtos for it) should
	// produce a fingerprint starting with "t13" — transport=t (TCP-based
	// TLS, not QUIC/DTLS), version=13 (TLS 1.3, the default Go's client
	// negotiates). Asserting the literal prefix pins this down as a real,
	// correctly-computed JA4 rather than just "some non-empty string".
	assert.True(t, strings.HasPrefix(receivedJA4TLS, "t13"), "expected a TLS 1.3 JA4 fingerprint, got %q", receivedJA4TLS)

	// JA4H must still work unaffected — TLS JA4 is additive, not a
	// replacement.
	assert.NotEmpty(t, receivedJA4H)
}

func TestGatewayJA4TLS_AbsentWhenTLSDisabled(t *testing.T) {
	var receivedJA4TLS string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedJA4TLS = r.Header.Get(fingerprint.JA4TLSHeaderName)
		fmt.Fprint(w, "ok")
	}))
	t.Cleanup(backend.Close)

	cfg := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin", Analytics: true},
		Routes: []config.RouteConfig{
			{Name: "Backend", From: "/*", To: []string{backend.URL}},
		},
	}

	gw, err := NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)
	assert.Nil(t, gw.tlsJA4, "a plain-HTTP gateway must not build a tlsJA4 capturer at all")

	rr := httptest.NewRecorder()
	gw.Mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/ping", nil))

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, receivedJA4TLS, "no TLS connection ever happened, so there must be no fingerprint to report")
}
