package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"

	"slices"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db" // Import db for Session type
	"github.com/jmaister/taronja-gateway/session"
)

// ErrorWithResponse is a custom error type.
type ErrorWithResponse struct {
	Code    int
	Message string
}

func (e *ErrorWithResponse) Error() string {
	return fmt.Sprintf("error with code %d: %s", e.Code, e.Message)
}

// GetSessionToken extracts the session token from the HTTP request.
func GetSessionToken(r *http.Request) (string, error) {
	cookie, err := r.Cookie(session.SessionCookieName) // Corrected: Use SessionCookieName
	if err != nil {
		if err == http.ErrNoCookie {
			return "", nil // No cookie is not an error in this context, just means no token found
		}
		return "", err // Other errors (e.g., malformed cookies by user agent)
	}
	return cookie.Value, nil
}

var OperationWithNoSecurity = []string{
	"login", // TODO: fix name when implemented using OpenAPI
	"LogoutUser",
	"HealthCheck",
	// Add any other operations that should not require authentication (cookie or token)
}

// StrictSessionMiddleware creates a strict middleware for session handling based on OpenAPI operation security requirements.
// This middleware now supports both cookie-based sessions and bearer token authentication.
func StrictSessionMiddleware(store session.SessionStore, tokenService session.TokenService, loginRedirectPathBase string, adminRequired bool) api.StrictMiddlewareFunc {
	return func(f api.StrictHandlerFunc, operationID string) api.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (responseObject interface{}, err error) {

			// Check if the operation ID is in the list of operations with no security
			authIsRequired := true
			if slices.Contains(OperationWithNoSecurity, operationID) {
				authIsRequired = false
			}

			// If auth is not required, proceed to the next handler
			if !authIsRequired {
				log.Printf("SessionStrictMiddleware: Authentication not required for operation '%s' (path: %s). Proceeding without authentication.", operationID, r.URL.Path)
				return f(ctx, w, r, requestObject)
			}

			// Try to validate the session using multiple authentication methods
			var validSession *db.Session
			var isAuthenticated bool
			var authMethod string

			// Method 1: Try cookie-based session validation first
			validSession, isAuthenticated = store.ValidateSession(r)
			if isAuthenticated && validSession != nil {
				authMethod = "cookie"
			} else {
				// Method 2: If cookie auth failed, try bearer token authentication
				validSession, isAuthenticated = store.ValidateTokenAuth(r, tokenService)
				if isAuthenticated && validSession != nil {
					authMethod = "token"
				}
			}

			// Check if we have a valid authentication and proper admin access if required
			if isAuthenticated && validSession != nil && ((adminRequired && validSession.IsAdmin) || !adminRequired) {
				// Enrich the context passed to the next handler with the session data.
				newCtx := context.WithValue(ctx, session.SessionKey, validSession)
				log.Printf("SessionStrictMiddleware: Authentication successful via %s for operation '%s' (path: %s), user: %s", authMethod, operationID, r.URL.Path, validSession.Username)
				return f(newCtx, w, r, requestObject)
			}

			// If we reach here, authentication is required but failed
			if adminRequired && validSession != nil && !validSession.IsAdmin {
				log.Printf("SessionStrictMiddleware: Admin access required for operation '%s' (path: %s), but user %s is not admin", operationID, r.URL.Path, validSession.UserID)
				return nil, &ErrorWithResponse{Code: http.StatusForbidden, Message: "Admin access required"}
			}

			log.Printf("SessionStrictMiddleware: Authentication required for operation '%s' (path: %s), but no valid session or token found", operationID, r.URL.Path)
			return nil, &ErrorWithResponse{Code: http.StatusUnauthorized, Message: "Unauthorized, no valid session or token found"}

		}
	}
}

// SessionMiddleware validates that the session cookie is present and valid, with optional token fallback
func SessionMiddleware(next http.HandlerFunc, sessionStore session.SessionStore, tokenService session.TokenService, isStatic bool, managementPrefix string, adminRequired bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Add cache-control headers to prevent caching of authenticated content
		w.Header().Set("Cache-Control", "private, no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		sessionObject, exists := sessionStore.ValidateSession(r)
		authMethod := "cookie"

		// If cookie auth failed, try token auth
		if !exists {
			sessionObject, exists = sessionStore.ValidateTokenAuth(r, tokenService)
			if exists {
				authMethod = "token"
			}
		}

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

		log.Printf("SessionMiddleware: Authentication successful via %s for path %s, user: %s", authMethod, r.URL.Path, sessionObject.Username)

		// Check if admin access is required and user is not admin
		if adminRequired && !sessionObject.IsAdmin {
			log.Printf("Admin access required but user %s (session %s) is not admin", sessionObject.UserID, sessionObject.Token)
			if isStatic {
				// Redirect to login page with error message
				originalURL := r.URL.RequestURI()
				redirectURL := managementPrefix + "/login?redirect=" + url.QueryEscape(originalURL)
				http.Redirect(w, r, redirectURL, http.StatusFound)
			} else {
				// Return 403 Forbidden for API requests
				http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
			}
			return
		}

		ctx := r.Context()
		ctx = context.WithValue(ctx, session.SessionKey, sessionObject)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// SessionMiddlewareFunc creates an api.MiddlewareFunc from the existing SessionMiddleware.
// This allows SessionMiddleware to be used with OpenAPI generated handlers that expect api.MiddlewareFunc.
func SessionMiddlewareFunc(sessionStore session.SessionStore, tokenService session.TokenService, isStatic bool, managementPrefix string, adminRequired bool) api.MiddlewareFunc {
	return func(nextHandler http.Handler) http.Handler {
		// Adapt nextHandler (an http.Handler) to http.HandlerFunc for SessionMiddleware.
		// The result of SessionMiddleware is an http.HandlerFunc, which satisfies the http.Handler interface.
		return SessionMiddleware(http.HandlerFunc(nextHandler.ServeHTTP), sessionStore, tokenService, isStatic, managementPrefix, adminRequired)
	}
}
