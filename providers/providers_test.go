package providers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"golang.org/x/oauth2"
)

var testUserRepo db.UserRepository
var testSessionStore session.SessionStore

// TestMain sets up the test database and repositories.
func TestMain(m *testing.M) {
	db.InitForTest()
	testUserRepo = db.NewDBUserRepository(db.GetConnection())
	testSessionRepo := db.NewSessionRepositoryDB()
	testSessionStore = session.NewSessionStore(testSessionRepo, 24*time.Hour)
	exitVal := m.Run()
	os.Exit(exitVal)
}

func createTestAuthProvider() *AuthenticationProvider {
	provider := NewSimpleAuthProvider("test")
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/_/auth/test/callback",
		Scopes:       []string{"email", "profile"},
	}

	// Create mock gateway config
	gatewayConfig := &config.GatewayConfig{
		Management: config.ManagementConfig{
			Session: config.SessionConfig{
				SecondsDuration: 86400, // 24 hours
			},
		},
	}

	return NewAuthenticationProvider(oauthConfig, provider, "Test Provider", testUserRepo, testSessionStore, gatewayConfig)
}

func TestLogoutWithValidSession(t *testing.T) {
	authProvider := createTestAuthProvider()

	// Create a test user and session
	user := &db.User{
		ID:             "test-user-logout",
		Username:       "testuser",
		Email:          "test@example.com",
		EmailConfirmed: true,
	}
	err := testUserRepo.CreateUser(user)
	assert.NoError(t, err)

	// Create a session
	sessionReq := httptest.NewRequest("GET", "/", nil)
	sessionObj, err := testSessionStore.NewSession(sessionReq, user, "test", time.Hour)
	assert.NoError(t, err)
	assert.NotNil(t, sessionObj)

	// Verify session exists and is valid
	validateReq := httptest.NewRequest("GET", "/", nil)
	validateReq.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: sessionObj.Token})
	_, isValid := testSessionStore.ValidateSession(validateReq)
	assert.True(t, isValid)

	// Create logout request with session cookie
	req := httptest.NewRequest("GET", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: sessionObj.Token})

	w := httptest.NewRecorder()
	authProvider.Logout(w, req)

	// Check response
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/", w.Header().Get("Location"))

	// Check cache control headers
	assert.Equal(t, "no-store, no-cache, must-revalidate, post-check=0, pre-check=0", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Equal(t, "0", w.Header().Get("Expires"))

	// Check that session cookie is cleared
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	assert.NotNil(t, sessionCookie)
	assert.Equal(t, "", sessionCookie.Value)
	assert.Equal(t, -1, sessionCookie.MaxAge)
	assert.True(t, sessionCookie.HttpOnly)

	// Verify session is ended in store
	validateReq2 := httptest.NewRequest("GET", "/", nil)
	validateReq2.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: sessionObj.Token})
	_, isValidAfterLogout := testSessionStore.ValidateSession(validateReq2)
	assert.False(t, isValidAfterLogout)
}

func TestLogoutWithoutSessionCookie(t *testing.T) {
	authProvider := createTestAuthProvider()

	// Create logout request without session cookie
	req := httptest.NewRequest("GET", "/logout", nil)
	w := httptest.NewRecorder()

	authProvider.Logout(w, req)

	// Check response
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/", w.Header().Get("Location"))

	// Check cache control headers are still set
	assert.Equal(t, "no-store, no-cache, must-revalidate, post-check=0, pre-check=0", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Equal(t, "0", w.Header().Get("Expires"))

	// Should not set any session cookie since none existed
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	assert.Nil(t, sessionCookie)
}

func TestLogoutWithInvalidSessionToken(t *testing.T) {
	authProvider := createTestAuthProvider()

	// Create logout request with invalid session token
	req := httptest.NewRequest("GET", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: "invalid-token"})

	w := httptest.NewRecorder()
	authProvider.Logout(w, req)

	// Check response
	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/", w.Header().Get("Location"))

	// Check cache control headers
	assert.Equal(t, "no-store, no-cache, must-revalidate, post-check=0, pre-check=0", w.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", w.Header().Get("Pragma"))
	assert.Equal(t, "0", w.Header().Get("Expires"))

	// Check that session cookie is cleared anyway
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	assert.NotNil(t, sessionCookie)
	assert.Equal(t, "", sessionCookie.Value)
	assert.Equal(t, -1, sessionCookie.MaxAge)
	assert.True(t, sessionCookie.HttpOnly)
}

func TestLogoutCookieSecureFlag(t *testing.T) {
	authProvider := createTestAuthProvider()

	// Test with TLS (secure connection)
	req := httptest.NewRequest("GET", "/logout", nil)
	req.TLS = &tls.ConnectionState{} // Mock TLS connection
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: "some-token"})

	w := httptest.NewRecorder()
	authProvider.Logout(w, req)

	// Check that secure flag is set when TLS is present
	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	assert.NotNil(t, sessionCookie)
	assert.True(t, sessionCookie.Secure)

	// Test without TLS (insecure connection)
	req2 := httptest.NewRequest("GET", "/logout", nil)
	req2.TLS = nil // No TLS
	req2.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: "some-token"})

	w2 := httptest.NewRecorder()
	authProvider.Logout(w2, req2)

	// Check that secure flag is not set when TLS is absent
	cookies2 := w2.Result().Cookies()
	var sessionCookie2 *http.Cookie
	for _, cookie := range cookies2 {
		if cookie.Name == session.SessionCookieName {
			sessionCookie2 = cookie
			break
		}
	}
	assert.NotNil(t, sessionCookie2)
	assert.False(t, sessionCookie2.Secure)
}

