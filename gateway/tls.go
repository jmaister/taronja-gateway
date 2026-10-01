package gateway

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"golang.org/x/crypto/acme"
	"golang.org/x/crypto/acme/autocert"
)

// staticCert holds a TLS certificate loaded once, at startup, from a
// configured certFile/keyFile pair.
type staticCert struct {
	cert *tls.Certificate
}

// newStaticCert loads certFile/keyFile once up front and returns an error
// if that fails, rather than a half-usable instance that would otherwise
// report a nil certificate on every handshake with no way to tell why.
func newStaticCert(certFile, keyFile string) (*staticCert, error) {
	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS certificate/key (certFile=%q, keyFile=%q): %w", certFile, keyFile, err)
	}
	return &staticCert{cert: &cert}, nil
}

// GetCertificate implements the tls.Config.GetCertificate callback
// signature, called by the runtime on every TLS handshake.
func (sc *staticCert) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	return sc.cert, nil
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
//
// allowedHosts, when non-empty, is the exact set of Host values (see
// allowedRedirectHosts) this handler will build a redirect target from — a
// request whose Host isn't in it gets a plain 400 instead. Without this, a
// direct request with a forged Host header got a 308 pointing at whatever
// host the attacker supplied, off the gateway's own domain — an
// open-redirect/reputation-laundering primitive, even though this listener
// never terminates TLS itself and so never exposes a cookie doing it. An
// empty allowedHosts (nothing could be derived from cfg — see that
// function) falls back to today's fully permissive behavior rather than
// rejecting every request, since a deployment that hasn't set server.url
// shouldn't have its redirect listener broken outright by this check.
func httpsRedirectHandler(httpsPort int, allowedHosts map[string]bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := requestHost(r)
		if len(allowedHosts) > 0 && !allowedHosts[strings.ToLower(host)] {
			http.Error(w, "invalid host", http.StatusBadRequest)
			return
		}
		if httpsPort != 443 {
			host = net.JoinHostPort(host, strconv.Itoa(httpsPort))
		}
		target := "https://" + host + r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

// allowedRedirectHosts derives the Host values httpsRedirectHandler should
// accept from cfg: when ACME is configured, its own Domains list — the
// exact set of hostnames the gateway will actually present a valid
// certificate for, so nothing else is a legitimate redirect target anyway
// — otherwise the hostname parsed out of server.url. Returns an empty map
// (not nil is not required; either works, see httpsRedirectHandler) when
// neither is configured, in which case there's genuinely no known-good
// value in this config to validate a Host header against.
func allowedRedirectHosts(cfg *config.GatewayConfig) map[string]bool {
	hosts := map[string]bool{}
	if cfg.Server.TLS.ACME != nil {
		for _, domain := range cfg.Server.TLS.ACME.Domains {
			hosts[strings.ToLower(domain)] = true
		}
		return hosts
	}
	if cfg.Server.URL != "" {
		if parsed, err := url.Parse(cfg.Server.URL); err == nil && parsed.Hostname() != "" {
			hosts[strings.ToLower(parsed.Hostname())] = true
		}
	}
	return hosts
}

// trustedForwardedHost returns r.Host for use as the outbound
// X-Forwarded-Host value the reverse-proxy director sends to a backend, or
// "" if it shouldn't be forwarded at all.
//
// Nothing about routing in this gateway depends on the incoming Host
// header — every route matches purely on request path — so before this,
// a direct client's own, entirely unverified Host header was forwarded to
// the backend as fact. A backend that (reasonably) trusts its own
// gateway's X-Forwarded-Host to build absolute URLs (a password-reset
// link, an OAuth redirect, a cache key) had no way to tell that value
// apart from one the gateway operator actually configured.
//
// allowedHosts is allowedRedirectHosts(cfg) — the same known-good set
// httpsRedirectHandler already validates a forged Host header against for
// exactly this reason. An empty allowedHosts (nothing configured to
// validate against — see that function) falls back to forwarding r.Host
// unchanged, matching today's existing behavior, rather than dropping the
// header for every deployment that hasn't set server.url/ACME domains.
func trustedForwardedHost(r *http.Request, allowedHosts map[string]bool) string {
	if len(allowedHosts) == 0 {
		return r.Host
	}
	if allowedHosts[strings.ToLower(requestHost(r))] {
		return r.Host
	}
	return ""
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
// the given static certificate — pulled out of NewGatewayWithDependencies
// mainly so its MinVersion choice has one place to be documented: TLS 1.2
// is the floor every major gateway (nginx, Traefik, Envoy) still defaults
// to for broad client compatibility, with TLS 1.3 preferred automatically
// whenever both ends support it. Shared with the ACME path (see
// newACMEManager) so both certificate sources enforce the same minimum.
func newTLSConfig(cert *staticCert) *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: cert.GetCertificate,
	}
}

// newACMEManager builds the autocert.Manager that obtains and renews the
// gateway's certificate automatically via the ACME protocol, from
// server.tls.acme. It makes no network calls itself — registration and
// certificate issuance only happen lazily, on a real TLS handshake for a
// configured domain (autocert.Manager.GetCertificate's own behavior) — so
// constructing this at gateway startup has no side effects to worry about,
// the same property config.LoadConfig's TLS validation relies on for ACME
// (see its comment: there's nothing more to check than the config's shape).
//
// HostPolicy is always set to exactly cfg.Domains (autocert.HostWhitelist):
// leaving it nil would let anyone connecting by IP with an arbitrary SNI
// hostname trigger a real certificate request for that hostname, which is
// both a request-forgery risk and a fast way to exhaust the CA's rate
// limit — see the HostPolicy field's own doc comment in autocert for the
// same warning.
func newACMEManager(cfg *config.ACMEConfig) *autocert.Manager {
	manager := &autocert.Manager{
		Prompt:     autocert.AcceptTOS,
		Cache:      autocert.DirCache(cfg.CacheDir),
		HostPolicy: autocert.HostWhitelist(cfg.Domains...),
		Email:      cfg.Email,
	}
	if cfg.DirectoryURL != "" {
		manager.Client = &acme.Client{DirectoryURL: cfg.DirectoryURL}
	}
	return manager
}

// acmeTLSConfig returns the *tls.Config for an ACME-backed listener:
// manager.TLSConfig() already wires GetCertificate and the NextProtos ACME
// needs (including "acme-tls/1" for the tls-alpn-01 challenge, alongside
// "h2"/"http/1.1" for normal traffic) — this only adds the same MinVersion
// floor newTLSConfig uses for the static-file path, so both certificate
// sources enforce it identically.
func acmeTLSConfig(manager *autocert.Manager) *tls.Config {
	tlsConfig := manager.TLSConfig()
	tlsConfig.MinVersion = tls.VersionTLS12
	return tlsConfig
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
		Handler:      httpsRedirectHandler(cfg.Server.Port, allowedRedirectHosts(cfg)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  120 * time.Second,
	}
}
