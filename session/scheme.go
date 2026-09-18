package session

import (
	"net"
	"net/http"
)

// RequestIsSecure reports whether r should be treated as having arrived
// over HTTPS end-to-end: either this gateway's own listener terminated
// TLS itself (r.TLS != nil), or the immediate peer is a trusted proxy —
// see IsTrustedProxy, the same loopback/private-range boundary
// GetClientIP already applies to X-Forwarded-For/X-Real-IP/X-Client-IP —
// that claims via X-Forwarded-Proto: https to have terminated TLS on this
// gateway's behalf.
//
// This is what a cookie's Secure flag (see providers/basicAuthentication.go,
// providers/providers.go) and the reverse-proxy director's own outbound
// X-Forwarded-Proto (see gateway/gateway.go) both need answered, and for
// the same two reasons:
//
//   - A deployment that terminates TLS somewhere upstream of this gateway
//     (a cloud load balancer, another reverse proxy in front of it) never
//     has r.TLS set on the request this gateway itself receives — only
//     the upstream terminator's own listener does. Without honoring a
//     trusted upstream's X-Forwarded-Proto claim, a session cookie would
//     ship without Secure even though the browser's actual connection was
//     HTTPS end-to-end, and every downstream HTTP->HTTPS-only decision
//     this gateway makes would see plain HTTP instead of the real scheme.
//   - Trusting that claim from just anyone would go too far the other
//     way: a direct client connecting over plain HTTP could set
//     X-Forwarded-Proto: https itself and have this gateway (and any
//     backend behind it that trusts this gateway's own forwarded headers)
//     treat a genuinely insecure connection as secure. Restricting the
//     claim to a trusted peer — the same restriction already applied to
//     the client-IP headers — closes that without requiring an operator
//     to configure anything, for the deployment shapes (proxy on the same
//     host or same private network) this gateway already assumes
//     elsewhere.
func RequestIsSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if r.Header.Get("X-Forwarded-Proto") != "https" {
		return false
	}
	remoteIP := r.RemoteAddr
	if host, _, err := net.SplitHostPort(remoteIP); err == nil {
		remoteIP = host
	}
	return IsTrustedProxy(remoteIP)
}
