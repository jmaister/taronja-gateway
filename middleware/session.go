package middleware

import (
	"context"
	"net/http"
	"net/url"

	"github.com/jmaister/taronja-gateway/session"
)

// SessionMiddleware validates that the session cookie is present and valid
func SessionMiddleware(next http.HandlerFunc, sessionStore session.SessionStore, isStatic bool, managementPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Add cache-control headers to prevent caching of authenticated content
		w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		sessionObject, exists := sessionStore.ValidateSession(r)
		if !exists {
			if isStatic {
				// Redirect to login page with the original URL as the redirect parameter
				originalURL := r.URL.RequestURI()
				redirectURL := managementPrefix + "/login?redirect=" + url.QueryEscape(originalURL)
				http.Redirect(w, r, redirectURL, http.StatusFound)
			} else {
				// Return 401 Unauthorized for API requests
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
			}
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, session.SessionKey, sessionObject)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
