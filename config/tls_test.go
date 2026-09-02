package config

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTLSConfig_EffectiveRedirectPort(t *testing.T) {
	tests := []struct {
		name string
		cfg  TLSConfig
		want int
	}{
		{"unset defaults to 80", TLSConfig{}, 80},
		{"explicit 0 disables the redirect listener", TLSConfig{RedirectPort: intPtr(0)}, 0},
		{"explicit custom port", TLSConfig{RedirectPort: intPtr(8080)}, 8080},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.cfg.EffectiveRedirectPort())
		})
	}
}

// writeSelfSignedCert generates a throwaway self-signed cert/key pair and
// writes them as PEM files under a fresh temp dir, returning their paths.
func writeSelfSignedCert(t *testing.T) (certPath, keyPath string) {
	t.Helper()

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	require.NoError(t, err)
	keyBytes, err := x509.MarshalECPrivateKey(priv)
	require.NoError(t, err)

	dir := t.TempDir()
	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644))
	require.NoError(t, os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), 0o600))
	return certPath, keyPath
}

// writeServerTestConfig writes a minimal valid gateway config with the given
// server-level YAML (e.g. a "tls:" block) nested under "server:", to a temp
// file, and returns its path. Distinct from middleware_test.go's
// writeTestConfig, which already hardcodes its own "server:" block — YAML
// doesn't support usefully merging a second top-level "server:" key appended
// after it, so TLS tests need their own template with server as the
// variable part instead.
func writeServerTestConfig(t *testing.T, serverYAML string) string {
	t.Helper()
	content := "version: 1\nname: Test Gateway\nserver:\n" + serverYAML + `
management:
  admin:
    enabled: false
routes:
  - name: root
    from: /
    to: http://localhost:9999
`
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	return path
}

func TestLoadConfig_TLSDisabledByDefault(t *testing.T) {
	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8080\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.False(t, cfg.Server.TLS.Enabled, "TLS must default to disabled when the section is omitted entirely")
}

func TestLoadConfig_TLSEnabledRequiresCertAndKey(t *testing.T) {
	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8443\n  tls:\n    enabled: true\n")
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "certFile and/or keyFile is not set")
}

func TestLoadConfig_TLSEnabledRejectsUnparseableCert(t *testing.T) {
	dir := t.TempDir()
	badCert := filepath.Join(dir, "bad-cert.pem")
	badKey := filepath.Join(dir, "bad-key.pem")
	require.NoError(t, os.WriteFile(badCert, []byte("not a real cert"), 0o644))
	require.NoError(t, os.WriteFile(badKey, []byte("not a real key"), 0o600))

	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8443\n  tls:\n"+
		"    enabled: true\n    certFile: "+badCert+"\n    keyFile: "+badKey+"\n")
	_, err := LoadConfig(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load certificate/key pair")
}

func TestLoadConfig_TLSEnabledWithValidCertSucceeds(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t)

	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8443\n  tls:\n"+
		"    enabled: true\n    certFile: "+certPath+"\n    keyFile: "+keyPath+"\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.True(t, cfg.Server.TLS.Enabled)
	assert.Equal(t, certPath, cfg.Server.TLS.CertFile, "an already-absolute path must be left unchanged")
	assert.Equal(t, keyPath, cfg.Server.TLS.KeyFile)
}

func TestLoadConfig_TLSResolvesRelativeCertPaths(t *testing.T) {
	certPath, keyPath := writeSelfSignedCert(t)
	certDir := filepath.Dir(certPath)

	cwd, err := os.Getwd()
	require.NoError(t, err)
	relCert, err := filepath.Rel(cwd, certPath)
	require.NoError(t, err)
	relKey, err := filepath.Rel(cwd, keyPath)
	require.NoError(t, err)

	path := writeServerTestConfig(t, "  host: 127.0.0.1\n  port: 8443\n  tls:\n"+
		"    enabled: true\n    certFile: "+relCert+"\n    keyFile: "+relKey+"\n")
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	assert.Equal(t, certPath, cfg.Server.TLS.CertFile, "a relative certFile must resolve against the working directory, same as route toFile/toFolder")
	assert.True(t, filepath.IsAbs(cfg.Server.TLS.CertFile))
	assert.Equal(t, certDir, filepath.Dir(cfg.Server.TLS.CertFile))
}
