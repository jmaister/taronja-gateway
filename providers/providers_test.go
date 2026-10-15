package providers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

var testUserRepo repositories.UserRepository
var testSessionStore session.SessionStore

// TestMain sets up the test database and repositories.
func TestMain(m *testing.M) {
	db.SetupTestDB("TestProviders")
	testUserRepo = repositories.NewDBUserRepository(db.GetConnection())
	testSessionRepo := repositories.NewSessionRepositoryDB(db.GetConnection())
	testSessionStore = session.NewSessionStore(testSessionRepo, 24*time.Hour)
	exitVal := m.Run()
	os.Exit(exitVal)
}

// testAccountsFor builds a real accounts service on the current test database,
// around userRepo.
func testAccountsFor(userRepo repositories.UserRepository) *accounts.Service {
	conn := db.GetConnection()
	return accounts.NewService(accounts.Repositories{
		Users:      userRepo,
		Sessions:   repositories.NewSessionRepositoryDB(conn),
		Tokens:     repositories.NewAuthTokenRepositoryDB(conn),
		Identities: repositories.NewUserIdentityRepositoryDB(conn),
		Events:     repositories.NewAuthEventRepositoryDB(conn),
	}, nil, accounts.Options{})
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
				DurationSeconds: 86400, // 24 hours
			},
		},
	}

	return NewAuthenticationProvider(oauthConfig, provider, "Test Provider", testUserRepo, testSessionStore, gatewayConfig, testAccountsFor(testUserRepo))
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

// TestLogoutCookieSecureFlag_TrustedProxyHTTPS is the regression test for
// the needs-validation item this closes: a deployment that terminates TLS
// upstream of this gateway (a cloud load balancer, another reverse proxy)
// never sets req.TLS on the request this gateway itself receives — only a
// bare req.TLS check (as TestLogoutCookieSecureFlag's own "without TLS"
// case exercises for the opposite, correct case: a genuinely plain-HTTP,
// untrusted-peer request) would incorrectly omit Secure on a connection
// that was actually HTTPS end-to-end. See session.RequestIsSecure's doc
// comment for the full reasoning, including why the claim is only honored
// from a trusted peer.
func TestLogoutCookieSecureFlag_TrustedProxyHTTPS(t *testing.T) {
	authProvider := createTestAuthProvider()

	req := httptest.NewRequest("GET", "/logout", nil)
	req.RemoteAddr = "127.0.0.1:12345" // the trusted-proxy peer session.IsTrustedProxy recognizes
	req.Header.Set("X-Forwarded-Proto", "https")
	req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: "some-token"})

	w := httptest.NewRecorder()
	authProvider.Logout(w, req)

	cookies := w.Result().Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == session.SessionCookieName {
			sessionCookie = cookie
			break
		}
	}
	assert.NotNil(t, sessionCookie)
	assert.True(t, sessionCookie.Secure, "a trusted upstream's X-Forwarded-Proto: https claim should set Secure even with no local req.TLS")
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
		assert.Equal(t, http.SameSiteLaxMode, stateCookie.SameSite, "Lax, not Strict — this cookie must survive the OAuth provider's own cross-site redirect back here")
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

