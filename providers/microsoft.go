package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/microsoft"
)

type MicrosoftProvider struct{}

func (p MicrosoftProvider) Name() string {
	return "microsoft"
}

type MicrosoftUserDataFetcher struct {
	OAuthConfig *oauth2.Config
}

// FetchUserData reads the signed-in user's profile from Microsoft Graph
// (https://graph.microsoft.com/v1.0/me), which needs the User.Read scope
// (requested in RegisterMicrosoftAuth's Scopes).
func (f *MicrosoftUserDataFetcher) FetchUserData(accessToken string) (*UserInfo, error) {
	req, err := http.NewRequest("GET", "https://graph.microsoft.com/v1.0/me", nil)
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

	// Graph's /me response. mail is nullable — some accounts (mailbox-less
	// guest/service accounts) only ever have a userPrincipalName, which is
	// itself an email-shaped identifier and Microsoft's own recommended
	// fallback for exactly this case.
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
		ID:    msUser.ID,
		Email: email,
		// userPrincipalName is already unique per tenant and email-shaped,
		// unlike GitHub's separate login/email — no separate username field
		// exists in Graph's profile to prefer over it.
		Username: email,
		// Graph's /me doesn't expose an explicit "is this verified" flag the
		// way Google's userinfo endpoint does — an organizational or
		// Microsoft account's own identity provider already vouches for it,
		// same assumption Google/GitHub's OAuth logins make.
		VerifiedEmail: email != "",
		Name:          msUser.DisplayName,
		GivenName:     msUser.GivenName,
		FamilyName:    msUser.Surname,
		// Graph exposes a profile photo only via a separate binary endpoint
		// (/me/photo/$value, no plain URL), unlike Google/GitHub's picture
		// URL — left empty rather than fetching and inlining image bytes
		// here.
		Provider: "microsoft",
	}, nil
}

// RegisterMicrosoftAuth configures and registers Microsoft (Entra ID / Azure
// AD) OAuth2 authentication.
func RegisterMicrosoftAuth(mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository) {
	creds := gatewayConfig.AuthenticationProviders.Microsoft
	if creds.ClientId == "" || creds.ClientSecret == "" {
		return // Skip if not configured
	}

	// AzureADEndpoint("") uses the "common" tenant, accepting both personal
	// Microsoft accounts and any organizational (Entra ID) account —
	// config.MicrosoftAuthProviderCredentials.Tenant narrows that to one
	// organization when set.
	oauthConfig := &oauth2.Config{
		ClientID:     creds.ClientId,
		ClientSecret: creds.ClientSecret,
		RedirectURL:  fmt.Sprintf("%s%s/auth/microsoft/callback", gatewayConfig.Server.URL, gatewayConfig.Management.Prefix),
		Scopes:       []string{"openid", "profile", "email", "User.Read"},
		Endpoint:     microsoft.AzureADEndpoint(creds.Tenant),
	}

	provider := MicrosoftProvider{}
	fetcher := &MicrosoftUserDataFetcher{OAuthConfig: oauthConfig}

	authProvider := NewAuthenticationProvider(
		oauthConfig,
		provider,
		"Microsoft",
		userRepo,
		sessionStore,
		gatewayConfig,
	)
	authProvider.Fetcher = fetcher

	authProvider.RegisterEndpoints(mux)
}
