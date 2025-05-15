package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

type GoogleProvider struct{}

func (p GoogleProvider) Name() string {
	return "google"
}

type GoogleUserDataFetcher struct {
	OAuthConfig *oauth2.Config
}

func (f *GoogleUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	// Make a request to the Google userinfo API using the access token
	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)
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

	// Parse the response
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

	// Convert to our UserInfo struct
	return &UserInfo{
		ID:            googleUser.ID,
		Email:         googleUser.Email,
		Username:      googleUser.Email, // Use email as username for Google
		VerifiedEmail: googleUser.VerifiedEmail,
		Name:          googleUser.Name,
		GivenName:     googleUser.GivenName,
		FamilyName:    googleUser.FamilyName,
		Picture:       googleUser.Picture,
		Locale:        googleUser.Locale,
		Provider:      "google",
	}, nil
}

// RegisterGoogleAuth configures and registers Google OAuth2 authentication
func RegisterGoogleAuth(mux *http.ServeMux, sessionRepo db.SessionRepository, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository) {
	if gatewayConfig.AuthenticationProviders.Google.ClientId == "" ||
		gatewayConfig.AuthenticationProviders.Google.ClientSecret == "" {
		return // Skip if not configured
	}

	// Create Google OAuth2 config
	oauthConfig := &oauth2.Config{
		ClientID:     gatewayConfig.AuthenticationProviders.Google.ClientId,
		ClientSecret: gatewayConfig.AuthenticationProviders.Google.ClientSecret,
		RedirectURL:  fmt.Sprintf("%s%s/auth/google/callback", gatewayConfig.Server.URL, gatewayConfig.Management.Prefix),
		Scopes:       []string{"https://www.googleapis.com/auth/userinfo.profile", "https://www.googleapis.com/auth/userinfo.email"},
		Endpoint:     google.Endpoint,
	}

	// Create the provider components
	provider := GoogleProvider{}
	fetcher := &GoogleUserDataFetcher{OAuthConfig: oauthConfig}

	// Create the authentication provider
	authProvider := NewAuthenticationProvider(
		oauthConfig,
		provider,
		"Google",
		userRepo,
		sessionRepo,
	)

	// Set the fetcher
	authProvider.Fetcher = fetcher

	// Register endpoints
	authProvider.RegisterEndpoints(mux)
}
