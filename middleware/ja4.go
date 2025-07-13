package middleware

import (
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/middleware/fingerprint"
	"github.com/lum8rjack/go-ja4h"
)

// JA4Middleware is a middleware that calculates JA4H fingerprint for each request
// JA4H is the HTTP version of JA4 fingerprinting that analyzes HTTP request characteristics
func JA4Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Calculate JA4H fingerprint from the HTTP request
		ja4hFingerprint := ja4h.JA4H(r)

		if ja4hFingerprint == "" {
			log.Printf("Warning: JA4H fingerprint is empty for request %s %s", r.Method, r.URL.Path)
		} else {
			log.Printf("JA4H fingerprint for request %s %s: %s", r.Method, r.URL.Path, ja4hFingerprint)
		}

		// Store the fingerprint in a custom header
		r.Header.Set(fingerprint.JA4HHeaderName, ja4hFingerprint)

		next.ServeHTTP(w, r)
	})
}