func TestLogoutMultipleSessionsForUser(t *testing.T) {
	authProvider := createTestAuthProvider()

	// Create a test user
	user := &db.User{
		ID:             "test-user-multiple",
		Username:       "testuser-multiple",
		Email:          "testmultiple@example.com",
		EmailConfirmed: true,
	}
	err := testUserRepo.CreateUser(user)
	assert.NoError(t, err)

	// Create multiple sessions for the same user
	sessionReq := httptest.NewRequest("GET", "/", nil)
	session1, err := testSessionStore.NewSession(sessionReq, user, "test", time.Hour)
	assert.NoError(t, err)

	session2, err := testSessionStore.NewSession(sessionReq, user, "test", time.Hour)
	assert.NoError(t, err)

	// Verify both sessions are valid
	validateReq1 := httptest.NewRequest("GET", "/", nil)
	validateReq1.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: session1.Token})
	_, isValid1 := testSessionStore.ValidateSession(validateReq1)
	assert.True(t, isValid1)

	validateReq2 := httptest.NewRequest("GET", "/", nil)
	validateReq2.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: session2.Token})
	_, isValid2 := testSessionStore.ValidateSession(validateReq2)
	assert.True(t, isValid2)

	// Logout with first session
	req := httptest.NewRequest("GET", "/logout", nil)
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: session1.Token})

	w := httptest.NewRecorder()
	authProvider.Logout(w, req)

	// Check that first session is ended
	validateReq1After := httptest.NewRequest("GET", "/", nil)
	validateReq1After.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: session1.Token})
	_, isValid1After := testSessionStore.ValidateSession(validateReq1After)
	assert.False(t, isValid1After)

	// Check that second session is still valid
	validateReq2After := httptest.NewRequest("GET", "/", nil)
	validateReq2After.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: session2.Token})
	_, isValid2After := testSessionStore.ValidateSession(validateReq2After)
	assert.True(t, isValid2After)
}

