package fingerprint

import "net/http"

// Fingerprint type identifiers — the value db.ClientInfo.FingerprintType
// takes, naming which algorithm produced db.ClientInfo.Fingerprint. See
// SelectFingerprint for the priority order between them.
const (
	TypeJA4TLS = "ja4_tls"
	TypeStable = "stable"
	TypeJA4H   = "ja4h"
)

// SelectFingerprint picks the single best available client fingerprint for
// req, so callers (session.NewClientInfo) always have exactly one
// value+type to store rather than three parallel, independently-present
// fields. Priority is most-reliable-available-signal-wins:
//
//  1. TLS-level JA4 (TypeJA4TLS) — a property of the client's TLS stack,
//     stable across every request on the same connection. Only present at
//     all when the gateway terminates TLS itself; see gateway/ja4tls.go.
//  2. The reduced-entropy "stable" fingerprint (TypeStable) — works
//     without TLS, and unlike JA4H stays constant across different request
//     types from the same client; see StableFingerprint's doc comment.
//  3. JA4H (TypeJA4H) — always computed when the ja4_fingerprint
//     middleware runs, but the noisiest of the three; see
//     doc/middleware/ja4-fingerprint.md.
//
// In practice tier 3 is rarely actually selected: StableFingerprint is
// present whenever the client sent any of a handful of near-universal
// browser headers (see its own doc comment), which is true of virtually
// every real HTTP client that also produces a usable JA4H. Tier 3 mainly
// matters for a request with no browser-identifying headers at all, where
// even JA4H itself is likely to be low-signal.
//
// Returns ("", "") if none of the three produced anything — no
// ja4_fingerprint middleware ran at all, most likely.
func SelectFingerprint(req *http.Request) (value, fingerprintType string) {
	if v := GetJA4TLSFromRequest(req); v != "" {
		return v, TypeJA4TLS
	}
	if v := GetStableFingerprintFromRequest(req); v != "" {
		return v, TypeStable
	}
	if v := GetJA4FromRequest(req); v != "" {
		return v, TypeJA4H
	}
	return "", ""
}
