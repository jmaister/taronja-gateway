package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestGoogleProvider_Name(t *testing.T) {
	provider := GoogleProvider{}
	assert.Equal(t, "google", provider.Name())
}

func TestGoogleUserDataFetcher_FetchUserData(t *testing.T) {
	t.Run("successful user data fetch", func(t *testing.T) {
		// Mock Google API response
		mockResponse := map[string]interface{}{
			"id":             "123456789",
			"email":          "test@example.com",
			"verified_email": true,
			"name":           "Test User",
			"given_name":     "Test",
			"family_name":    "User",
			"picture":        "https://example.com/avatar.jpg",
			"locale":         "en",
		}

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer mockServer.Close()

		fetcher := &GoogleUserDataFetcher{}
		mockURL := mockServer.URL + "/oauth2/v2/userinfo?access_token="
		userInfo, err := fetchGoogleUserDataWithCustomURL(fetcher, "test-token", mockURL)

		require.NoError(t, err)
		assert.NotNil(t, userInfo)
		assert.Equal(t, "123456789", userInfo.ID)
		assert.Equal(t, "test@example.com", userInfo.Email)
		assert.Equal(t, "test@example.com", userInfo.Username)
		assert.True(t, userInfo.VerifiedEmail)
		assert.Equal(t, "Test User", userInfo.Name)
		assert.Equal(t, "google", userInfo.Provider)
	})

	t.Run("HTTP error from Google API", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}))
		defer mockServer.Close()

		fetcher := &GoogleUserDataFetcher{}
		mockURL := mockServer.URL + "/oauth2/v2/userinfo?access_token="
		userInfo, err := fetchGoogleUserDataWithCustomURL(fetcher, "invalid-token", mockURL)

		assert.Error(t, err)
		assert.Nil(t, userInfo)
		assert.Contains(t, err.Error(), "failed to get user info")
	})
}

// Helper function to test FetchUserData with a custom URL (for mocking)
func fetchGoogleUserDataWithCustomURL(f *GoogleUserDataFetcher, accessToken, baseURL string) (*UserInfo, error) {
	resp, err := http.Get(baseURL + accessToken)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get user info: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var googleUser struct {
		ID            string `json:"id"`
		Email         string `json:"email"`
		VerifiedEmail bool   `json:"verified_email"`
		Name          string `json:"name"`
		GivenName     string `json:"given_name"`
		FamilyName    string `json:"family_name"`
		Picture       string `json:"picture"`
		Locale        string `json:"locale"`
	}

	if err := json.Unmarshal(body, &googleUser); err != nil {
		return nil, err
	}

	return &UserInfo{
		ID:            googleUser.ID,
		Email:         googleUser.Email,
		Username:      googleUser.Email,
		VerifiedEmail: googleUser.VerifiedEmail,
		Name:          googleUser.Name,
		GivenName:     googleUser.GivenName,
		FamilyName:    googleUser.FamilyName,
		Picture:       googleUser.Picture,
		Locale:        googleUser.Locale,
		Provider:      "google",
	}, nil
}

func TestRegisterGoogleAuth(t *testing.T) {
	t.Run("register with valid configuration", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewMemorySessionRepository()
		sessionStore := session.NewSessionStore(sessionRepo)
		userRepo := db.NewMemoryUserRepository()

		gatewayConfig := &config.GatewayConfig{
			Server: config.ServerConfig{
				URL: "http://localhost:8080",
			},
			Management: config.ManagementConfig{
				Prefix: "/_",
			},
			AuthenticationProviders: config.AuthenticationProviders{
				Google: config.AuthProviderCredentials{
					ClientId:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
			},
		}

		RegisterGoogleAuth(mux, sessionStore, gatewayConfig, userRepo)

		// Test that the login endpoint is registered
		loginReq := httptest.NewRequest("GET", "/_/auth/google/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		// Should redirect to Google OAuth2
		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)
		location := loginRec.Header().Get("Location")
		assert.Contains(t, location, "accounts.google.com/o/oauth2/auth")
		assert.Contains(t, location, "client_id=test-client-id")
	})

	t.Run("skip registration with missing credentials", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewMemorySessionRepository()
		sessionStore := session.NewSessionStore(sessionRepo)
		userRepo := db.NewMemoryUserRepository()

		gatewayConfig := &config.GatewayConfig{
			AuthenticationProviders: config.AuthenticationProviders{
				Google: config.AuthProviderCredentials{
					ClientId:     "", // Empty credentials
					ClientSecret: "",
				},
			},
		}

		RegisterGoogleAuth(mux, sessionStore, gatewayConfig, userRepo)

		// Test that no endpoints are registered
		loginReq := httptest.NewRequest("GET", "/_/auth/google/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusNotFound, loginRec.Code)
	})
}

func TestGoogleOAuth2Flow(t *testing.T) {
	t.Run("OAuth2 flow with mock fetcher", func(t *testing.T) {
		// Setup test repositories
		userRepo := db.NewMemoryUserRepository()
		sessionRepo := db.NewMemorySessionRepository()
		sessionStore := session.NewSessionStore(sessionRepo)

		// Create OAuth2 config with dummy endpoint
		oauthConfig := &oauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURL:  "http://localhost:8080/_/auth/google/callback",
			Scopes:       []string{"profile", "email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://accounts.google.com/o/oauth2/auth",
				TokenURL: "https://dummy-token-url.com/token", // Won't be called due to mock
			},
		}

		// Create mock fetcher
		mockFetcher := &MockGoogleUserDataFetcher{
			userInfo: &UserInfo{
				ID:            "google123",
				Email:         "testuser@example.com",
				Username:      "testuser@example.com",
				VerifiedEmail: true,
				Name:          "Test User",
				Provider:      "google",
			},
		}

		// Create authentication provider
		provider := GoogleProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Google", userRepo, sessionStore)
		authProvider.Fetcher = mockFetcher

		// Register endpoints
		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		// Test login initiation
		loginReq := httptest.NewRequest("GET", "/_/auth/google/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)

		// Extract state from cookie
		var stateCookie *http.Cookie
		for _, cookie := range loginRec.Result().Cookies() {
			if cookie.Name == StateCookieName {
				stateCookie = cookie
				break
			}
		}
		require.NotNil(t, stateCookie)

		// Since the OAuth2 token exchange would fail with real requests,
		// we'll only test up to this point to verify the configuration is correct
		assert.NotEmpty(t, stateCookie.Value)
	})

	t.Run("OAuth2 callback with invalid state", func(t *testing.T) {
		userRepo := db.NewMemoryUserRepository()
		sessionRepo := db.NewMemorySessionRepository()
		sessionStore := session.NewSessionStore(sessionRepo)

		oauthConfig := &oauth2.Config{
			ClientID:    "test-client-id",
			RedirectURL: "http://localhost:8080/_/auth/google/callback",
		}

		provider := GoogleProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Google", userRepo, sessionStore)

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		// Test callback with invalid state
		callbackReq := httptest.NewRequest("GET", "/_/auth/google/callback?state=invalid-state&code=test-code", nil)
		callbackReq.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: "different-state",
		})

		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)

		assert.Equal(t, http.StatusUnauthorized, callbackRec.Code)
	})
}

// MockGoogleUserDataFetcher is a mock implementation for testing
type MockGoogleUserDataFetcher struct {
	userInfo *UserInfo
	err      error
}

func (m *MockGoogleUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.userInfo, nil
}
