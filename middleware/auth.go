package middleware

import (
	"net/http"

	"github.com/jmaister/taronja-gateway/session"
)

// AuthMiddleware provides authentication middleware functionality for routes
type AuthMiddleware struct {
	SessionStore     session.SessionStore
	TokenService     session.TokenService
	ManagementPrefix string
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(sessionStore session.SessionStore, tokenService session.TokenService, managementPrefix string) *AuthMiddleware {
	return &AuthMiddleware{
		SessionStore:     sessionStore,
		TokenService:     tokenService,
		ManagementPrefix: managementPrefix,
	}
}

// AuthMiddlewareFunc creates a middleware function for authentication
// isStatic determines whether to redirect (true) or return 401 (false) on auth failure
func (a *AuthMiddleware) AuthMiddlewareFunc(isStatic bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Use SessionMiddleware logic directly
			handler := SessionMiddleware(next.(http.HandlerFunc), a.SessionStore, a.TokenService, isStatic, a.ManagementPrefix, false)
			handler.ServeHTTP(w, r)
		})
	}
}
