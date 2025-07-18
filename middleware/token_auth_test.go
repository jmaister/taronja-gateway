package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/auth"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTokenAuthMiddleware(t *testing.T) {
	// Setup
	userRepo := db.NewMemoryUserRepository()
	tokenRepo := db.NewTokenRepositoryMemory()
	tokenService := auth.NewTokenService(tokenRepo, userRepo)
	middleware := NewTokenAuthMiddleware(tokenService)

	// Create test users
	regularUser := &db.User{
		ID:       "user-123",
		Username: "testuser",
		Email:    "test@example.com",
		Name:     "Test User",
		Provider: "regular",
	}
	err := userRepo.CreateUser(regularUser)
	require.NoError(t, err)

	adminUser := &db.User{
		ID:       "admin-123",
		Username: "admin",
		Email:    "admin@example.com",
		Name:     "Admin User",
		Provider: db.AdminProvider,
	}
	err = userRepo.CreateUser(adminUser)
	require.NoError(t, err)

	// Generate tokens
	regularToken, _, err := tokenService.GenerateToken(regularUser.ID, "Regular Token", nil, nil, "test", nil)
	require.NoError(t, err)

	adminToken, _, err := tokenService.GenerateToken(adminUser.ID, "Admin Token", nil, nil, "test", nil)
	require.NoError(t, err)

	t.Run("GetAuthorizationToken", func(t *testing.T) {
		// Test with valid Bearer token
		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)

		token, err := GetAuthorizationToken(req)
		require.NoError(t, err)
		assert.Equal(t, regularToken, token)

		// Test with no Authorization header
		req = httptest.NewRequest("GET", "/test", nil)
		token, err = GetAuthorizationToken(req)
		require.NoError(t, err)
		assert.Empty(t, token)

		// Test with invalid format
		req = httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Invalid format")
		token, err = GetAuthorizationToken(req)
		require.NoError(t, err)
		assert.Empty(t, token)
	})

	t.Run("TokenAuthMiddlewareFunc_Success", func(t *testing.T) {
		handler := middleware.TokenAuthMiddlewareFunc(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check that session is set in context
			sessionObj := r.Context().Value(session.SessionKey)
			assert.NotNil(t, sessionObj)

			session, ok := sessionObj.(*db.Session)
			assert.True(t, ok)
			assert.Equal(t, regularUser.ID, session.UserID)
			assert.Equal(t, regularUser.Username, session.Username)
			assert.True(t, session.IsAuthenticated)

			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("TokenAuthMiddlewareFunc_NoToken", func(t *testing.T) {
		handler := middleware.TokenAuthMiddlewareFunc(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("TokenAuthMiddlewareFunc_InvalidToken", func(t *testing.T) {
		handler := middleware.TokenAuthMiddlewareFunc(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("TokenAuthMiddlewareFunc_AdminRequired_RegularUser", func(t *testing.T) {
		handler := middleware.TokenAuthMiddlewareFunc(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Error("Handler should not be called")
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("TokenAuthMiddlewareFunc_AdminRequired_AdminUser", func(t *testing.T) {
		handler := middleware.TokenAuthMiddlewareFunc(true)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check that session is set in context
			sessionObj := r.Context().Value(session.SessionKey)
			assert.NotNil(t, sessionObj)

			session, ok := sessionObj.(*db.Session)
			assert.True(t, ok)
			assert.Equal(t, adminUser.ID, session.UserID)
			assert.True(t, session.IsAdmin)

			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()

		handler.ServeHTTP(w, req)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("StrictTokenAuthMiddleware_Success", func(t *testing.T) {
		strictHandler := middleware.StrictTokenAuthMiddleware(false)

		handlerFunc := strictHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (responseObject interface{}, err error) {
			// Check that session is set in context
			sessionObj := ctx.Value(session.SessionKey)
			assert.NotNil(t, sessionObj)

			session, ok := sessionObj.(*db.Session)
			assert.True(t, ok)
			assert.Equal(t, regularUser.ID, session.UserID)

			return map[string]string{"status": "ok"}, nil
		}, "TestOperation")

		req := httptest.NewRequest("GET", "/test", nil)
		req.Header.Set("Authorization", "Bearer "+regularToken)
		w := httptest.NewRecorder()

		result, err := handlerFunc(context.Background(), w, req, nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("StrictTokenAuthMiddleware_NoSecurityRequired", func(t *testing.T) {
		strictHandler := middleware.StrictTokenAuthMiddleware(false)

		handlerFunc := strictHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (responseObject interface{}, err error) {
			return map[string]string{"status": "ok"}, nil
		}, "HealthCheck") // HealthCheck is in the no-security list

		req := httptest.NewRequest("GET", "/health", nil)
		// No Authorization header
		w := httptest.NewRecorder()

		result, err := handlerFunc(context.Background(), w, req, nil)
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("StrictTokenAuthMiddleware_Unauthorized", func(t *testing.T) {
		strictHandler := middleware.StrictTokenAuthMiddleware(false)

		handlerFunc := strictHandler(func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (responseObject interface{}, err error) {
			t.Error("Handler should not be called")
			return nil, nil
		}, "TestOperation")

		req := httptest.NewRequest("GET", "/test", nil)
		// No Authorization header
		w := httptest.NewRecorder()

		result, err := handlerFunc(context.Background(), w, req, nil)
		assert.Error(t, err)
		assert.Nil(t, result)

		errorWithResponse, ok := err.(*ErrorWithResponse)
		assert.True(t, ok)
		assert.Equal(t, http.StatusUnauthorized, errorWithResponse.Code)
	})
}