func TestCallbackWithStateCookieAndRedirectUrlCookie(t *testing.T) {
	authProvider := createTestAuthProvider()

	// Create a mock fetcher for user data
	mockFetcher := &MockUserDataFetcher{
		userInfo: &UserInfo{
			ID:            "oauth-user-123",
			Email:         "oauth@example.com",
			Username:      "oauthuser",
			VerifiedEmail: true,
			Name:          "OAuth User",
			GivenName:     "OAuth",
			FamilyName:    "User",
			Picture:       "https://example.com/avatar.jpg",
			Locale:        "en",
			Provider:      "test",
		},
		err: nil,
	}
	authProvider.Fetcher = mockFetcher

	// Create a mock OAuth2 config that won't actually make HTTP calls
	authProvider.OAuthConfig = &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/_/auth/test/callback",
		Scopes:       []string{"email", "profile"},
	}

	t.Run("successful callback with valid state and redirect URL cookies", func(t *testing.T) {
		// Generate a state value
		state := "test-state-value"
		redirectURL := "/dashboard"
		code := "test-auth-code"

		// Create request with query parameters
		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		// Add state cookie
		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		// Add redirect URL cookie
		req.AddCookie(&http.Cookie{
			Name:  RedirectUrlCookieName,
			Value: redirectURL,
		})

		w := httptest.NewRecorder()

		// Mock the OAuth2 token exchange by creating a custom context
		// Since we can't easily mock oauth2.Config.Exchange, we'll test the parts we can control
		// and create a separate test for the OAuth exchange flow
		authProvider.Callback(w, req)

		// Note: This test will fail at the token exchange step because we can't easily mock it
		// The OAuth2 library makes actual HTTP calls. In a real implementation, you might:
		// 1. Use dependency injection for the OAuth2 config
		// 2. Use httptest.Server to mock the OAuth provider
		// 3. Or test the individual components separately

		// For now, we expect an error at the token exchange step
		assert.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, w.Body.String(), "Failed to exchange auth code")
	})

	t.Run("callback with invalid state cookie", func(t *testing.T) {
		state := "test-state-value"
		invalidState := "different-state-value"
		code := "test-auth-code"

		// Create request with query parameters
		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		// Add invalid state cookie (different from query parameter)
		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: invalidState,
		})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// Should return unauthorized due to state mismatch
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid OAuth2 state")
	})

	t.Run("callback with missing state cookie", func(t *testing.T) {
		state := "test-state-value"
		code := "test-auth-code"

		// Create request with query parameters but no state cookie
		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// Should return unauthorized due to missing state cookie
		assert.Equal(t, http.StatusUnauthorized, w.Code)
		assert.Contains(t, w.Body.String(), "Invalid OAuth2 state")
	})

	t.Run("callback clears state cookie after validation", func(t *testing.T) {
		state := "test-state-value"
		code := "test-auth-code"

		// Create request with query parameters
		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		// Add state cookie
		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// Check that state cookie is cleared (set to empty with MaxAge -1)
		cookies := w.Result().Cookies()
		var stateCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == StateCookieName {
				stateCookie = cookie
				break
			}
		}

		assert.NotNil(t, stateCookie, "State cookie should be present in response")
		assert.Equal(t, "", stateCookie.Value, "State cookie value should be empty")
		assert.Equal(t, -1, stateCookie.MaxAge, "State cookie should be set to expire immediately")
		assert.True(t, stateCookie.HttpOnly, "State cookie should be HttpOnly")
		assert.Equal(t, "/", stateCookie.Path, "State cookie path should be /")
	})

	t.Run("callback with redirect URL cookie", func(t *testing.T) {
		// This test verifies that the redirect URL cookie logic is in place
		// Even though the OAuth exchange will fail, we can verify the cookie handling
		state := "test-state-value"
		redirectURL := "/custom-dashboard"
		code := "test-auth-code"

		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		req.AddCookie(&http.Cookie{
			Name:  RedirectUrlCookieName,
			Value: redirectURL,
		})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// The test will fail at OAuth exchange, but we can verify state cookie handling
		// Check that state cookie is cleared
		cookies := w.Result().Cookies()
		var stateCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == StateCookieName {
				stateCookie = cookie
				break
			}
		}

		assert.NotNil(t, stateCookie, "State cookie should be cleared")
		assert.Equal(t, "", stateCookie.Value)
		assert.Equal(t, -1, stateCookie.MaxAge)
	})

	t.Run("callback sets secure flag based on TLS", func(t *testing.T) {
		state := "test-state-value"
		code := "test-auth-code"

		// Test with TLS
		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)
		req.TLS = &tls.ConnectionState{} // Mock TLS connection
		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// Check that state cookie has secure flag set
		cookies := w.Result().Cookies()
		var stateCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == StateCookieName {
				stateCookie = cookie
				break
			}
		}

		assert.NotNil(t, stateCookie)
		assert.True(t, stateCookie.Secure, "State cookie should have Secure flag when TLS is present")

		// Test without TLS
		req2 := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)
		req2.TLS = nil // No TLS
		req2.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		w2 := httptest.NewRecorder()
		authProvider.Callback(w2, req2)

		// Check that state cookie does not have secure flag set
		cookies2 := w2.Result().Cookies()
		var stateCookie2 *http.Cookie
		for _, cookie := range cookies2 {
			if cookie.Name == StateCookieName {
				stateCookie2 = cookie
				break
			}
		}

		assert.NotNil(t, stateCookie2)
		assert.False(t, stateCookie2.Secure, "State cookie should not have Secure flag when TLS is absent")
	})
}

