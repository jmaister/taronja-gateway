package middleware

import (
	"context"
	"net/http"
	"net/url"

	"github.com/jmaister/taronja-gateway/session"
)

// SessionMiddleware validates that the session cookie is present and valid
func SessionMiddleware(next http.HandlerFunc, store session.SessionStore, isStatic bool, managementPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionObject, exists := store.Validate(r)
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
