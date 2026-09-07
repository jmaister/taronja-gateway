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

func TestMicrosoftProvider_Name(t *testing.T) {
	provider := MicrosoftProvider{}
	assert.Equal(t, "microsoft", provider.Name())
}

func TestMicrosoftUserDataFetcher_FetchUserData(t *testing.T) {
	t.Run("successful user data fetch, mail present", func(t *testing.T) {
		mockResponse := map[string]interface{}{
			"id":                "abc-123",
			"mail":              "test@contoso.com",
			"userPrincipalName": "test@contoso.onmicrosoft.com",
			"displayName":       "Test User",
			"givenName":         "Test",
			"surname":           "User",
		}

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer mockServer.Close()

		userInfo, err := fetchMicrosoftUserDataWithCustomURL("test-token", mockServer.URL)

		require.NoError(t, err)
		assert.NotNil(t, userInfo)
		assert.Equal(t, "abc-123", userInfo.ID)
		// mail wins over userPrincipalName when both are present.
		assert.Equal(t, "test@contoso.com", userInfo.Email)
		assert.Equal(t, "test@contoso.com", userInfo.Username)
		assert.True(t, userInfo.VerifiedEmail)
		assert.Equal(t, "Test User", userInfo.Name)
		assert.Equal(t, "Test", userInfo.GivenName)
		assert.Equal(t, "User", userInfo.FamilyName)
		assert.Equal(t, "microsoft", userInfo.Provider)
	})

	t.Run("falls back to userPrincipalName when mail is null", func(t *testing.T) {
		// A real, documented Graph API shape: mailbox-less accounts return
		// "mail": null rather than omitting the field.
		mockResponse := map[string]interface{}{
			"id":                "abc-456",
			"mail":              nil,
			"userPrincipalName": "guest@contoso.onmicrosoft.com",
			"displayName":       "Guest User",
		}

		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mockResponse)
		}))
		defer mockServer.Close()

		userInfo, err := fetchMicrosoftUserDataWithCustomURL("test-token", mockServer.URL)

		require.NoError(t, err)
		assert.Equal(t, "guest@contoso.onmicrosoft.com", userInfo.Email)
		assert.Equal(t, "guest@contoso.onmicrosoft.com", userInfo.Username)
		assert.True(t, userInfo.VerifiedEmail)
	})

	t.Run("HTTP error from Graph API", func(t *testing.T) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
		}))
		defer mockServer.Close()

		userInfo, err := fetchMicrosoftUserDataWithCustomURL("invalid-token", mockServer.URL)

		assert.Error(t, err)
		assert.Nil(t, userInfo)
		assert.Contains(t, err.Error(), "failed to get user info")
	})
}

// fetchMicrosoftUserDataWithCustomURL mirrors MicrosoftUserDataFetcher.FetchUserData
// exactly, but against an injectable base URL instead of the real, hardcoded
// Graph endpoint — the same approach google_test.go's
// fetchGoogleUserDataWithCustomURL uses, for the same reason: the real
// method's URL is fixed on purpose (it's always graph.microsoft.com), so
// exercising the JSON-mapping logic against a mock server needs this
// standalone copy instead.
func fetchMicrosoftUserDataWithCustomURL(accessToken, baseURL string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", baseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
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

	var msUser struct {
		ID                string  `json:"id"`
		Mail              *string `json:"mail"`
		UserPrincipalName string  `json:"userPrincipalName"`
		DisplayName       string  `json:"displayName"`
		GivenName         string  `json:"givenName"`
		Surname           string  `json:"surname"`
	}
	if err := json.Unmarshal(body, &msUser); err != nil {
		return nil, err
	}

	email := msUser.UserPrincipalName
	if msUser.Mail != nil && *msUser.Mail != "" {
		email = *msUser.Mail
	}

	return &UserInfo{
		ID:            msUser.ID,
		Email:         email,
		Username:      email,
		VerifiedEmail: email != "",
		Name:          msUser.DisplayName,
		GivenName:     msUser.GivenName,
		FamilyName:    msUser.Surname,
		Provider:      "microsoft",
	}, nil
}

