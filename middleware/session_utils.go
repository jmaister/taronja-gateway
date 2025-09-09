package middleware

import (
	"context"
	"log"
	"net/http"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
)

// SessionValidationResult contains the result of session validation
type SessionValidationResult struct {
	Session         *db.Session
	IsAuthenticated bool
	AuthMethod      string
}

// ValidateSessionFromRequest validates a session using both cookie and token authentication methods
// This is the shared session validation logic used across all middleware
func ValidateSessionFromRequest(r *http.Request, sessionStore session.SessionStore, tokenService session.TokenService) *SessionValidationResult {
	var validSession *db.Session
	var isAuthenticated bool
	var authMethod string

	// Method 1: Try cookie-based session validation first
	validSession, isAuthenticated = sessionStore.ValidateSession(r)
	if isAuthenticated && validSession != nil {
		authMethod = "cookie"
	} else {
		// Method 2: If cookie auth failed, try bearer token authentication
		validSession, isAuthenticated = sessionStore.ValidateTokenAuth(r, tokenService)
		if isAuthenticated && validSession != nil {
			authMethod = "token"
		}
	}

	return &SessionValidationResult{
		Session:         validSession,
		IsAuthenticated: isAuthenticated,
		AuthMethod:      authMethod,
	}
}

// AddSessionToContext adds a session to the request context
// Returns a new request with the session added to context
func AddSessionToContext(r *http.Request, sessionData *db.Session) *http.Request {
	if sessionData != nil {
		ctx := context.WithValue(r.Context(), session.SessionKey, sessionData)
		return r.WithContext(ctx)
	}
	return r
}

// AddSessionToContextValue adds a session to a context value
// Returns a new context with the session added
func AddSessionToContextValue(ctx context.Context, sessionData *db.Session) context.Context {
	if sessionData != nil {
		return context.WithValue(ctx, session.SessionKey, sessionData)
	}
	return ctx
}

// LogAuthenticationResult logs the result of authentication attempt
func LogAuthenticationResult(result *SessionValidationResult, operationID, path string, isSuccessful bool) {
	if isSuccessful && result.IsAuthenticated && result.Session != nil {
		log.Printf("Authentication successful via %s for operation '%s' (path: %s), user: %s",
			result.AuthMethod, operationID, path, result.Session.Username)
	} else {
		log.Printf("Authentication failed for operation '%s' (path: %s)", operationID, path)
	}
}

// CheckAdminAccess validates if a session has admin access when required
func CheckAdminAccess(session *db.Session, adminRequired bool) bool {
	if !adminRequired {
		return true // Admin access not required
	}
	return session != nil && session.IsAdmin
}
