package gateway

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jmaister/taronja-gateway/config"
)

// certReloader holds the gateway's current TLS certificate behind an atomic
// pointer, so it can be swapped at runtime (see Gateway.ReloadTLSCertificate,
// driven by main.go's cert-file watcher) without touching the listener a
// *tls.Config using GetCertificate as its source is already bound to —
// unlike server.host/port, a certificate's *content* can change without any
// socket-level consequence.
type certReloader struct {
	certFile string
	keyFile  string
	current  atomic.Pointer[tls.Certificate]
}

// newCertReloader loads certFile/keyFile once up front — a certReloader
// that failed to load anything would otherwise report a nil certificate on
// every handshake with no way to tell why — and returns an error rather
// than a half-usable instance if that fails.
func newCertReloader(certFile, keyFile string) (*certReloader, error) {
	cr := &certReloader{certFile: certFile, keyFile: keyFile}
	if err := cr.Reload(); err != nil {
		return nil, err
	}
	return cr, nil
}

// Reload re-reads the cert/key files from disk and, if they parse
// successfully, atomically swaps them in as the certificate future TLS
// handshakes use. On error, it leaves whatever was already loaded serving
// unchanged — the same fail-safe behavior as a config reload — since a
// renewal tool can momentarily leave a half-written file on disk.
func (cr *certReloader) Reload() error {
	cert, err := tls.LoadX509KeyPair(cr.certFile, cr.keyFile)
	if err != nil {
		return fmt.Errorf("failed to load TLS certificate/key (certFile=%q, keyFile=%q): %w", cr.certFile, cr.keyFile, err)
	}
	cr.current.Store(&cert)
	return nil
}

// GetCertificate implements the tls.Config.GetCertificate callback
// signature, called by the runtime on every TLS handshake.
func (cr *certReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return cr.current.Load(), nil
}

// ReloadTLSCertificate re-reads the configured cert/key files from disk and
// swaps them in for future TLS handshakes, without touching the listening
// socket — see certReloader.Reload. A no-op returning nil if TLS isn't
// enabled, so callers (main.go's cert-file watcher) don't need to check
// first.
func (g *Gateway) ReloadTLSCertificate() error {
	if g.tlsCertReloader == nil {
		return nil
	}
	return g.tlsCertReloader.Reload()
}

// httpsRedirectHandler returns a handler that redirects every request to
// the HTTPS equivalent on httpsPort, preserving host, path, and query. Used
// as the Handler for Gateway.RedirectServer — the plain-HTTP listener
// TLSConfig.RedirectPort binds when TLS is enabled.
//
// Uses 308 Permanent Redirect rather than the more common 301: 301
// technically permits (and some clients historically did) rewriting a
// redirected POST into a GET, dropping the body; 308 explicitly preserves
// both method and body, which matters for a blanket HTTP->HTTPS redirect
// that has no idea what request it's redirecting.
func httpsRedirectHandler(httpsPort int) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := requestHost(r)
		if httpsPort != 443 {
			host = net.JoinHostPort(host, strconv.Itoa(httpsPort))
		}
		target := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

// requestHost returns r.Host with any port stripped, tolerant of a bare
// host with no port at all (the common case for r.Host on a plain HTTP
// request without an explicit port) — net.SplitHostPort itself errors on
// that input rather than returning it unchanged.
func requestHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		return r.Host
	}
	return host
}

// newTLSConfig builds the *tls.Config for the gateway's main listener from
// the given reloader — pulled out of NewGatewayWithDependencies mainly so
// its MinVersion choice has one place to be documented: TLS 1.2 is the
// floor every major gateway (nginx, Traefik, Envoy) still defaults to for
// broad client compatibility, with TLS 1.3 preferred automatically whenever
// both ends support it.
func newTLSConfig(reloader *certReloader) *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: reloader.GetCertificate,
	}
}

// buildRedirectServer returns the plain-HTTP server that redirects to
// HTTPS on cfg.Server.Port, or nil if TLSConfig.EffectiveRedirectPort() is
// 0 (redirect explicitly disabled).
func buildRedirectServer(cfg *config.GatewayConfig) *http.Server {
	redirectPort := cfg.Server.TLS.EffectiveRedirectPort()
	if redirectPort == 0 {
		return nil
	}
	return &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.Server.Host, redirectPort),
		Handler:      httpsRedirectHandler(cfg.Server.Port),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}
