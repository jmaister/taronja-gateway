package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
)

type GithubProvider struct{}

func (p GithubProvider) Name() string {
	return "github"
}

type GithubUserDataFetcher struct {
	OAuthConfig *oauth2.Config
}

func (f *GithubUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	// Create request to GitHub API
	req, err := http.NewRequest("GET", "https://api.github.com/user", nil)
	if err != nil {
		return nil, err
	}

	// Set authorization header
	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	// Make the request
	client := &http.Client{}
	resp, err := client.Do(req)
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
	var githubUser struct {
		ID        int    `json:"id"`
		Login     string `json:"login"`
		Name      string `json:"name"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.Unmarshal(body, &githubUser); err != nil {
		return nil, err
	}

	// GitHub might not expose the user's email directly, so we need to fetch it separately
	email := githubUser.Email
	if email == "" {
		email, err = fetchGitHubEmail(accessToken)
		if err != nil {
			// Not a critical error, just log it
			fmt.Printf("Warning: Could not fetch GitHub user email: %v\n", err)
		}
	}

	// Extract name parts (GitHub doesn't separate them)
	var givenName, familyName string
	if githubUser.Name != "" {
		nameParts := strings.SplitN(githubUser.Name, " ", 2)
		givenName = nameParts[0]
		if len(nameParts) > 1 {
			familyName = nameParts[1]
		}
	}

	// Convert to our UserInfo struct
	return &UserInfo{
		ID:            fmt.Sprintf("%d", githubUser.ID),
		Email:         email,
		Username:      githubUser.Login,
		VerifiedEmail: email != "",
		Name:          githubUser.Name,
		GivenName:     givenName,
		FamilyName:    familyName,
		Picture:       githubUser.AvatarURL,
		Provider:      "github",
	}, nil
}

// fetchGitHubEmail gets the primary email from GitHub API
func fetchGitHubEmail(accessToken string) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/user/emails", nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Authorization", "token "+accessToken)
	req.Header.Set("Accept", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to get emails: %s", resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}

	if err := json.Unmarshal(body, &emails); err != nil {
		return "", err
	}

	// Find the primary email
	for _, email := range emails {
		if email.Primary && email.Verified {
			return email.Email, nil
		}
	}

	// If no primary verified email found, return the first verified one
	for _, email := range emails {
		if email.Verified {
			return email.Email, nil
		}
	}

	// If no verified email found, return empty string
	return "", fmt.Errorf("no verified email found")
}

// RegisterGithubAuth configures and registers GitHub OAuth2 authentication
func RegisterGithubAuth(mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository, userLoginRepo db.UserLoginRepository) {
	if gatewayConfig.AuthenticationProviders.Github.ClientId == "" ||
		gatewayConfig.AuthenticationProviders.Github.ClientSecret == "" {
		return // Skip if not configured
	}

	// Create GitHub OAuth2 config
	oauthConfig := &oauth2.Config{
		ClientID:     gatewayConfig.AuthenticationProviders.Github.ClientId,
		ClientSecret: gatewayConfig.AuthenticationProviders.Github.ClientSecret,
		RedirectURL:  fmt.Sprintf("%s%s/auth/github/callback", gatewayConfig.Server.URL, gatewayConfig.Management.Prefix),
		Scopes:       []string{"read:user", "user:email"},
		Endpoint:     github.Endpoint,
	}

	// Create the provider components
	provider := GithubProvider{}
	fetcher := &GithubUserDataFetcher{OAuthConfig: oauthConfig}

	// Create the authentication provider
	authProvider := NewAuthenticationProvider(
		oauthConfig,
		provider,
		"GitHub",
		userRepo,
		userLoginRepo,
		sessionStore,
	)

	// Set the fetcher
	authProvider.Fetcher = fetcher

	// Register endpoints
	authProvider.RegisterEndpoints(mux)
}
