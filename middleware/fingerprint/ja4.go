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
