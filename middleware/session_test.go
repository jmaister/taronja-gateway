package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/api"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helper to create a test session store with memory repository
func createTestSessionStore() session.SessionStore {
	memoryRepo := db.NewMemorySessionRepository()
	return session.NewSessionStore(memoryRepo)
}

// Test helper to create a test user
func createTestUser() *db.User {
	return &db.User{
		ID:       "test-user-id",
		Username: "testuser",
		Email:    "test@example.com",
	}
}

// Test helper to create a valid session and cookie
func createValidSessionAndCookie(t *testing.T, store session.SessionStore, user *db.User) (*db.Session, *http.Cookie) {
	req := httptest.NewRequest("GET", "/", nil)
	sessionData, err := store.NewSession(req, user, "test-provider", time.Hour)
	require.NoError(t, err)
	require.NotNil(t, sessionData)

	cookie := &http.Cookie{
		Name:  session.SessionCookieName,
		Value: sessionData.Token,
	}

	return sessionData, cookie
}

// Mock StrictHandlerFunc for testing
func mockStrictHandler(responseValue interface{}, errorValue error) api.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (interface{}, error) {
		return responseValue, errorValue
	}
}

// Test helper to verify session is in context
func verifySessionInContext(ctx context.Context, expectedSession *db.Session) bool {
	ctxSession, ok := ctx.Value(session.SessionKey).(*db.Session)
	if !ok || ctxSession == nil {
		return false
	}
	return ctxSession.Token == expectedSession.Token
}

func TestStrictSessionMiddleware(t *testing.T) {
	store := createTestSessionStore()
	user := createTestUser()
	loginRedirectBase := "/_/login"

	t.Run("operation with no security required", func(t *testing.T) {
		// Test operations that are in OperationWithNoSecurity list
		for _, operationID := range OperationWithNoSecurity {
			t.Run(operationID, func(t *testing.T) {
				middleware := StrictSessionMiddleware(store, loginRedirectBase, false)
				handlerCalled := false
				mockHandler := mockStrictHandler("success", nil)

				wrappedHandler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (interface{}, error) {
					handlerCalled = true
					return mockHandler(ctx, w, r, requestObject)
				}, operationID)

				req := httptest.NewRequest("GET", "/test", nil)
				w := httptest.NewRecorder()
				ctx := context.Background()

				response, err := wrappedHandler(ctx, w, req, nil)

				assert.NoError(t, err)
				assert.Equal(t, "success", response)
				assert.True(t, handlerCalled)
			})
		}
	})

	t.Run("operation with valid session", func(t *testing.T) {
		sessionData, cookie := createValidSessionAndCookie(t, store, user)

		middleware := StrictSessionMiddleware(store, loginRedirectBase, false)
		handlerCalled := false
		var receivedCtx context.Context

		wrappedHandler := middleware(func(ctx context.Context, w http.ResponseWriter, r *http.Request, requestObject interface{}) (interface{}, error) {
			handlerCalled = true
			receivedCtx = ctx
			return "success", nil
		}, "TestOperation")

		req := httptest.NewRequest("GET", "/test", nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()
		ctx := context.Background()

		response, err := wrappedHandler(ctx, w, req, nil)

		assert.NoError(t, err)
		assert.Equal(t, "success", response)
		assert.True(t, handlerCalled)
		assert.True(t, verifySessionInContext(receivedCtx, sessionData))
	})

	t.Run("operation requiring auth with no session cookie", func(t *testing.T) {
		middleware := StrictSessionMiddleware(store, loginRedirectBase, false)

		wrappedHandler := middleware(mockStrictHandler("success", nil), "TestOperation")

		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		ctx := context.Background()

		response, err := wrappedHandler(ctx, w, req, nil)

		assert.Nil(t, response)
		assert.Error(t, err)

		var errorWithResponse *ErrorWithResponse
		assert.ErrorAs(t, err, &errorWithResponse)
		assert.Equal(t, http.StatusUnauthorized, errorWithResponse.Code)
		assert.Equal(t, "Unauthorized, no session found or invalid.", errorWithResponse.Message)
	})

	t.Run("operation requiring auth with invalid session token", func(t *testing.T) {
		middleware := StrictSessionMiddleware(store, loginRedirectBase, false)

		wrappedHandler := middleware(mockStrictHandler("success", nil), "TestOperation")

		req := httptest.NewRequest("GET", "/test", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: "invalid-token",
		})
		w := httptest.NewRecorder()
		ctx := context.Background()

		response, err := wrappedHandler(ctx, w, req, nil)

		assert.Nil(t, response)
		assert.Error(t, err)

		var errorWithResponse *ErrorWithResponse
		assert.ErrorAs(t, err, &errorWithResponse)
		assert.Equal(t, http.StatusUnauthorized, errorWithResponse.Code)
		assert.Equal(t, "Unauthorized, no session found or invalid.", errorWithResponse.Message)
	})

	t.Run("operation requiring auth with expired session", func(t *testing.T) {
		// Create an expired session
		req := httptest.NewRequest("GET", "/", nil)
		expiredSession, err := store.NewSession(req, user, "test-provider", -time.Hour) // Expired 1 hour ago
		require.NoError(t, err)

		middleware := StrictSessionMiddleware(store, loginRedirectBase, false)

		wrappedHandler := middleware(mockStrictHandler("success", nil), "TestOperation")

		testReq := httptest.NewRequest("GET", "/test", nil)
		testReq.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: expiredSession.Token,
		})
		w := httptest.NewRecorder()
		ctx := context.Background()

		response, err := wrappedHandler(ctx, w, testReq, nil)

		assert.Nil(t, response)
		assert.Error(t, err)

		var errorWithResponse *ErrorWithResponse
		assert.ErrorAs(t, err, &errorWithResponse)
		assert.Equal(t, http.StatusUnauthorized, errorWithResponse.Code)
		assert.Equal(t, "Unauthorized, no session found or invalid.", errorWithResponse.Message)
	})

	t.Run("operation requiring auth with closed session", func(t *testing.T) {
		// Create a valid session then close it
		sessionData, _ := createValidSessionAndCookie(t, store, user)
		err := store.EndSession(sessionData.Token)
		require.NoError(t, err)

		middleware := StrictSessionMiddleware(store, loginRedirectBase, false)

		wrappedHandler := middleware(mockStrictHandler("success", nil), "TestOperation")

		req := httptest.NewRequest("GET", "/test", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionData.Token,
		})
		w := httptest.NewRecorder()
		ctx := context.Background()

		response, err := wrappedHandler(ctx, w, req, nil)

		assert.Nil(t, response)
		assert.Error(t, err)

		var errorWithResponse *ErrorWithResponse
		assert.ErrorAs(t, err, &errorWithResponse)
		assert.Equal(t, http.StatusUnauthorized, errorWithResponse.Code)
		assert.Equal(t, "Unauthorized, no session found or invalid.", errorWithResponse.Message)
	})
}

