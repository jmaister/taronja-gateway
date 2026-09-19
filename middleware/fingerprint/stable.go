package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// stableHeaders lists the request headers StableFingerprint hashes, chosen
// specifically because a real browser sends the same values for them
// regardless of what kind of request it's making — unlike JA4H's inputs
// (header count, the set of header names present, cookie/referer presence),
// which differ between a page navigation and its own subresource/XHR
// requests even from the identical browser tab. Accept and HTTP method are
// deliberately excluded for the same reason: Accept alone varies from
// "text/html,..." on a navigation to "application/json" or "*/*" on an
// API call, and method varies with what the request is doing, not with who
// is doing it. HTTP version is excluded too, unlike JA4H's use of it —
// some CDNs/domains route static assets over a different HTTP version than
// the main document, which would reintroduce exactly the per-request-type
// noise this fingerprint exists to avoid.
var stableHeaders = []string{
	"User-Agent",
	"Accept-Encoding",
	"Accept-Language",
	// Low-entropy User-Agent Client Hints: Chromium browsers send these on
	// every request by default (unlike the high-entropy hints, which need
	// an explicit Accept-CH/permissions-policy opt-in and are scoped more
	// narrowly), so they don't reintroduce request-type variance.
	"Sec-Ch-Ua",
	"Sec-Ch-Ua-Mobile",
	"Sec-Ch-Ua-Platform",
}

// StableFingerprint computes a deliberately reduced-entropy fingerprint
// from req — a custom, project-specific signal (not part of the JA4
// family; see StableFingerprintHeaderName) meant to identify the same real
// client more *consistently* across different request types than JA4H
// does, at the cost of being coarser (more distinct real clients can share
// one value) and, like any header-based signal, spoofable by a client that
// wants to avoid it.
//
// Returns "" if none of stableHeaders are present at all — a bare
// non-browser client with no User-Agent and no other listed header would
// otherwise hash down to the same fixed value as every other such client,
// which is a meaningless "fingerprint" rather than a coarse one.
func StableFingerprint(req *http.Request) string {
	var b strings.Builder
	present := false
	for _, h := range stableHeaders {
		v := req.Header.Get(h)
		if v != "" {
			present = true
		}
		b.WriteString(h)
		b.WriteByte('=')
		b.WriteString(v)
		b.WriteByte('|')
	}
	if !present {
		return ""
	}

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])[:16]
}
