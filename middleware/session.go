package middleware

import (
	"context"
	"net/http"

	"github.com/jmaister/taronja-gateway/session"
)

// SessionMiddleware validates that the session cookie is present and valid
func SessionMiddleware(next http.HandlerFunc, store session.SessionStore, isStatic bool, managementPrefix string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionObject, exists := store.Validate(r)
		if !exists {
			if isStatic {
				// Redirect to login page for static files
				http.Redirect(w, r, managementPrefix+"/login", http.StatusFound)
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