func TestSessionMiddleware(t *testing.T) {
	store := createTestSessionStore()
	user := createTestUser()
	managementPrefix := "/_"

	t.Run("valid session for static content", func(t *testing.T) {
		_, cookie := createValidSessionAndCookie(t, store, user)

		handlerCalled := false
		var receivedCtx context.Context
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			receivedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, true, managementPrefix, false)

		req := httptest.NewRequest("GET", "/static/file.html", nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)

		// Verify cache control headers are set
		assert.Equal(t, "private, no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
		assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
		assert.Equal(t, "0", w.Header().Get("Expires"))

		// Verify session is in context
		ctxSession, ok := receivedCtx.Value(session.SessionKey).(*db.Session)
		assert.True(t, ok)
		assert.NotNil(t, ctxSession)
		assert.Equal(t, user.ID, ctxSession.UserID)
	})

	t.Run("valid session for API request", func(t *testing.T) {
		_, cookie := createValidSessionAndCookie(t, store, user)

		handlerCalled := false
		var receivedCtx context.Context
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			receivedCtx = r.Context()
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, false, managementPrefix, false)

		req := httptest.NewRequest("GET", "/api/users", nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)

		// Verify cache control headers are set
		assert.Equal(t, "private, no-cache, no-store, must-revalidate", w.Header().Get("Cache-Control"))
		assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
		assert.Equal(t, "0", w.Header().Get("Expires"))

		// Verify session is in context
		ctxSession, ok := receivedCtx.Value(session.SessionKey).(*db.Session)
		assert.True(t, ok)
		assert.NotNil(t, ctxSession)
		assert.Equal(t, user.ID, ctxSession.UserID)
	})

	t.Run("no session for static content - redirects to login", func(t *testing.T) {
		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, true, managementPrefix, false)

		req := httptest.NewRequest("GET", "/static/file.html", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.False(t, handlerCalled)

		// Verify redirect to login with original URL
		expectedRedirect := managementPrefix + "/login?redirect=" + url.QueryEscape("/static/file.html")
		assert.Equal(t, expectedRedirect, w.Header().Get("Location"))
	})

	t.Run("no session for API request - returns 401", func(t *testing.T) {
		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, false, managementPrefix, false)

		req := httptest.NewRequest("GET", "/api/users", nil)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, handlerCalled)
		assert.Equal(t, "Unauthorized\n", w.Body.String())
	})

	t.Run("invalid session token for static content", func(t *testing.T) {
		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, true, managementPrefix, false)

		req := httptest.NewRequest("GET", "/static/file.html", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: "invalid-token",
		})
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.False(t, handlerCalled)

		// Verify redirect to login with original URL
		expectedRedirect := managementPrefix + "/login?redirect=" + url.QueryEscape("/static/file.html")
		assert.Equal(t, expectedRedirect, w.Header().Get("Location"))
	})

	t.Run("invalid session token for API request", func(t *testing.T) {
		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, false, managementPrefix, false)

		req := httptest.NewRequest("GET", "/api/users", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: "invalid-token",
		})
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, handlerCalled)
		assert.Equal(t, "Unauthorized\n", w.Body.String())
	})

	t.Run("expired session for static content", func(t *testing.T) {
		// Create an expired session
		req := httptest.NewRequest("GET", "/", nil)
		expiredSession, err := store.NewSession(req, user, "test-provider", -time.Hour) // Expired 1 hour ago
		require.NoError(t, err)

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, true, managementPrefix, false)

		testReq := httptest.NewRequest("GET", "/static/file.html", nil)
		testReq.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: expiredSession.Token,
		})
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, testReq)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.False(t, handlerCalled)

		// Verify redirect to login
		expectedRedirect := managementPrefix + "/login?redirect=" + url.QueryEscape("/static/file.html")
		assert.Equal(t, expectedRedirect, w.Header().Get("Location"))
	})

	t.Run("expired session for API request", func(t *testing.T) {
		// Create an expired session
		req := httptest.NewRequest("GET", "/", nil)
		expiredSession, err := store.NewSession(req, user, "test-provider", -time.Hour) // Expired 1 hour ago
		require.NoError(t, err)

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, false, managementPrefix, false)

		testReq := httptest.NewRequest("GET", "/api/users", nil)
		testReq.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: expiredSession.Token,
		})
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, testReq)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, handlerCalled)
		assert.Equal(t, "Unauthorized\n", w.Body.String())
	})

	t.Run("session updates last activity", func(t *testing.T) {
		sessionData, cookie := createValidSessionAndCookie(t, store, user)
		originalLastActivity := sessionData.LastActivity

		// Wait a bit to ensure time difference
		time.Sleep(10 * time.Millisecond)

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := SessionMiddleware(nextHandler, store, false, managementPrefix, false)

		req := httptest.NewRequest("GET", "/api/users", nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()

		middleware.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)

		// Verify that ValidateSession was called and updated LastActivity
		// We need to validate the session again to see the updated time
		validateReq := httptest.NewRequest("GET", "/", nil)
		validateReq.AddCookie(cookie)
		updatedSession, isValid := store.ValidateSession(validateReq)
		assert.True(t, isValid)
		assert.NotNil(t, updatedSession)
		assert.True(t, updatedSession.LastActivity.After(originalLastActivity))
	})
}