func (m *MockUserDataFetcher) FetchUserData(r *http.Request, token *oauth2.Token) (*UserInfo, error) {
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
				DurationSeconds: 86400, // 24 hours
			},
		},
	}

	authProvider := NewAuthenticationProvider(oauthConfig, provider, "Test Provider", testUserRepo, testSessionStore, gatewayConfig, testAccountsFor(testUserRepo))

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
		// SameSite=Lax (not Strict): this response is itself the arrival
		// leg of a cross-site top-level navigation (the OAuth provider
		// redirecting back here), and Lax — unlike Strict — still lets a
		// cookie set here be sent on the very next same-site request, which
		// is all a session cookie actually needs going forward.
		assert.Equal(t, http.SameSiteLaxMode, sessionCookie.SameSite)

		// Check that redirect cookie is cleared
		assert.NotNil(t, redirectCookie, "Redirect cookie should be cleared")
		assert.Equal(t, "", redirectCookie.Value)
		assert.Equal(t, -1, redirectCookie.MaxAge)
		assert.True(t, redirectCookie.HttpOnly, "the clearing Set-Cookie must match the original's attributes to reliably overwrite it")
		assert.Equal(t, http.SameSiteLaxMode, redirectCookie.SameSite)

		// Verify user was created in repository
		createdUser, err := testUserRepo.FindUserByIdOrUsername("", "", "newuser@example.com")
		assert.NoError(t, err)
		assert.NotNil(t, createdUser)
		assert.Equal(t, "newuser@example.com", createdUser.Email)
		assert.Equal(t, "newuser", createdUser.Username)
		assert.Equal(t, "test", createdUser.Provider)
		assert.True(t, createdUser.EmailConfirmed)
	})

	// TestCallbackWithMockedOAuthFlow/successful_OAuth_callback_flow_rejects_an_open_redirect_cookie
	// is the regression test for the vulnerability: Callback used to read
	// the RedirectUrlCookieName cookie and redirect to its value with no
	// validation at all — and since cookies aren't signed, nothing stops a
	// client from setting an absolute-URL value in it before completing a
	// real OAuth round trip, bouncing the browser to an attacker-controlled
	// page right after a genuine successful login.
	t.Run("successful OAuth callback flow rejects an open redirect cookie", func(t *testing.T) {
		state := "test-state-value-openredirect"
		redirectURL := "https://evil.example/phish"
		code := "test-auth-code-openredirect"

		mockFetcher.userInfo.Email = "openredirect-user@example.com"
		mockFetcher.userInfo.Username = "openredirectuser"

		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)
		req.AddCookie(&http.Cookie{Name: StateCookieName, Value: state})
		req.AddCookie(&http.Cookie{Name: RedirectUrlCookieName, Value: redirectURL})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"), "an absolute-URL redirect cookie must be rejected, not forwarded to the client")
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

	// An unverified email from the provider must neither log in nor create an
	// account: an unusable account would still claim the address.
	t.Run("OAuth callback rejects login when the provider reports an unverified email", func(t *testing.T) {
		state := "test-state-value-unverified"
		code := "test-auth-code-unverified"

		mockFetcher.userInfo.ID = "oauth-user-unverified"
		mockFetcher.userInfo.Email = "unverified-user@example.com"
		mockFetcher.userInfo.Username = "unverifieduser"
		mockFetcher.userInfo.VerifiedEmail = false
		defer func() { mockFetcher.userInfo.VerifiedEmail = true }() // restore for any test added after this one

		req := httptest.NewRequest("GET", "/_/auth/test/callback?state="+state+"&code="+code, nil)
		req.AddCookie(&http.Cookie{Name: StateCookieName, Value: state})

		w := httptest.NewRecorder()
		authProvider.Callback(w, req)

		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/_/login?error=email_not_verified", w.Header().Get("Location"))
		for _, cookie := range w.Result().Cookies() {
			assert.NotEqual(t, session.SessionCookieName, cookie.Name, "no session must be issued for an unverified-email login")
		}

		_, err := testUserRepo.FindUserByIdOrUsername("", "", "unverified-user@example.com")
		assert.Error(t, err, "no account is created")
	})
}

// callbackWith runs a callback for profile, with extra cookies on the request.
func callbackWith(t *testing.T, ap *AuthenticationProvider, fetcher *MockUserDataFetcher, info UserInfo, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	fetcher.userInfo = &info
	req := httptest.NewRequest("GET", "/_/auth/test/callback?state=s&code=c", nil)
	req.AddCookie(&http.Cookie{Name: StateCookieName, Value: "s"})
	for _, c := range cookies {
		req.AddCookie(c)
	}
	w := httptest.NewRecorder()
	ap.Callback(w, req)
	return w
}

