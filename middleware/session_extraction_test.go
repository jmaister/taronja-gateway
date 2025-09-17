package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSessionExtractionMiddleware(t *testing.T) {
	t.Run("extracts session from cookie and adds to context", func(t *testing.T) {
		// Initialize test database and reset after test
		db.ResetConnection()
		db.SetupTestDB("TestSessionExtractionMiddleware_Cookie")
		defer db.ResetConnection()

		// Set up repositories and services
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		tokenService := createMockTokenService()

		// Create a user and session
		user := &db.User{
			ID:       "user-123",
			Username: "testuser",
			Email:    "test@example.com",
		}

		// Create a session with valid future expiration
		testSession := &db.Session{
			Token:        "test-session-token",
			UserID:       user.ID,
			Username:     user.Username,
			IsAdmin:      false,
			ValidUntil:   time.Now().Add(24 * time.Hour), // Valid for 24 hours
			LastActivity: time.Now(),
		}
		sessionRepo.CreateSession("test-session-token", testSession)

		// Create the middleware
		middleware := SessionExtractionMiddleware(sessionStore, tokenService)

		// Create a handler that checks for session in context
		var capturedSession *db.Session
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessionData, exists := r.Context().Value(session.SessionKey).(*db.Session); exists {
				capturedSession = sessionData
			}
			w.WriteHeader(http.StatusOK)
		})

		// Wrap handler with middleware
		wrappedHandler := middleware(handler)

		// Create request with session cookie
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: "test-session-token",
		})

		// Execute request
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Verify session was extracted and added to context
		assert.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, capturedSession)
		assert.Equal(t, "user-123", capturedSession.UserID)
		assert.Equal(t, "testuser", capturedSession.Username)
		assert.Equal(t, "test-session-token", capturedSession.Token)
	})

	t.Run("continues without session when no valid session found", func(t *testing.T) {
		// Initialize test database and reset after test
		db.ResetConnection()
		db.SetupTestDB("TestSessionExtractionMiddleware_NoSession")
		defer db.ResetConnection()

		// Set up repositories and services
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		tokenService := createMockTokenService()

		// Create the middleware
		middleware := SessionExtractionMiddleware(sessionStore, tokenService)

		// Create a handler that checks for session in context
		var capturedSession *db.Session
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessionData, exists := r.Context().Value(session.SessionKey).(*db.Session); exists {
				capturedSession = sessionData
			}
			w.WriteHeader(http.StatusOK)
		})

		// Wrap handler with middleware
		wrappedHandler := middleware(handler)

		// Create request without session cookie
		req := httptest.NewRequest("GET", "/api/test", nil)

		// Execute request
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Verify request proceeded normally but no session was found
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Nil(t, capturedSession)
	})

	t.Run("extracts session from bearer token and adds to context", func(t *testing.T) {
		// Initialize test database and reset after test
		db.ResetConnection()
		db.SetupTestDB("TestSessionExtractionMiddleware_BearerToken")
		defer db.ResetConnection()

		// Set up repositories and services
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		tokenService := createMockTokenService()

		// Create the middleware
		middleware := SessionExtractionMiddleware(sessionStore, tokenService)

		// Create a handler that checks for session in context
		var capturedSession *db.Session
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessionData, exists := r.Context().Value(session.SessionKey).(*db.Session); exists {
				capturedSession = sessionData
			}
			w.WriteHeader(http.StatusOK)
		})

		// Wrap handler with middleware
		wrappedHandler := middleware(handler)

		// Create request with bearer token
		req := httptest.NewRequest("GET", "/api/test", nil)
		req.Header.Set("Authorization", "Bearer valid-bearer-token")

		// Execute request
		w := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(w, req)

		// Verify session was extracted and added to context (token auth creates session on the fly)
		assert.Equal(t, http.StatusOK, w.Code)
		require.NotNil(t, capturedSession)
		assert.Equal(t, "test-user-id", capturedSession.UserID)
		assert.Equal(t, "testuser", capturedSession.Username)
	})
}
