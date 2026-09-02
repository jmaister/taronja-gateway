package gateway

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/gateway/deps"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeSelfSignedCert generates a throwaway self-signed cert/key pair
// (serialNumber lets a test tell two generated certs apart, e.g. for a
// reload test) and writes them as PEM files under a fresh temp dir,
// returning their paths.
func writeSelfSignedCert(t *testing.T, dir string, serialNumber int64) (certPath, keyPath string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(serialNumber),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), 0o600))
	return certPath, keyPath
}

// --- certReloader ---------------------------------------------------------

func TestCertReloader_LoadsAndServesTheCertificate(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t, t.TempDir(), 1)

	cr, err := newCertReloader(certPath, keyPath)
	require.NoError(t, err)

	cert, err := cr.GetCertificate(nil)
	require.NoError(t, err)
	require.NotNil(t, cert)
	assert.Equal(t, big.NewInt(1), cert.Leaf.SerialNumber)
}

func TestCertReloader_New_FailsOnUnparseableFiles(t *testing.T) {
	dir := t.TempDir()
	badCert := filepath.Join(dir, "bad-cert.pem")
	badKey := filepath.Join(dir, "bad-key.pem")
	require.NoError(t, os.WriteFile(badCert, []byte("not a cert"), 0o644))
	require.NoError(t, os.WriteFile(badKey, []byte("not a key"), 0o600))

	_, err := newCertReloader(badCert, badKey)
	assert.Error(t, err)
}

func TestCertReloader_Reload_SwapsInTheNewCertificate(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeSelfSignedCert(t, dir, 1)

	cr, err := newCertReloader(certPath, keyPath)
	require.NoError(t, err)
	cert, err := cr.GetCertificate(nil)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(1), cert.Leaf.SerialNumber)

	// Overwrite with a distinct certificate at the same paths, as a renewal
	// tool would.
	writeSelfSignedCert(t, dir, 2)
	require.NoError(t, cr.Reload())

	cert, err = cr.GetCertificate(nil)
	require.NoError(t, err)
	assert.Equal(t, big.NewInt(2), cert.Leaf.SerialNumber, "Reload must swap in the certificate now on disk")
}

func TestCertReloader_Reload_KeepsOldCertificateOnParseFailure(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := writeSelfSignedCert(t, dir, 1)

	cr, err := newCertReloader(certPath, keyPath)
	require.NoError(t, err)

	// Simulate a renewal tool leaving a half-written file mid-copy.
	require.NoError(t, os.WriteFile(certPath, []byte("garbage"), 0o644))
	err = cr.Reload()
	assert.Error(t, err)

	cert, getErr := cr.GetCertificate(nil)
	require.NoError(t, getErr)
	assert.Equal(t, big.NewInt(1), cert.Leaf.SerialNumber, "a failed reload must not disturb the certificate already serving")
}

// --- requestHost / httpsRedirectHandler -----------------------------------

func TestRequestHost(t *testing.T) {
	tests := []struct {
		host string
		want string
	}{
		{"example.com", "example.com"},
		{"example.com:8080", "example.com"},
		{"127.0.0.1:8080", "127.0.0.1"},
		{"[::1]:8080", "::1"},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = tt.host
		assert.Equal(t, tt.want, requestHost(req), "Host: %q", tt.host)
	}
}

func TestHTTPSRedirectHandler(t *testing.T) {
	tests := []struct {
		name      string
		host      string
		httpsPort int
		path      string
		want      string
	}{
		{"default 443 port omitted from Location", "example.com", 443, "/foo?bar=1", "https://example.com/foo?bar=1"},
		{"non-443 port included in Location", "example.com:80", 8443, "/foo", "https://example.com:8443/foo"},
		{"IP host", "127.0.0.1:80", 8443, "/", "https://127.0.0.1:8443/"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := httpsRedirectHandler(tt.httpsPort)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Host = tt.host
			rw := httptest.NewRecorder()
			handler.ServeHTTP(rw, req)

			assert.Equal(t, http.StatusPermanentRedirect, rw.Code)
			assert.Equal(t, tt.want, rw.Header().Get("Location"))
		})
	}
}

func TestHTTPSRedirectHandler_PreservesMethodAndBody(t *testing.T) {
	// 308 (unlike 301) must not let a client silently turn a POST into a
	// GET on the redirected request — this only asserts the status code,
	// since actually following the redirect and re-sending the body is the
	// HTTP client's job, not the handler's, but 308 is the whole point.
	handler := httpsRedirectHandler(443)
	req := httptest.NewRequest(http.MethodPost, "/submit", nil)
	req.Host = "example.com"
	rw := httptest.NewRecorder()
	handler.ServeHTTP(rw, req)

	assert.Equal(t, http.StatusPermanentRedirect, rw.Code)
	assert.Equal(t, "https://example.com/submit", rw.Header().Get("Location"))
}

// --- buildRedirectServer ---------------------------------------------------

