package middleware

import (
	"context"
	"log"
	"net/http"
	"slices"
	"strings"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// TokenAuthMiddleware creates a middleware for token-based authentication
type TokenAuthMiddleware struct {
	tokenService *auth.TokenService
}

// NewTokenAuthMiddleware creates a new token authentication middleware
func NewTokenAuthMiddleware(tokenService *auth.TokenService) *TokenAuthMiddleware {
	return &TokenAuthMiddleware{
		tokenService: tokenService,
	}
}

// GetAuthorizationToken extracts the bearer token from the Authorization header
func GetAuthorizationToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", nil // No authorization header is not an error, just means no token
	}

	// Check if it's a Bearer token
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return "", nil // Not a bearer token, not an error
	}

	return parts[1], nil
}

// Operations that don't require token authentication (extends the session middleware list)
var OperationWithNoTokenSecurity = []string{
	"login",
	"LogoutUser",
	"HealthCheck",
	// Add any other operations that should not require token auth
}

// StrictTokenAuthMiddleware creates a strict middleware for token-based authentication
func (m *TokenAuthMiddleware) StrictTokenAuthMiddleware(adminRequired bool) api.StrictMiddlewareFunc {
	return func(f api.StrictHandlerFunc, operationID string) api.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (responseObject interface{}, err error) {

			// Check if the operation ID is in the list of operations with no security
			authIsRequired := true
			if slices.Contains(OperationWithNoTokenSecurity, operationID) {
				authIsRequired = false
			}

			// If auth is not required, proceed to the next handler
			if !authIsRequired {
				log.Printf("TokenAuthMiddleware: Token auth not required for operation '%s' (path: %s). Proceeding without token validation.", operationID, r.URL.Path)
				return f(ctx, w, r, requestObject)
			}

			// Extract token from Authorization header
			token, err := GetAuthorizationToken(r)
			if err != nil {
				log.Printf("TokenAuthMiddleware: Error extracting token for operation '%s' (path: %s): %v", operationID, r.URL.Path, err)
				return nil, &ErrorWithResponse{Code: http.StatusUnauthorized, Message: "Invalid authorization header format"}
			}

			if token == "" {
				log.Printf("TokenAuthMiddleware: No token provided for operation '%s' (path: %s)", operationID, r.URL.Path)
				return nil, &ErrorWithResponse{Code: http.StatusUnauthorized, Message: "Authorization token required"}
			}

			// Validate the token
			user, tokenData, err := m.tokenService.ValidateToken(token)
			if err != nil {
				log.Printf("TokenAuthMiddleware: Token validation failed for operation '%s' (path: %s): %v", operationID, r.URL.Path, err)
				return nil, &ErrorWithResponse{Code: http.StatusUnauthorized, Message: "Invalid or expired token"}
			}

			// Check if admin access is required
			if adminRequired && user.Provider != db.AdminProvider {
				log.Printf("TokenAuthMiddleware: Admin access required for operation '%s' (path: %s), but user %s is not admin", operationID, r.URL.Path, user.ID)
				return nil, &ErrorWithResponse{Code: http.StatusForbidden, Message: "Admin access required"}
			}

			// Create a session-like object for compatibility with existing code
			sessionObject := &db.Session{
				Token:           tokenData.ID, // Use token ID as session token
				UserID:          user.ID,
				Username:        user.Username,
				Email:           user.Email,
				IsAuthenticated: true,
				IsAdmin:         user.Provider == db.AdminProvider,
				Provider:        user.Provider,
				SessionName:     tokenData.Name,
				CreatedFrom:     "token_auth",
			}

			// Enrich the context with both user and session data
			newCtx := context.WithValue(ctx, session.SessionKey, sessionObject)
			newCtx = context.WithValue(newCtx, "user", user)
			newCtx = context.WithValue(newCtx, "token", tokenData)

			log.Printf("TokenAuthMiddleware: Token validation successful for operation '%s' (path: %s), user: %s", operationID, r.URL.Path, user.Username)
			return f(newCtx, w, r, requestObject)
		}
	}
}

// TokenAuthMiddlewareFunc creates a standard middleware function for token authentication
func (m *TokenAuthMiddleware) TokenAuthMiddlewareFunc(adminRequired bool) api.MiddlewareFunc {
	return func(nextHandler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add cache-control headers to prevent caching of authenticated content
			w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")

			// Extract token from Authorization header
			token, err := GetAuthorizationToken(r)
			if err != nil {
				log.Printf("TokenAuthMiddleware: Error extracting token (path: %s): %v", r.URL.Path, err)
				http.Error(w, "Invalid authorization header format", http.StatusUnauthorized)
				return
			}

			if token == "" {
				log.Printf("TokenAuthMiddleware: No token provided (path: %s)", r.URL.Path)
				http.Error(w, "Authorization token required", http.StatusUnauthorized)
				return
			}

			// Validate the token
			user, tokenData, err := m.tokenService.ValidateToken(token)
			if err != nil {
				log.Printf("TokenAuthMiddleware: Token validation failed (path: %s): %v", r.URL.Path, err)
				http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
				return
			}

			// Check if admin access is required
			if adminRequired && user.Provider != db.AdminProvider {
				log.Printf("TokenAuthMiddleware: Admin access required (path: %s), but user %s is not admin", r.URL.Path, user.ID)
				http.Error(w, "Admin access required", http.StatusForbidden)
				return
			}

			// Create a session-like object for compatibility with existing code
			sessionObject := &db.Session{
				Token:           tokenData.ID, // Use token ID as session token
				UserID:          user.ID,
				Username:        user.Username,
				Email:           user.Email,
				IsAuthenticated: true,
				IsAdmin:         user.Provider == db.AdminProvider,
				Provider:        user.Provider,
				SessionName:     tokenData.Name,
				CreatedFrom:     "token_auth",
			}

			// Enrich the context with both user and session data
			ctx := r.Context()
			ctx = context.WithValue(ctx, session.SessionKey, sessionObject)
			ctx = context.WithValue(ctx, "user", user)
			ctx = context.WithValue(ctx, "token", tokenData)

			log.Printf("TokenAuthMiddleware: Token validation successful (path: %s), user: %s", r.URL.Path, user.Username)
			nextHandler.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