func TestRegisterMicrosoftAuth(t *testing.T) {
	t.Run("register with valid configuration, common tenant", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		userRepo := db.NewDBUserRepository(nil)

		gatewayConfig := &config.GatewayConfig{
			Server: config.ServerConfig{
				URL: "http://localhost:8080",
			},
			Management: config.ManagementConfig{
				Prefix: "/_",
			},
			AuthenticationProviders: config.AuthenticationProviders{
				Microsoft: config.MicrosoftAuthProviderCredentials{
					ClientId:     "test-client-id",
					ClientSecret: "test-client-secret",
				},
			},
		}

		RegisterMicrosoftAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/microsoft/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)
		location := loginRec.Header().Get("Location")
		// "common" is the default tenant when Tenant is left unset.
		assert.Contains(t, location, "login.microsoftonline.com/common/oauth2/v2.0/authorize")
		assert.Contains(t, location, "client_id=test-client-id")
	})

	t.Run("register with a specific tenant", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		userRepo := db.NewDBUserRepository(nil)

		gatewayConfig := &config.GatewayConfig{
			Server:     config.ServerConfig{URL: "http://localhost:8080"},
			Management: config.ManagementConfig{Prefix: "/_"},
			AuthenticationProviders: config.AuthenticationProviders{
				Microsoft: config.MicrosoftAuthProviderCredentials{
					ClientId:     "test-client-id",
					ClientSecret: "test-client-secret",
					Tenant:       "contoso.onmicrosoft.com",
				},
			},
		}

		RegisterMicrosoftAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/microsoft/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		location := loginRec.Header().Get("Location")
		assert.Contains(t, location, "login.microsoftonline.com/contoso.onmicrosoft.com/oauth2/v2.0/authorize")
	})

	t.Run("skip registration with missing credentials", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		db.SetupTestDB("TestMicrosoftAuth")
		userRepo := db.NewDBUserRepository(db.GetConnection())

		gatewayConfig := &config.GatewayConfig{
			AuthenticationProviders: config.AuthenticationProviders{
				Microsoft: config.MicrosoftAuthProviderCredentials{
					ClientId:     "",
					ClientSecret: "",
				},
			},
		}

		RegisterMicrosoftAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/microsoft/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusNotFound, loginRec.Code)
	})
}

func TestMicrosoftOAuth2Flow(t *testing.T) {
	t.Run("OAuth2 flow with mock fetcher", func(t *testing.T) {
		db.SetupTestDB("TestMicrosoftAuth")
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
			RedirectURL:  "http://localhost:8080/_/auth/microsoft/callback",
			Scopes:       []string{"openid", "profile", "email", "User.Read"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
				TokenURL: "https://dummy-token-url.com/token", // Won't be called due to mock
			},
		}

		mockFetcher := &MockMicrosoftUserDataFetcher{
			userInfo: &UserInfo{
				ID:            "microsoft123",
				Email:         "testuser@contoso.com",
				Username:      "testuser@contoso.com",
				VerifiedEmail: true,
				Name:          "Test User",
				Provider:      "microsoft",
			},
		}

		provider := MicrosoftProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Microsoft", userRepo, sessionStore, gatewayConfig)
		authProvider.Fetcher = mockFetcher

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		loginReq := httptest.NewRequest("GET", "/_/auth/microsoft/login", nil)
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
		db.SetupTestDB("TestMicrosoftAuth")
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
			RedirectURL: "http://localhost:8080/_/auth/microsoft/callback",
		}

		provider := MicrosoftProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Microsoft", userRepo, sessionStore, gatewayConfig)

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		callbackReq := httptest.NewRequest("GET", "/_/auth/microsoft/callback?state=invalid-state&code=test-code", nil)
		callbackReq.AddCookie(&http.Cookie{
			Name:  StateCookieName,
			Value: "different-state",
		})

		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)

		assert.Equal(t, http.StatusUnauthorized, callbackRec.Code)
	})
}

// MockMicrosoftUserDataFetcher is a mock implementation for testing
type MockMicrosoftUserDataFetcher struct {
	userInfo *UserInfo
	err      error
}

func (m *MockMicrosoftUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.userInfo, nil
}
