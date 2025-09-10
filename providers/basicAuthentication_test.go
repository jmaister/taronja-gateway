package providers

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/encryption"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegisterBasicAuth verifies that the basic authentication provider works correctly.
// This test uses session.MemorySessionStore rather than a mock to ensure realistic behavior.
func TestRegisterBasicAuth(t *testing.T) {
	managementPrefix := "/_"
	testPassword := "password123" // Use a consistent password for all tests

	// Helper to create a test config with admin disabled by default
	createTestConfig := func() *config.GatewayConfig {
		return &config.GatewayConfig{
			Management: config.ManagementConfig{
				Prefix: managementPrefix,
				Session: config.SessionConfig{
					SecondsDuration: 86400, // 24 hours
				},
				Admin: config.AdminConfig{
					Enabled:  false,
					Username: "",
					Password: "",
				},
			},
		}
	}

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
		realSessionStore := session.NewSessionStore(realSessionRepo, 24*time.Hour)
		mockUserRepo := setupUserAndRepo(t) // Create test user
		testConfig := createTestConfig()
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo, testConfig) // CHANGED: Use real registration

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
		realSessionStore := session.NewSessionStore(realSessionRepo, 24*time.Hour) // ADDED
		mockUserRepo := setupUserAndRepo(t)
		testConfig := createTestConfig()
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo, testConfig) // CHANGED realSessionRepo to realSessionStore

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
		realSessionStore := session.NewSessionStore(realSessionRepo, 24*time.Hour) // ADDED
		mockUserRepo := setupUserAndRepo(t)
		testConfig := createTestConfig()
		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo, testConfig) // CHANGED: Use real registration

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

	t.Run("successful admin authentication from config", func(t *testing.T) {
		// Per-test setup
		mux := http.NewServeMux()
		realSessionRepo := db.NewMemorySessionRepository()
		realSessionStore := session.NewSessionStore(realSessionRepo, 24*time.Hour)
		mockUserRepo := setupUserAndRepo(t) // Regular user repo (admin won't be found here)

		// Generate proper hash for the admin password
		adminPassword := "password123"
		hashedPassword, err := encryption.GeneratePasswordHash(adminPassword)
		require.NoError(t, err)

		// Create config with admin enabled
		testConfig := &config.GatewayConfig{
			Management: config.ManagementConfig{
				Prefix: managementPrefix,
				Admin: config.AdminConfig{
					Enabled:  true,
					Username: "configadmin",
					Password: hashedPassword,
				},
			},
		}

		// Ensure admin user exists in repository (simulating gateway initialization)
		err = mockUserRepo.EnsureAdminUser("configadmin", "admin@example.com", hashedPassword)
		require.NoError(t, err)

		RegisterBasicAuth(mux, realSessionStore, managementPrefix, mockUserRepo, testConfig)

		formData := url.Values{
			"username": {"configadmin"},
			"password": {adminPassword},
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

		sessionObj, err := realSessionRepo.FindSessionByToken(sessionCookie.Value)
		require.NoError(t, err)
		assert.Equal(t, "configadmin", sessionObj.Username)
		assert.Equal(t, "admin@example.com", sessionObj.Email)
		assert.True(t, sessionObj.IsAuthenticated)
	})

}
