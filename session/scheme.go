package session

import (
	"net"
	"net/http"
)

// RequestIsSecure reports whether r should be treated as HTTPS end to end: the
// listener terminated TLS itself, or the peer is a trusted proxy (IsTrustedProxy)
// claiming it via X-Forwarded-Proto: https.
//
// TLS terminated upstream never sets r.TLS here, so without the claim cookies would
// lack Secure. Accepting the claim from any peer would let a plain-HTTP client
// appear secure, hence the same trusted-peer rule used for client-IP headers.
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
