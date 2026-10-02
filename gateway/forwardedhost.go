package gateway

import (
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
)

// allowedRedirectHosts returns the lowercased hostname parsed out of
// server.url, the only known-good Host value this config contains. Empty
// when server.url is unset or unparseable.
func allowedRedirectHosts(cfg *config.GatewayConfig) map[string]bool {
	hosts := map[string]bool{}
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
// Routing never depends on the incoming Host header, so a direct client's
// own, unverified Host header must not reach a backend as fact: a backend
// that trusts X-Forwarded-Host to build absolute URLs (password-reset links,
// OAuth redirects, cache keys) could not tell it apart from a configured
// value.
//
// An empty allowedHosts (no server.url configured) forwards r.Host
// unchanged rather than dropping the header for every deployment that
// hasn't set it.
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
// host with no port (net.SplitHostPort errors on that input).
func requestHost(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.Host)
	if err != nil {
		return r.Host
	}
	return host
}
