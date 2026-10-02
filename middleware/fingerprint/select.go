package fingerprint

import (
	"net/http"
	"strings"
)

// HeaderName is the single request header carrying the client fingerprint
// downstream, formatted as "<type>:<value>" (see Set and SelectFingerprint).
const HeaderName = "X-Taronja-Fingerprint"

// Fingerprint type identifiers — the value db.ClientInfo.FingerprintType
// takes, naming which algorithm produced db.ClientInfo.Fingerprint.
const (
	// TypeJA4TLS only appears on rows stored by earlier versions.
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
//  1. The reduced-entropy "stable" fingerprint (TypeStable) — constant
//     across different request types from the same client; see
//     StableFingerprint.
//  2. ja4h (TypeJA4H), the caller-computed JA4H value — noisier; see
//     doc/middleware/ja4-fingerprint.md.
//
// If none is available the header is cleared.
func Assign(req *http.Request, ja4h string) {
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
