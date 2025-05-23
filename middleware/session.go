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
}

// StrictSessionMiddleware creates a strict middleware for session handling based on OpenAPI operation security requirements.
func StrictSessionMiddleware(store session.SessionStore, loginRedirectPathBase string) api.StrictMiddlewareFunc {
	return func(f api.StrictHandlerFunc, operationID string) api.StrictHandlerFunc {
		return func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (responseObject interface{}, err error) {

			// Check if the operation ID is in the list of operations with no security
			authIsRequired := true
			if slices.Contains(OperationWithNoSecurity, operationID) {
				authIsRequired = false
			}

			// If auth is not required, proceed to the next handler
			if !authIsRequired {
				log.Printf("SessionStrictMiddleware: Session not required for operation '%s' (path: %s). Proceeding without session.", operationID, r.URL.Path)
				return f(ctx, w, r, requestObject)
			}

			// Try to validate the session using the SessionStore's ValidateSession method
			// This method should handle token extraction from the request (e.g., cookie)
			var validSession *db.Session
			var isAuthenticated bool

			// ValidateSession is expected to handle cookie extraction and validation.
			// It returns the session data and a boolean indicating if the session is valid.
			validSession, isAuthenticated = store.ValidateSession(r)

			if isAuthenticated && validSession != nil { // Valid session exists
				// Enrich the context passed to the next handler with the session data.
				newCtx := context.WithValue(ctx, session.SessionKey, validSession) // Corrected: Use SessionKey
				log.Printf("SessionStrictMiddleware: Session valid for operation '%s' (path: %s). Proceeding with session.", operationID, r.URL.Path)
				return f(newCtx, w, r, requestObject)
			}

			// If we reach here, authIsRequired is true, and the session is not valid (isAuthenticated is false or validSession is nil).
			// Therefore, a redirect is necessary.
			log.Printf("SessionStrictMiddleware: Session required for operation '%s' (path: %s), but none found or invalid. Redirecting.", operationID, r.URL.Path)

			// These operations that are not authenticated return a 401 error
			return nil, &ErrorWithResponse{Code: http.StatusUnauthorized, Message: "Unauthorized, no session found or invalid."}

		}
	}
}

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

// SessionMiddlewareFunc creates an api.MiddlewareFunc from the existing SessionMiddleware.
// This allows SessionMiddleware to be used with OpenAPI generated handlers that expect api.MiddlewareFunc.
func SessionMiddlewareFunc(sessionStore session.SessionStore, isStatic bool, managementPrefix string) api.MiddlewareFunc {
	return func(nextHandler http.Handler) http.Handler {
		// Adapt nextHandler (an http.Handler) to http.HandlerFunc for SessionMiddleware.
		// The result of SessionMiddleware is an http.HandlerFunc, which satisfies the http.Handler interface.
		return SessionMiddleware(http.HandlerFunc(nextHandler.ServeHTTP), sessionStore, isStatic, managementPrefix)
	}
}
