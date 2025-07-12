package fingerprint

import (
	"context"
	"net/http"
)

// JA4HKeyType is the type for JA4H context keys to avoid collisions
type JA4HKeyType string

// JA4HKey is the key used for storing JA4H fingerprints in request context
const JA4HKey JA4HKeyType = "ja4h_fingerprint"

// GetJA4FromContext retrieves the JA4H fingerprint from the request context
func GetJA4FromContext(ctx context.Context) string {
	if fingerprint, ok := ctx.Value(JA4HKey).(string); ok {
		return fingerprint
	}
	return ""
}

// GetJA4FromRequest retrieves the JA4H fingerprint from the HTTP request context
func GetJA4FromRequest(req *http.Request) string {
	if ctx := req.Context(); ctx != nil {
		return GetJA4FromContext(ctx)
	}
	return ""
}