// MockUserDataFetcher implements UserDataFetcher for testing
type MockUserDataFetcher struct {
	userInfo *UserInfo
	err      error
}

func (m *MockUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.userInfo, nil
}

func TestCallbackWithMockedOAuthFlow(t *testing.T) {
	// This test demonstrates how to test the full OAuth flow with a mock server
	// Create a mock OAuth2 provider server
	mockOAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			// Mock token exchange endpoint
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"mock-access-token","token_type":"Bearer","expires_in":3600}`))
		}
	}))
	defer mockOAuthServer.Close()

	// Create auth provider with mock OAuth config
	provider := NewSimpleAuthProvider("test")
	oauthConfig := &oauth2.Config{
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret",
		RedirectURL:  "http://localhost:8080/_/auth/test/callback",
		Scopes:       []string{"email", "profile"},
		Endpoint: oauth2.Endpoint{
			TokenURL: mockOAuthServer.URL + "/token",
		},
	}

	// Create mock gateway config
	gatewayConfig := &config.GatewayConfig{
		Management: config.ManagementConfig{
			Session: config.SessionConfig{
				SecondsDuration: 86400, // 24 hours
			},
		},
	}

	authProvider := NewAuthenticationProvider(oauthConfig, provider, "Test Provider", testUserRepo, testSessionStore, gatewayConfig)

	// Set mock fetcher
	mockFetcher := &MockUserDataFetcher{
		userInfo: &UserInfo{
			ID:            "oauth-user-456",
			Email:         "newuser@example.com",
			Username:      "newuser",
			VerifiedEmail: true,
			Name:          "New User",
			GivenName:     "New",
			FamilyName:    "User",
			Picture:       "https://example.com/newavatar.jpg",
			Locale:        "en",
			Provider:      "test",
		},
		err: nil,
	}
	authProvider.Fetcher = mockFetcher

	t.Run("successful OAuth callback flow with new user", func(t *testing.T) {
		state := "test-state-value"
		redirectURL := "/success"
		code := "test-auth-code"

		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		// Add required cookies
		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		req.AddCookie(&http.Cookie{
			Name:  RedirectUrlCookieName,
			Value: redirectURL,
		})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// Should redirect to the redirect URL after successful authentication
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, redirectURL, w.Header().Get("Location"))

		// Check that session cookie is set
		cookies := w.Result().Cookies()
		var sessionCookie *http.Cookie
		var redirectCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == session.SessionCookieName {
				sessionCookie = cookie
			}
			if cookie.Name == RedirectUrlCookieName {
				redirectCookie = cookie
			}
		}

		assert.NotNil(t, sessionCookie, "Session cookie should be set")
		assert.NotEmpty(t, sessionCookie.Value, "Session token should not be empty")
		assert.Equal(t, "/", sessionCookie.Path)
		assert.True(t, sessionCookie.HttpOnly)
		assert.Equal(t, 86400, sessionCookie.MaxAge) // 24 hours

		// Check that redirect cookie is cleared
		assert.NotNil(t, redirectCookie, "Redirect cookie should be cleared")
		assert.Equal(t, "", redirectCookie.Value)
		assert.Equal(t, -1, redirectCookie.MaxAge)

		// Verify user was created in repository
		createdUser, err := testUserRepo.FindUserByIdOrUsername("", "", "newuser@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, createdUser)
		assert.Equal(t, "newuser@example.com", createdUser.Email)
		assert.Equal(t, "newuser", createdUser.Username)
		assert.Equal(t, "test", createdUser.Provider)
		assert.True(t, createdUser.EmailConfirmed)
	})

	t.Run("successful OAuth callback flow without redirect URL cookie", func(t *testing.T) {
		state := "test-state-value-2"
		code := "test-auth-code-2"

		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)

		// Add only state cookie, no redirect URL cookie
		req.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: state,
		})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		// Should redirect to default "/" when no redirect URL cookie is present
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))

		// Check that session cookie is set
		cookies := w.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, cookie := range cookies {
			if cookie.Name == session.SessionCookieName {
				sessionCookie = cookie
				break
			}
		}

		assert.NotNil(t, sessionCookie, "Session cookie should be set")
		assert.NotEmpty(t, sessionCookie.Value)
	})
}
