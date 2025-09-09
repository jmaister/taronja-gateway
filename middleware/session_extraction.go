package middleware

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/session"
)

// SessionExtractionMiddleware extracts session information if available, without enforcing authentication.
// This middleware is used to populate the request context with session data for logging/analytics purposes,
// even on routes that don't require authentication.
func SessionExtractionMiddleware(store session.SessionStore, tokenService session.TokenService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use shared session validation logic
			result := ValidateSessionFromRequest(r, store, tokenService)

			// If we have a valid session, add it to the context
			if result.IsAuthenticated && result.Session != nil {
				r = AddSessionToContext(r, result.Session)
			}

			// Continue to next handler regardless of authentication status
			next.ServeHTTP(w, r)
		})
	}
}
