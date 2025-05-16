package providers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/handlers"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterBasicAuth verifies that the basic authentication provider works correctly.
// This test uses session.MemorySessionStore rather than a mock to ensure realistic behavior.
func TestRegisterBasicAuth(t *testing.T) {
	managementPrefix := "/_"
	testPassword := "password123" // Use a consistent password for all tests

	// Helper to create a standard test user and repo for each test case
	setupUserAndRepo := func(t *testing.T) db.UserRepository {
		mockUserRepo := db.NewMemoryUserRepository()

		testUser := &db.User{
			ID:       "user1",
			Username: "admin",
			Email:    "admin@example.com",
			Password: testPassword,
		}

		err := mockUserRepo.CreateUser(testUser)
		require.NoError(t, err, "User creation should succeed")

		return mockUserRepo
	}

	t.Run("successful form authentication", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionRepo := db.NewMemorySessionRepository()
		realSessionStore := session.NewSessionStoreDB(realSessionRepo)
		mockUserRepo := setupUserAndRepo(t)                                      // Create test user
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo) // CHANGED: Use real registration

		// Create a request with valid form credentials
		formData := url.Values{
			"username": {"admin"},
			"password": {testPassword},
		}
		formBody := formData.Encode()

		req := httptest.NewRequest("POST", "/_/auth/basic/login", strings.NewReader(formBody))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Content-Length", strconv.Itoa(len(formBody)))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))

		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)

		sessionCookie := cookies[0]
		assert.Equal(t, session.SessionCookieName, sessionCookie.Name) // CHANGED
		assert.NotEmpty(t, sessionCookie.Value, "Session token should not be empty")
		assert.Equal(t, "/", sessionCookie.Path)
		assert.True(t, sessionCookie.HttpOnly)
		assert.Equal(t, 86400, sessionCookie.MaxAge)

		sessionObj, err := realSessionRepo.FindSessionByToken(sessionCookie.Value)
		require.NoError(t, err)
		assert.Equal(t, "admin", sessionObj.Username)
		assert.Equal(t, "admin@example.com", sessionObj.Email)
		assert.True(t, sessionObj.IsAuthenticated)
	})

	t.Run("failed authentication - invalid credentials", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionRepo := db.NewMemorySessionRepository()
		realSessionStore := session.NewSessionStoreDB(realSessionRepo) // ADDED
		mockUserRepo := setupUserAndRepo(t)
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo) // CHANGED realSessionRepo to realSessionStore

		formData := url.Values{
			"username": {"admin"},
			"password": {"wrongpassword"},
		}
		formBody := formData.Encode()
		req := httptest.NewRequest("POST", "/_/auth/basic/login", strings.NewReader(formBody))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Content-Length", strconv.Itoa(len(formBody)))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Equal(t, "Invalid credentials\n", w.Body.String()) // Add newline to match actual output

		cookies := w.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == session.SessionCookieName { // CHANGED
				sessionCookie = c
				break
			}
		}
		assert.Nil(t, sessionCookie, "No session cookie should be set on failed login")
	})

	t.Run("successful authentication with redirect", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionRepo := db.NewMemorySessionRepository()
		realSessionStore := session.NewSessionStoreDB(realSessionRepo) // ADDED
		mockUserRepo := setupUserAndRepo(t)
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo) // CHANGED: Use real registration

		formData := url.Values{
			"username": {"admin"},
			"password": {testPassword},
		}
		formBody := formData.Encode()

		req := httptest.NewRequest("POST", "/_/auth/basic/login?redirect=/dashboard", strings.NewReader(formBody))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Content-Length", strconv.Itoa(len(formBody)))
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/dashboard", w.Header().Get("Location"))

		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)
		sessionCookie := cookies[0]
		sessionObj, err := realSessionRepo.FindSessionByToken(sessionCookie.Value)
		require.NoError(t, err)
		assert.Equal(t, "admin", sessionObj.Username)
	})

	t.Run("logout successfully invalidates session", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionRepo := db.NewMemorySessionRepository()
		realSessionStore := session.NewSessionStoreDB(realSessionRepo)
		// For RegisterBasicAuth, a user repo is needed. An empty one is sufficient here
		// as the login functionality itself is not being tested in this logout scenario.
		mockUserRepo := db.NewMemoryUserRepository() // REVERTED: Use a new empty repo.
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo)

		// Register the real logout route handler for this test
		handlers.RegisterLogoutHandler(mux, realSessionStore, managementPrefix)

		// Manually create a session for a different user "testuser"
		sessionToken, err := session.GenerateToken()
		require.NoError(t, err)
		sessionObj := db.Session{
			Username:        "testuser",
			Email:           "test@example.com",
			IsAuthenticated: true,
			Provider:        "basic",
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}
		err = realSessionRepo.CreateSession(sessionToken, &sessionObj)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/_/logout", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionToken,
		})
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location")) // Redirects to home after logout

		// Verify session is deleted from store
		_, err = realSessionRepo.FindSessionByToken(sessionToken)
		assert.Error(t, err, "session has been closed")

		// Verify cookie is expired
		cookies := w.Result().Cookies()
		require.Len(t, cookies, 1)
		logoutCookie := cookies[0]
		assert.Equal(t, session.SessionCookieName, logoutCookie.Name)
		assert.Equal(t, "", logoutCookie.Value) // Value cleared
		assert.True(t, logoutCookie.MaxAge < 0) // MaxAge set to a negative value
	})
}
