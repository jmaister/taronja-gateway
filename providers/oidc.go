package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
)

// oidcHTTPTimeout bounds each call to an OIDC provider.
const oidcHTTPTimeout = 10 * time.Second

// OIDCProvider is a generic OpenID Connect provider, named by its config.
type OIDCProvider struct {
	name string
}

func (p OIDCProvider) Name() string {
	return p.name
}

// oidcMetadata is the part of the discovery document the gateway uses.
type oidcMetadata struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// oidcDiscovery loads the discovery document on first use rather than at
// startup, so the gateway starts even while the provider is down, and retries
// on every login until it succeeds. After that the result is kept.
type oidcDiscovery struct {
	issuer string
	client *http.Client

	mu       sync.Mutex
	metadata *oidcMetadata
	// onLoad runs once, under mu, with the first metadata loaded.
	onLoad func(*oidcMetadata)
}

func (d *oidcDiscovery) load(ctx context.Context) (*oidcMetadata, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.metadata != nil {
		return d.metadata, nil
	}

	url := strings.TrimSuffix(d.issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: %s", url, resp.Status)
	}

	var metadata oidcMetadata
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&metadata)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", url, err)
	}
	// A different issuer means the URL is wrong or something answers for it
	// that should not (OIDC Discovery 1.0, section 4.3).
	if strings.TrimSuffix(metadata.Issuer, "/") != strings.TrimSuffix(d.issuer, "/") {
		return nil, fmt.Errorf("discovery document at %s is for issuer %q, expected %q", url, metadata.Issuer, d.issuer)
	}
	if metadata.AuthorizationEndpoint == "" || metadata.TokenEndpoint == "" || metadata.UserinfoEndpoint == "" {
		return nil, fmt.Errorf("discovery document at %s lacks the authorization, token or userinfo endpoint", url)
	}

	if d.onLoad != nil {
		d.onLoad(&metadata)
	}
	d.metadata = &metadata
	log.Printf("OIDC provider %s discovered: authorize=%s token=%s userinfo=%s", d.issuer, metadata.AuthorizationEndpoint, metadata.TokenEndpoint, metadata.UserinfoEndpoint)
	return d.metadata, nil
}

// OIDCUserDataFetcher reads the user from the provider's userinfo endpoint.
// The userinfo response comes straight from the provider over the connection
// the access token authorizes, so no ID token signature check is needed.
type OIDCUserDataFetcher struct {
	discovery  *oidcDiscovery
	trustEmail bool
	provider   string
}

// oidcClaims are the standard claims the gateway maps onto a user.
type oidcClaims struct {
	Subject           string          `json:"sub"`
	Email             string          `json:"email"`
	EmailVerified     json.RawMessage `json:"email_verified"`
	PreferredUsername string          `json:"preferred_username"`
	Name              string          `json:"name"`
	GivenName         string          `json:"given_name"`
	FamilyName        string          `json:"family_name"`
	Picture           string          `json:"picture"`
	Locale            string          `json:"locale"`
}

// emailVerified accepts true and "true": some providers send the claim as a string.
func (c oidcClaims) emailVerified() bool {
	value := strings.Trim(string(c.EmailVerified), `"`)
	return value == "true"
}

func (f *OIDCUserDataFetcher) FetchUserData(r *http.Request, token *oauth2.Token) (*UserInfo, error) {
	metadata, err := f.discovery.load(r.Context())
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, metadata.UserinfoEndpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)
	req.Header.Set("Accept", "application/json")
	resp, err := f.discovery.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("userinfo request failed: %s", resp.Status)
	}

	var claims oidcClaims
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&claims)
	if err != nil {
		return nil, fmt.Errorf("decoding userinfo: %w", err)
	}
	if claims.Subject == "" {
		return nil, errors.New("userinfo response has no sub claim")
	}

	return &UserInfo{
		ID:            claims.Subject,
		Email:         claims.Email,
		Username:      claims.PreferredUsername,
		VerifiedEmail: claims.Email != "" && (claims.emailVerified() || f.trustEmail),
		Name:          claims.Name,
		GivenName:     claims.GivenName,
		FamilyName:    claims.FamilyName,
		Picture:       claims.Picture,
		Locale:        claims.Locale,
		Provider:      f.provider,
	}, nil
}

// NewOIDCAuthenticationProvider builds the login flow for one configured OIDC
// provider. Its endpoints are filled in by discovery on first use.
func NewOIDCAuthenticationProvider(cfg config.OIDCProviderConfig, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo repositories.UserRepository, accountsService *accounts.Service) *AuthenticationProvider {
	scopes := cfg.Scopes
	if len(scopes) == 0 {
		scopes = []string{"openid", "email", "profile"}
	}
	oauthConfig := &oauth2.Config{
		ClientID:     cfg.ClientId,
		ClientSecret: cfg.ClientSecret,
		RedirectURL:  fmt.Sprintf("%s%s/auth/%s/callback", gatewayConfig.Server.URL, config.ManagementPrefix, cfg.Name),
		Scopes:       scopes,
	}
	discovery := &oidcDiscovery{
		issuer: cfg.Issuer,
		client: &http.Client{Timeout: oidcHTTPTimeout},
		// Endpoint is written once here, under the discovery lock, and only
		// read after Prepare (which takes the same lock) has returned.
		onLoad: func(m *oidcMetadata) {
			oauthConfig.Endpoint = oauth2.Endpoint{AuthURL: m.AuthorizationEndpoint, TokenURL: m.TokenEndpoint}
			if cfg.ClientSecret == "" {
				// A public client authenticates with PKCE only; it still
				// identifies itself with client_id in the token request.
				oauthConfig.Endpoint.AuthStyle = oauth2.AuthStyleInParams
			}
		},
	}

	ap := NewAuthenticationProvider(oauthConfig, OIDCProvider{name: cfg.Name}, cfg.DisplayLabel(), userRepo, sessionStore, gatewayConfig, accountsService)
	ap.Fetcher = &OIDCUserDataFetcher{discovery: discovery, trustEmail: cfg.TrustEmail, provider: cfg.Name}
	ap.UsePKCE = true
	ap.AllowSignup = cfg.Signup
	ap.Prepare = func(ctx context.Context) error {
		_, err := discovery.load(ctx)
		return err
	}
	return ap
}

// RegisterOIDCAuth registers the login, callback and link endpoints of every
// configured OIDC provider.
func RegisterOIDCAuth(mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo repositories.UserRepository, accountsService *accounts.Service) {
	for _, cfg := range gatewayConfig.AuthenticationProviders.OIDC {
		NewOIDCAuthenticationProvider(cfg, sessionStore, gatewayConfig, userRepo, accountsService).RegisterEndpoints(mux)
	}
}
