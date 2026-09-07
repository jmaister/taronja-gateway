package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/facebook"
)

type FacebookProvider struct{}

func (p FacebookProvider) Name() string {
	return "facebook"
}

type FacebookUserDataFetcher struct {
	OAuthConfig *oauth2.Config
}

func (f *FacebookUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	// The Graph API's /me only returns the fields explicitly requested.
	// picture comes back as a nested {data: {url: ...}} object, not a plain
	// string, unlike every other provider here.
	reqURL := "https://graph.facebook.com/me?fields=id,name,email,first_name,last_name,picture&access_token=" + url.QueryEscape(accessToken)
	resp, err := http.Get(reqURL)
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

	// Facebook only returns email at all once the user has a verified one
	// on their account and grants the email permission — an empty value
	// here means neither is true, not a fetch failure.
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

// RegisterFacebookAuth configures and registers Facebook OAuth2 authentication
func RegisterFacebookAuth(mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository) {
	if gatewayConfig.AuthenticationProviders.Facebook.ClientId == "" ||
		gatewayConfig.AuthenticationProviders.Facebook.ClientSecret == "" {
		return // Skip if not configured
	}

	oauthConfig := &oauth2.Config{
		ClientID:     gatewayConfig.AuthenticationProviders.Facebook.ClientId,
		ClientSecret: gatewayConfig.AuthenticationProviders.Facebook.ClientSecret,
		RedirectURL:  fmt.Sprintf("%s%s/auth/facebook/callback", gatewayConfig.Server.URL, gatewayConfig.Management.Prefix),
		Scopes:       []string{"email", "public_profile"},
		Endpoint:     facebook.Endpoint,
	}

	provider := FacebookProvider{}
	fetcher := &FacebookUserDataFetcher{OAuthConfig: oauthConfig}

	authProvider := NewAuthenticationProvider(
		oauthConfig,
		provider,
		"Facebook",
		userRepo,
		sessionStore,
		gatewayConfig,
	)
	authProvider.Fetcher = fetcher

	authProvider.RegisterEndpoints(mux)
}