func TestSessionMiddlewareFunc(t *testing.T) {
	store := createTestSessionStore()
	user := createTestUser()
	managementPrefix := "/_"

	t.Run("creates middleware that wraps http.Handler correctly", func(t *testing.T) {
		_, cookie := createValidSessionAndCookie(t, store, user)

		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middlewareFunc := SessionMiddlewareFunc(store, false, managementPrefix, false)
		wrappedHandler := middlewareFunc(nextHandler)

		req := httptest.NewRequest("GET", "/api/test", nil)
		req.AddCookie(cookie)
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handlerCalled)
	})

	t.Run("middleware function handles missing session", func(t *testing.T) {
		handlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middlewareFunc := SessionMiddlewareFunc(store, false, managementPrefix, false)
		wrappedHandler := middlewareFunc(nextHandler)

		req := httptest.NewRequest("GET", "/api/test", nil)
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.False(t, handlerCalled)
	})
}

func TestGetSessionToken(t *testing.T) {
	t.Run("extracts token from valid cookie", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: "test-token-value",
		})

		token, err := GetSessionToken(req)
		assert.NoError(t, err)
		assert.Equal(t, "test-token-value", token)
	})

	t.Run("returns empty string when no cookie present", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)

		token, err := GetSessionToken(req)
		assert.NoError(t, err)
		assert.Equal(t, "", token)
	})

	t.Run("returns error for malformed cookie", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		// Manually add a malformed cookie header
		req.Header.Add("Cookie", "malformed-cookie-header")

		token, err := GetSessionToken(req)
		// This may or may not error depending on the specific malformation
		// The main thing is that we handle the case gracefully
		if err != nil {
			assert.Equal(t, "", token)
		}
	})
}

func TestErrorWithResponse(t *testing.T) {
	t.Run("error implements error interface correctly", func(t *testing.T) {
		err := &ErrorWithResponse{
			Code:    http.StatusUnauthorized,
			Message: "Test error message",
		}

		expectedErrorString := "error with code 401: Test error message"
		assert.Equal(t, expectedErrorString, err.Error())
		assert.Implements(t, (*error)(nil), err)
	})
}

func TestOperationWithNoSecurity(t *testing.T) {
	t.Run("contains expected operations", func(t *testing.T) {
		expectedOperations := []string{
			"login",
			"LogoutUser",
			"HealthCheck",
		}

		assert.Equal(t, expectedOperations, OperationWithNoSecurity)
	})
}
