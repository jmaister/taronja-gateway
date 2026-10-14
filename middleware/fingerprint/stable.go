package fingerprint

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
)

// stableHeaders are the headers StableFingerprint hashes: a browser sends the same
// values for them across navigations, XHR and subresources, unlike JA4H's inputs.
// Accept, method and HTTP version are excluded because they vary by request type.
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

// StableFingerprint computes a coarse, project-specific fingerprint (not JA4; see
// TypeStable) that identifies a client more consistently across request types than
// JA4H, at the cost of collisions and spoofability. Returns "" if none of
// stableHeaders is present, since all such clients would otherwise share one value.
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
