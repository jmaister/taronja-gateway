package fingerprint

import (
	"net/http"
	"strings"

	"github.com/exaring/ja4plus"
)

// HeaderName is the single request header carrying the client fingerprint
// downstream, formatted as "<type>:<value>" (see Set and SelectFingerprint).
const HeaderName = "X-Taronja-Fingerprint"

// Fingerprint type identifiers — the value db.ClientInfo.FingerprintType
// takes, naming which algorithm produced db.ClientInfo.Fingerprint.
const (
	TypeJA4TLS = "ja4_tls"
	TypeStable = "stable"
	TypeJA4H   = "ja4h"
)

// Set stores a fingerprint of the given type in the request's fingerprint
// header, replacing whatever the client sent. An empty value clears it.
func Set(req *http.Request, fingerprintType, value string) {
	if value == "" {
		req.Header.Del(HeaderName)
		return
	}
	req.Header.Set(HeaderName, fingerprintType+":"+value)
}

// Assign picks the single best available fingerprint for req and stores it
// with Set, overwriting any value the client sent. Priority is
// most-reliable-available-signal-wins:
//
//  1. TLS-level JA4 (TypeJA4TLS) — a property of the client's TLS stack,
//     stable across every request on the same connection. Only present when
//     the gateway terminates TLS itself; see gateway/ja4tls.go.
//  2. The reduced-entropy "stable" fingerprint (TypeStable) — works without
//     TLS, and unlike JA4H stays constant across different request types
//     from the same client; see StableFingerprint.
//  3. ja4h (TypeJA4H), the caller-computed JA4H value — the noisiest of the
//     three; see doc/middleware/ja4-fingerprint.md.
//
// If none is available the header is cleared.
func Assign(req *http.Request, ja4h string) {
	if v := ja4plus.JA4FromContext(req.Context()); v != "" {
		Set(req, TypeJA4TLS, v)
		return
	}
	if v := StableFingerprint(req); v != "" {
		Set(req, TypeStable, v)
		return
	}
	Set(req, TypeJA4H, ja4h)
}

// SelectFingerprint returns the fingerprint value and type stored by Assign
// or Set, or ("", "") if there is none.
func SelectFingerprint(req *http.Request) (value, fingerprintType string) {
	fingerprintType, value, ok := strings.Cut(req.Header.Get(HeaderName), ":")
	if !ok || value == "" {
		return "", ""
	}
	return value, fingerprintType
}
