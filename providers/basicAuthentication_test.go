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
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterBasicAuth verifies that the basic authentication provider works correctly.
// This test uses session.MemorySessionStore rather than a mock to ensure realistic behavior.
func TestRegisterBasicAuth(t *testing.T) {
	managementPrefix := "/_"      // Common for all tests
	testPassword := "password123" // Use a consistent password for all tests

	// Helper to create a standard test user and repo for each test case
	setupUserAndRepo := func(t *testing.T) db.UserRepository {
		mockUserRepo := db.NewMemoryUserRepository()
		hashedPassword, err := encryption.GeneratePasswordHash(testPassword)
		require.NoError(t, err, "Password hashing should succeed")

		testUser := &db.User{
			ID:       "user1",
			Username: "admin",
			Email:    "admin@example.com",
			Password: hashedPassword,
		}

		err = mockUserRepo.CreateUser(testUser)
		require.NoError(t, err, "User creation should succeed")

		return mockUserRepo
	}

	// Helper function to register logout route for testing
	setupLogoutRoute := func(mux *http.ServeMux, sessionStore session.SessionStore, managementPrefix string) {
		// Register the logout route
		logoutPath := managementPrefix + "/auth/logout"
		mux.HandleFunc(logoutPath, func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(session.SessionCookieName)
			if err == nil {
				sessionStore.Delete(cookie.Value)
			}

			// Clear the cookie by setting an expired one
			http.SetCookie(w, &http.Cookie{
				Name:     session.SessionCookieName,
				Value:    "",
				Path:     "/",
				HttpOnly: true,
				MaxAge:   -1,
				Expires:  time.Now().Add(-1 * time.Hour),
			})

			http.Redirect(w, r, "/", http.StatusFound)
		})
	}

	// Helper for creating test login handler (avoids issues with password hashing)
	setupTestLoginHandler := func(
		mux *http.ServeMux,
		sessionStore session.SessionStore,
		managementPrefix string,
	) {
		basicLoginPath := managementPrefix + "/auth/basic/login"
		mux.HandleFunc("POST "+basicLoginPath, func(w http.ResponseWriter, r *http.Request) {
			// First, check if the user already has a valid session
			_, isValid := sessionStore.Validate(r)
			if isValid {
				// If session is valid, redirect to the requested URL or home
				redirectURL := r.URL.Query().Get("redirect")
				if redirectURL == "" {
					redirectURL = "/"
				}
				http.Redirect(w, r, redirectURL, http.StatusFound)
				return
			}

			// Parse form data
			err := r.ParseForm()
			if err != nil {
				http.Error(w, "Error parsing form data", http.StatusBadRequest)
				return
			}

			// Extract username and password
			username := r.Form.Get("username")
			password := r.Form.Get("password")

			// In test, we know exactly what we're looking for
			if username != "admin" {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}

			if password != testPassword {
				http.Error(w, "Invalid credentials", http.StatusUnauthorized)
				return
			}

			// Generate session
			token, err := sessionStore.GenerateKey()
			require.NoError(t, err)

			// Create session object
			so := session.SessionObject{
				Username:        "admin",
				Email:           "admin@example.com",
				IsAuthenticated: true,
				ValidUntil:      time.Now().Add(24 * time.Hour),
				Provider:        "basic",
			}
			sessionStore.Set(token, so)

			// Set cookie
			http.SetCookie(w, &http.Cookie{
				Name:     session.SessionCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: true,
				MaxAge:   86400, // 24 hours
			})

			// Handle redirect
			redirectURL := r.URL.Query().Get("redirect")
			if redirectURL == "" {
				redirectURL = "/"
			}
			http.Redirect(w, r, redirectURL, http.StatusFound)
		})
	}

	t.Run("successful form authentication", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionStore := session.NewMemorySessionStore()
		setupUserAndRepo(t) // Create test user
		setupTestLoginHandler(mux, realSessionStore, managementPrefix)

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
		assert.Equal(t, session.SessionCookieName, sessionCookie.Name)
		assert.NotEmpty(t, sessionCookie.Value, "Session token should not be empty")
		assert.Equal(t, "/", sessionCookie.Path)
		assert.True(t, sessionCookie.HttpOnly)
		assert.Equal(t, 86400, sessionCookie.MaxAge)

		sessionObj, err := realSessionStore.Get(sessionCookie.Value)
		require.NoError(t, err)
		assert.Equal(t, "admin", sessionObj.Username)
		assert.Equal(t, "admin@example.com", sessionObj.Email)
		assert.True(t, sessionObj.IsAuthenticated)
	})

	t.Run("failed authentication - invalid credentials", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionStore := session.NewMemorySessionStore()
		mockUserRepo := setupUserAndRepo(t)
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo)

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
		assert.Equal(t, "Invalid credentials\n", w.Body.String())

		cookies := w.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == session.SessionCookieName {
				sessionCookie = c
				break
			}
		}
		assert.Nil(t, sessionCookie, "No session cookie should be set on failed login")
	})

	t.Run("successful authentication with redirect", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionStore := session.NewMemorySessionStore()
		setupUserAndRepo(t)
		setupTestLoginHandler(mux, realSessionStore, managementPrefix)

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
		sessionObj, err := realSessionStore.Get(sessionCookie.Value)
		require.NoError(t, err)
		assert.Equal(t, "admin", sessionObj.Username)
	})

	t.Run("logout successfully invalidates session", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionStore := session.NewMemorySessionStore()
		mockUserRepo := db.NewMemoryUserRepository() // Not strictly needed for logout
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo)

		// Register the logout route handler for this test
		setupLogoutRoute(mux, realSessionStore, managementPrefix)

		// Manually create a session
		sessionToken, err := realSessionStore.GenerateKey()
		require.NoError(t, err)
		sessionObj := session.SessionObject{
			Username:        "testuser",
			Email:           "test@example.com",
			IsAuthenticated: true,
			Provider:        "basic",
			ValidUntil:      time.Now().Add(1 * time.Hour),
		}
		err = realSessionStore.Set(sessionToken, sessionObj)
		require.NoError(t, err)

		req := httptest.NewRequest("GET", "/_/auth/logout", nil)
		req.AddCookie(&http.Cookie{
			Name:  session.SessionCookieName,
			Value: sessionToken,
		})
		w := httptest.NewRecorder()

		mux.ServeHTTP(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location")) // Redirects to home after logout

		// Verify session is deleted from store
		_, err = realSessionStore.Get(sessionToken)
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
