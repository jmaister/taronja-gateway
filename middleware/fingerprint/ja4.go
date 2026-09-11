package fingerprint

import (
	"net/http"
)

// JA4HKeyType is the type for JA4H context keys to avoid collisions
type JA4HKeyType string

// JA4HKey is the key used for storing JA4H fingerprints in request context
const JA4HKey JA4HKeyType = "ja4h_fingerprint"

// JA4HHeaderName is the HTTP header name used to store JA4H fingerprints
const JA4HHeaderName = "X-Taronja-JA4H"

// GetJA4FromRequest retrieves the JA4H fingerprint from the HTTP request headers
func GetJA4FromRequest(req *http.Request) string {
	return req.Header.Get(JA4HHeaderName)
}

// JA4TLSHeaderName is the HTTP header name used to store the TLS-level JA4
// fingerprint (github.com/exaring/ja4plus), computed once per TLS
// connection from the client's ClientHello (cipher suites, extensions,
// ALPN, TLS version) — see gateway/ja4tls.go. Unlike JA4H (JA4HHeaderName
// above), this is only ever set when the gateway terminates TLS itself,
// since that's the only point it has access to the ClientHello at all.
const JA4TLSHeaderName = "X-Taronja-JA4-TLS"

// GetJA4TLSFromRequest retrieves the TLS-level JA4 fingerprint from the
// HTTP request headers. Empty if TLS isn't enabled, or for a request that
// arrived before the connection's ClientHello was processed (there
// shouldn't be one, in practice — see gateway/ja4tls.go).
func GetJA4TLSFromRequest(req *http.Request) string {
	return req.Header.Get(JA4TLSHeaderName)
}

// StableFingerprintHeaderName is the HTTP header name used to store the
// "stable" client fingerprint — a deliberately reduced-entropy alternative
// to JA4H, built only from request properties that stay constant across a
// browsing session regardless of request type (User-Agent, Accept-Encoding,
// Accept-Language, low-entropy User-Agent Client Hints, HTTP version) and
// omitting JA4H's most volatile inputs (header count, the set of header
// names present, HTTP method, cookie/referer presence — all of which
// differ between a page navigation and its own subresource/API requests
// from the identical client; see doc/middleware/ja4-fingerprint.md). Not
// part of the JA4 spec family — a custom, project-specific signal, named
// to avoid implying otherwise. See middleware/fingerprint/stable.go.
const StableFingerprintHeaderName = "X-Taronja-Stable-Fingerprint"

// GetStableFingerprintFromRequest retrieves the stable fingerprint from the
// HTTP request headers.
func GetStableFingerprintFromRequest(req *http.Request) string {
	return req.Header.Get(StableFingerprintHeaderName)
}