func TestBuildRedirectServer_NilWhenRedirectDisabled(t *testing.T) {
	zero := 0
	cfg := &config.GatewayConfig{}
	cfg.Server.TLS.RedirectPort = &zero
	assert.Nil(t, buildRedirectServer(cfg))
}

func TestBuildRedirectServer_DefaultsToPort80(t *testing.T) {
	cfg := &config.GatewayConfig{}
	cfg.Server.Host = "127.0.0.1"
	srv := buildRedirectServer(cfg)
	require.NotNil(t, srv)
	assert.Equal(t, "127.0.0.1:80", srv.Addr)
}

// --- End-to-end: a real TLS handshake and a real redirect response --------

// newTLSTestGateway builds a gateway with TLS enabled against a freshly
// generated self-signed cert, and one plain proxy route so a real request
// through it exercises the whole stack, not just the handshake.
func newTLSTestGateway(t *testing.T) (gw *Gateway, certPath, keyPath string) {
	t.Helper()
	certPath, keyPath = writeSelfSignedCert(t, t.TempDir(), 1)

	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "hello from backend")
	}))
	t.Cleanup(backend.Close)

	cfg := &config.GatewayConfig{
		Server:     config.ServerConfig{Host: "127.0.0.1", Port: 0},
		Management: config.ManagementConfig{Prefix: "/admin"},
		Routes: []config.RouteConfig{
			{Name: "Backend", From: "/*", To: []string{backend.URL}},
		},
	}
	cfg.Server.TLS.Enabled = true
	cfg.Server.TLS.CertFile = certPath
	cfg.Server.TLS.KeyFile = keyPath

	var err error
	gw, err = NewGatewayWithDependencies(cfg, nil, deps.NewTest())
	require.NoError(t, err)
	return gw, certPath, keyPath
}

func TestGatewayTLS_RealHandshakeServesRequests(t *testing.T) {
	gw, _, _ := newTLSTestGateway(t)
	require.NotNil(t, gw.Server.TLSConfig, "TLS-enabled gateway must have a TLSConfig")

	listener, err := net.Listen("tcp", gw.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	go gw.Server.ServeTLS(listener, "", "") //nolint:errcheck // shut down via listener.Close() in defer

	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test-only, self-signed cert
		},
		Timeout: 5 * time.Second,
	}

	var resp *http.Response
	require.Eventually(t, func() bool {
		var reqErr error
		resp, reqErr = client.Get(fmt.Sprintf("https://127.0.0.1:%d/", port))
		return reqErr == nil
	}, 2*time.Second, 20*time.Millisecond, "server did not start accepting TLS connections in time")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "hello from backend", string(body))
}

func TestGatewayTLS_CertificateHotReloadsWithoutRestart(t *testing.T) {
	gw, certPath, _ := newTLSTestGateway(t)

	listener, err := net.Listen("tcp", gw.Server.Addr)
	require.NoError(t, err)
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	go gw.Server.ServeTLS(listener, "", "") //nolint:errcheck

	dial := func() *x509.Certificate {
		var conn *tls.Conn
		require.Eventually(t, func() bool {
			var dialErr error
			conn, dialErr = tls.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
			return dialErr == nil
		}, 2*time.Second, 20*time.Millisecond)
		defer conn.Close()
		return conn.ConnectionState().PeerCertificates[0]
	}

	assert.Equal(t, big.NewInt(1), dial().SerialNumber)

	// Overwrite the same paths with a new cert (serial 2), as a renewal tool
	// would, then reload — the listener/socket must not need to change at
	// all for the new certificate to take effect.
	writeSelfSignedCert(t, filepath.Dir(certPath), 2)
	require.NoError(t, gw.ReloadTLSCertificate())

	assert.Equal(t, big.NewInt(2), dial().SerialNumber, "a new TLS handshake after reload must present the renewed certificate")
}

func TestGatewayTLS_RedirectServer_RealRequest(t *testing.T) {
	gw, _, _ := newTLSTestGateway(t)
	require.NotNil(t, gw.RedirectServer, "TLS enabled with no redirectPort override must build a redirect listener")

	// Rebind the redirect server to an ephemeral port for the test rather
	// than the real default 80, which is very likely unavailable/privileged
	// in a test environment.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer listener.Close()
	redirectPort := listener.Addr().(*net.TCPAddr).Port

	go gw.RedirectServer.Serve(listener) //nolint:errcheck

	client := &http.Client{
		Timeout: 5 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var resp *http.Response
	require.Eventually(t, func() bool {
		var reqErr error
		resp, reqErr = client.Get(fmt.Sprintf("http://127.0.0.1:%d/some/path", redirectPort))
		return reqErr == nil
	}, 2*time.Second, 20*time.Millisecond)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusPermanentRedirect, resp.StatusCode)
	location := resp.Header.Get("Location")
	assert.Contains(t, location, "https://127.0.0.1:")
	assert.Contains(t, location, "/some/path")
}
