package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper functions (duplicated from session_test.go for simplicity)
func createTestSessionStoreAdmin() session.SessionStore {
	memoryRepo := db.NewMemorySessionRepository()
	return session.NewSessionStore(memoryRepo)
}

func createMockTokenServiceAdmin() session.TokenService {
	return &mockTokenService{}
}

func TestSessionMiddlewareAdminRequired(t *testing.T) {
	store := createTestSessionStoreAdmin()
	tokenService := createMockTokenServiceAdmin()
	managementPrefix := "/_"

	// Create a regular user
	regularUser := &db.User{
		ID:       "regular-user-id",
		Username: "regularuser",
		Email:    "regular@example.com",
		IsAdmin:  false,
	}

	// Create an admin user
	adminUser := &db.User{
		ID:       "admin1",
		Username: "admin",
		Email:    "admin@example.com",
		IsAdmin:  true,
	}

	t.Run("regular user blocked from admin dashboard", func(t *testing.T) {
		// Create session for regular user
		req := httptest.NewRequest("GET", "/", nil)
		sessionData, err := store.NewSession(req, regularUser, "basic", time.Hour)
		require.NoError(t, err)
		require.NotNil(t, sessionData)
		require.False(t, sessionData.IsAdmin, "Regular user session should not be admin")

		cookie := &http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionData.Token,
		}

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Test with adminRequired = true
		middleware := SessionMiddleware(nextHandler, store, tokenService, true, managementPrefix, true)

		dashboardReq := httptest.NewRequest("GET", "/_/admin/dashboard", nil)
		dashboardReq.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, dashboardReq)

		// Should redirect to login, not call the handler
		assert.Equal(t, http.StatusFound, w.Code)
		assert.False(t, handlerCalled)
		assert.Contains(t, w.Header().Get("Location"), "/_/login")
	})

	t.Run("regular user blocked from admin API endpoint", func(t *testing.T) {
		// Create session for regular user
		req := httptest.NewRequest("GET", "/", nil)
		sessionData, err := store.NewSession(req, regularUser, "basic", time.Hour)
		require.NoError(t, err)
		require.NotNil(t, sessionData)
		require.False(t, sessionData.IsAdmin, "Regular user session should not be admin")

		cookie := &http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionData.Token,
		}

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Test API endpoint with adminRequired = true (isStatic = false)
		middleware := SessionMiddleware(nextHandler, store, tokenService, false, managementPrefix, true)

		apiReq := httptest.NewRequest("GET", "/_/api/admin/users", nil)
		apiReq.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, apiReq)

		// Should return 403 Forbidden for API requests
		assert.Equal(t, http.StatusForbidden, w.Code)
		assert.False(t, handlerCalled)
		assert.Contains(t, w.Body.String(), "Forbidden: Admin access required")
	})

	t.Run("admin user allowed access to admin dashboard", func(t *testing.T) {
		// Create session for admin user
		req := httptest.NewRequest("GET", "/", nil)
		sessionData, err := store.NewSession(req, adminUser, "admin", time.Hour)
		require.NoError(t, err)
		require.NotNil(t, sessionData)
		require.True(t, sessionData.IsAdmin, "Admin user session should be admin")

		cookie := &http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionData.Token,
		}

		handlerCalled := false
		var receivedCtx context.Context
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			receivedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})

		// Test with adminRequired = true
		middleware := SessionMiddleware(nextHandler, store, tokenService, true, managementPrefix, true)

		dashboardReq := httptest.NewRequest("GET", "/_/admin/dashboard", nil)
		dashboardReq.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, dashboardReq)

		// Should allow access
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)

		// Verify session is in context
		ctxSession, ok := receivedCtx.Value(session.SessionKey).(*db.Session)
		assert.True(t, ok)
		assert.NotNil(t, ctxSession)
		assert.Equal(t, adminUser.ID, ctxSession.UserID)
		assert.True(t, ctxSession.IsAdmin)
	})

	t.Run("regular user allowed access to non-admin areas", func(t *testing.T) {
		// Create session for regular user
		req := httptest.NewRequest("GET", "/", nil)
		sessionData, err := store.NewSession(req, regularUser, "basic", time.Hour)
		require.NoError(t, err)
		require.NotNil(t, sessionData)

		cookie := &http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionData.Token,
		}

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Test with adminRequired = false
		middleware := SessionMiddleware(nextHandler, store, tokenService, false, managementPrefix, false)

		regularReq := httptest.NewRequest("GET", "/api/users", nil)
		regularReq.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, regularReq)

		// Should allow access
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)
	})
}