func TestCallbackLinksAccounts(t *testing.T) {
	mockOAuthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"mock-access-token","token_type":"Bearer","expires_in":3600}`))
	}))
	defer mockOAuthServer.Close()

	oauthConfig := &oauth2.Config{
		ClientID:    "id",
		RedirectURL: "http://localhost:8080/_/auth/test/callback",
		Endpoint:    oauth2.Endpoint{TokenURL: mockOAuthServer.URL + "/token"},
	}
	gatewayConfig := &config.GatewayConfig{Management: config.ManagementConfig{Session: config.SessionConfig{DurationSeconds: 3600}}}
	ap := NewAuthenticationProvider(oauthConfig, NewSimpleAuthProvider("test"), "Test", testUserRepo, testSessionStore, gatewayConfig, testAccountsFor(testUserRepo))
	fetcher := &MockUserDataFetcher{}
	ap.Fetcher = fetcher

	owner := &db.User{Email: "linker@example.com", Username: "linker", Password: "pw-12345678", EmailConfirmed: true}
	require.NoError(t, testUserRepo.CreateUser(owner))
	ownerSession, err := testSessionStore.NewSession(nil, owner, "basic", time.Hour)
	require.NoError(t, err)

	t.Run("an existing account with the same proven email is linked on login", func(t *testing.T) {
		w := callbackWith(t, ap, fetcher, UserInfo{ID: "link-auto", Email: "linker@example.com", VerifiedEmail: true})
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/", w.Header().Get("Location"))
		identity, err := repositories.NewUserIdentityRepositoryDB(db.GetConnection()).FindByProviderSubject("test", "link-auto")
		require.NoError(t, err)
		assert.Equal(t, owner.ID, identity.UserID)
	})

	t.Run("an account the email may have been squatted on is not linked", func(t *testing.T) {
		squatted := &db.User{Email: "squatted@example.com", Username: "squatted", Password: "pw-12345678"}
		require.NoError(t, testUserRepo.CreateUser(squatted))
		w := callbackWith(t, ap, fetcher, UserInfo{ID: "link-squat", Email: "squatted@example.com", VerifiedEmail: true})
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, "/_/login?error=link_required", w.Header().Get("Location"))
	})

	t.Run("the account page links an account with a different email", func(t *testing.T) {
		intent := &http.Cookie{Name: LinkIntentCookieName, Value: accounts.CSRFToken(ownerSession.Token)}
		sessionCookie := &http.Cookie{Name: session.SessionCookieName, Value: ownerSession.Token}
		w := callbackWith(t, ap, fetcher, UserInfo{ID: "link-explicit", Email: "work@corp.example", VerifiedEmail: true}, intent, sessionCookie)
		assert.Equal(t, http.StatusFound, w.Code)
		assert.Equal(t, AccountPath+"?msg=linked", w.Header().Get("Location"))
		for _, c := range w.Result().Cookies() {
			if c.Name == session.SessionCookieName {
				t.Fatal("linking keeps the current session; it does not log in again")
			}
		}

		w = callbackWith(t, ap, fetcher, UserInfo{ID: "link-explicit", Email: "work@corp.example", VerifiedEmail: true})
		assert.Equal(t, "/", w.Header().Get("Location"), "the linked account now logs in as the owner")
	})

	t.Run("a link intent from another session is refused", func(t *testing.T) {
		intent := &http.Cookie{Name: LinkIntentCookieName, Value: accounts.CSRFToken("someone-else")}
		sessionCookie := &http.Cookie{Name: session.SessionCookieName, Value: ownerSession.Token}
		w := callbackWith(t, ap, fetcher, UserInfo{ID: "link-forged", Email: "x@corp.example", VerifiedEmail: true}, intent, sessionCookie)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("Link requires a session and its CSRF token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/_/auth/test/link", strings.NewReader("csrf=wrong"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: ownerSession.Token})
		w := httptest.NewRecorder()
		ap.Link(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)

		req = httptest.NewRequest(http.MethodPost, "/_/auth/test/link", strings.NewReader("csrf="+accounts.CSRFToken(ownerSession.Token)))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: session.SessionCookieName, Value: ownerSession.Token})
		w = httptest.NewRecorder()
		ap.Link(w, req)
		assert.Equal(t, http.StatusTemporaryRedirect, w.Code, "goes to the provider")
		var intentSet bool
		for _, c := range w.Result().Cookies() {
			if c.Name == LinkIntentCookieName && c.Value == accounts.CSRFToken(ownerSession.Token) {
				intentSet = true
			}
		}
		assert.True(t, intentSet)
	})
}
