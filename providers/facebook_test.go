package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestFacebookProvider_Name(t *testing.T) {
	provider := FacebookProvider{}
	assert.Equal(t, "facebook", provider.Name())
}

func TestFacebookUserDataFetcher_FetchUserData(t *testing.T) {
	t.Run("successful user data fetch", func(t *testing.T) {
		mockResponse := map[string]interface{}{
			"id":         "1234567890",
			"name":       "Test User",
			"email":      "test@example.com",
			"first_name": "Test",
			"last_name":  "User",
			"picture": map[string]interface{}{
				"data": map[string]interface{}{
					"url": "https://example.com/avatar.jpg",
				},
			},
		}

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer mockServer.Close()

		fetcher := &FacebookUserDataFetcher{}
		mockURL := mockServer.URL + "/me?fields=id,name,email,first_name,last_name,picture&access_token="
		userInfo, err := fetchFacebookUserDataWithCustomURL(fetcher, "test-token", mockURL)

		require.NoError(t, err)
		assert.NotNil(t, userInfo)
		assert.Equal(t, "1234567890", userInfo.ID)
		assert.Equal(t, "test@example.com", userInfo.Email)
		assert.Equal(t, "test@example.com", userInfo.Username)
		assert.True(t, userInfo.VerifiedEmail)
		assert.Equal(t, "Test User", userInfo.Name)
		assert.Equal(t, "Test", userInfo.GivenName)
		assert.Equal(t, "User", userInfo.FamilyName)
		assert.Equal(t, "https://example.com/avatar.jpg", userInfo.Picture)
		assert.Equal(t, "facebook", userInfo.Provider)
	})

	t.Run("no email granted", func(t *testing.T) {
		// A real, common case: the user declined the email permission, or
		// their Facebook account has no verified email at all — Graph
		// simply omits the field rather than erroring.
		mockResponse := map[string]interface{}{
			"id":   "1234567890",
			"name": "Test User",
		}

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer mockServer.Close()

		fetcher := &FacebookUserDataFetcher{}
		mockURL := mockServer.URL + "/me?fields=id,name,email,first_name,last_name,picture&access_token="
		userInfo, err := fetchFacebookUserDataWithCustomURL(fetcher, "test-token", mockURL)

		require.NoError(t, err)
		assert.Equal(t, "", userInfo.Email)
		assert.False(t, userInfo.VerifiedEmail)
	})

	t.Run("HTTP error from Facebook API", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}))
		defer mockServer.Close()

		fetcher := &FacebookUserDataFetcher{}
		mockURL := mockServer.URL + "/me?fields=id,name,email,first_name,last_name,picture&access_token="
		userInfo, err := fetchFacebookUserDataWithCustomURL(fetcher, "invalid-token", mockURL)

		assert.Error(t, err)
		assert.Nil(t, userInfo)
		assert.Contains(t, err.Error(), "failed to get user info")
	})
}

// fetchFacebookUserDataWithCustomURL mirrors FacebookUserDataFetcher.FetchUserData
// against an injectable base URL — see google_test.go's identical-purpose
// fetchGoogleUserDataWithCustomURL for why (the real method's URL is fixed
// on purpose).
func fetchFacebookUserDataWithCustomURL(f *FacebookUserDataFetcher, accessToken, baseURL string) (*UserInfo, error) {
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

	var fbUser struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Picture   struct {
			Data struct {
				URL string `json:"url"`
			} `json:"data"`
		} `json:"picture"`
	}
	if err := json.Unmarshal(body, &fbUser); err != nil {
		return nil, err
	}

	return &UserInfo{
		ID:            fbUser.ID,
		Email:         fbUser.Email,
		Username:      fbUser.Email,
		VerifiedEmail: fbUser.Email != "",
		Name:          fbUser.Name,
		GivenName:     fbUser.FirstName,
		FamilyName:    fbUser.LastName,
		Picture:       fbUser.Picture.Data.URL,
		Provider:      "facebook",
	}, nil
}

func TestRegisterFacebookAuth(t *testing.T) {
	t.Run("register with valid configuration", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		userRepo := db.NewDBUserRepository(nil)

		gatewayConfig := &config.GatewayConfig{
			Server:     config.ServerConfig{URL: "http://localhost:8080"},
			Management: config.ManagementConfig{Prefix: "/_"},
			AuthenticationProviders: config.AuthenticationProviders{
				Facebook: config.AuthProviderCredentials{
					ClientId:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
			},
		}

		RegisterFacebookAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/facebook/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)
		location := loginRec.Header().Get("Location")
		assert.Contains(t, location, "facebook.com")
		assert.Contains(t, location, "/dialog/oauth")
		assert.Contains(t, location, "client_id=test-client-id")
	})

	t.Run("skip registration with missing credentials", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		db.SetupTestDB("TestFacebookAuth")
		userRepo := db.NewDBUserRepository(db.GetConnection())

		gatewayConfig := &config.GatewayConfig{
			AuthenticationProviders: config.AuthenticationProviders{
				Facebook: config.AuthProviderCredentials{
					ClientId:     "",
					ClientSecret: "",
				},
			},
		}

		RegisterFacebookAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/facebook/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusNotFound, loginRec.Code)
	})
}

func TestFacebookOAuth2Flow(t *testing.T) {
	t.Run("OAuth2 flow with mock fetcher", func(t *testing.T) {
		db.SetupTestDB("TestFacebookAuth")
		gormDB := db.GetConnection()
		userRepo := db.NewDBUserRepository(gormDB)
		sessionRepo := db.NewSessionRepositoryDB(gormDB)
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)

		gatewayConfig := &config.GatewayConfig{
			Management: config.ManagementConfig{
				Session: config.SessionConfig{SecondsDuration: 86400},
			},
		}

		oauthConfig := &oauth2.Config{
			ClientID:     "test-client-id",
			ClientSecret: "test-client-secret",
			RedirectURL:  "http://localhost:8080/_/auth/facebook/callback",
			Scopes:       []string{"email", "public_profile"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://www.facebook.com/v3.2/dialog/oauth",
				TokenURL: "https://dummy-token-url.com/token", // Won't be called due to mock
			},
		}

		mockFetcher := &MockFacebookUserDataFetcher{
			userInfo: &UserInfo{
				ID:            "facebook123",
				Email:         "testuser@example.com",
				Username:      "testuser@example.com",
				VerifiedEmail: true,
				Name:          "Test User",
				Provider:      "facebook",
			},
		}

		provider := FacebookProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Facebook", userRepo, sessionStore, gatewayConfig)
		authProvider.Fetcher = mockFetcher

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		loginReq := httptest.NewRequest("GET", "/_/auth/facebook/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)

		var stateCookie *http.Cookie
		for _, cookie := range loginRec.Result().Cookies() {
			if cookie.Name == StateCookieName {
				stateCookie = cookie
				break
			}
		}
		require.NotNil(t, stateCookie)
		assert.NotEmpty(t, stateCookie.Value)
	})

	t.Run("OAuth2 callback with invalid state", func(t *testing.T) {
		db.SetupTestDB("TestFacebookAuth")
		gormDB := db.GetConnection()
		userRepo := db.NewDBUserRepository(gormDB)
		sessionRepo := db.NewSessionRepositoryDB(gormDB)
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)

		gatewayConfig := &config.GatewayConfig{
			Management: config.ManagementConfig{
				Session: config.SessionConfig{SecondsDuration: 86400},
			},
		}

		oauthConfig := &oauth2.Config{
			ClientID:    "test-client-id",
			RedirectURL: "http://localhost:8080/_/auth/facebook/callback",
		}

		provider := FacebookProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Facebook", userRepo, sessionStore, gatewayConfig)

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		callbackReq := httptest.NewRequest("GET", "/_/auth/facebook/callback?state=invalid-state&code=test-code", nil)
		callbackReq.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: "different-state",
		})

		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)

		assert.Equal(t, http.StatusUnauthorized, callbackRec.Code)
	})
}

// MockFacebookUserDataFetcher is a mock implementation for testing
type MockFacebookUserDataFetcher struct {
	userInfo *UserInfo
	err      error
}

func (m *MockFacebookUserDataFetcher) FetchUserData(r *http.Request, token *oauth2.Token) (*UserInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.userInfo, nil
}
