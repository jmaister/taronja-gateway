package providers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/jmaister/taronja-gateway/accounts"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/db/repositories"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
)

// More providers can be added here in the future: https://pkg.go.dev/golang.org/x/oauth2/endpoints

const RedirectUrlCookieName = "redirect_url"
const StateCookieName = "OAuthState"

// PKCEVerifierCookieName holds the PKCE code verifier between Login and Callback.
const PKCEVerifierCookieName = "tg_pkce_verifier"

// UserInfo is a struct that holds the user information that is returned by the OAuth2 provider.
type UserInfo struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	Username      string `json:"username"`
	VerifiedEmail bool   `json:"verified_email"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
	Provider      string `json:"provider"`
}

// UserDataFetcher turns a completed OAuth2 token exchange into UserInfo. The
// request is passed for providers that need data from the callback itself.
type UserDataFetcher interface {
	FetchUserData(r *http.Request, token *oauth2.Token) (*UserInfo, error)
}

// Define the AuthProvider interface
type AuthProvider interface {
	Name() string
}

// RegisterProviders registers all enabled authentication providers. ctx bounds
// any background goroutine a provider starts during registration.
func RegisterProviders(ctx context.Context, mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo repositories.UserRepository, accountsService *accounts.Service) {
	log.Printf("Registering authentication providers...")

	if gatewayConfig.AuthenticationProviders.Basic.Enabled || gatewayConfig.Management.Admin.Enabled {
		log.Printf("Registering Basic Authentication provider")
		RegisterBasicAuth(mux, sessionStore, config.ManagementPrefix, userRepo, gatewayConfig, accountsService)
	}

	if gatewayConfig.AuthenticationProviders.Github.ClientId != "" &&
		gatewayConfig.AuthenticationProviders.Github.ClientSecret != "" {
		log.Printf("Registering GitHub Authentication provider")
		RegisterGithubAuth(mux, sessionStore, gatewayConfig, userRepo, accountsService)
	} else {
		log.Printf("GitHub Authentication provider not configured, skipping registration")
	}

	if gatewayConfig.AuthenticationProviders.Google.ClientId != "" &&
		gatewayConfig.AuthenticationProviders.Google.ClientSecret != "" {
		log.Printf("Registering Google Authentication provider")
		RegisterGoogleAuth(mux, sessionStore, gatewayConfig, userRepo, accountsService)
	} else {
		log.Printf("Google Authentication provider not configured, skipping registration")
	}

	if len(gatewayConfig.AuthenticationProviders.OIDC) > 0 {
		log.Printf("Registering %d OIDC Authentication provider(s)", len(gatewayConfig.AuthenticationProviders.OIDC))
		RegisterOIDCAuth(mux, sessionStore, gatewayConfig, userRepo, accountsService)
	}

	// Microsoft, Facebook, and Apple registration lives on the
	// wip/microsoft-apple-facebook-auth branch — moved out of this branch
	// untested (see that branch's log for why), to come back to later.
}

type SimpleAuthProvider struct {
	name string
}

func (p SimpleAuthProvider) Name() string {
	return p.name
}

func NewSimpleAuthProvider(name string) *SimpleAuthProvider {
	return &SimpleAuthProvider{name: name}
}

// AuthenticationProvider manages OAuth2 authentication flows.
// It now uses repositories.SessionRepository.
type AuthenticationProvider struct {
	Provider      AuthProvider
	LongName      string
	OAuthConfig   *oauth2.Config
	Fetcher       UserDataFetcher
	UserRepo      repositories.UserRepository
	SessionStore  session.SessionStore
	GatewayConfig *config.GatewayConfig
	// Accounts maps the external login to a user (finding, linking or
	// creating one) and records the activity log.
	Accounts *accounts.Service

	// ClientSecretFunc, when set, regenerates OAuthConfig.ClientSecret before every
	// token exchange, for providers whose secret is short-lived. Otherwise nil.
	ClientSecretFunc func() (string, error)

	// ResponseMode, when set, is sent as the OAuth2 response_mode
	// authorization parameter. Apple requires "form_post" whenever
	// name/email scopes are requested — its callback then arrives as a
	// POST form body instead of GET query params (Callback below reads
	// state/code via r.FormValue, which transparently covers both).
	// Every other provider here leaves this empty for the default,
	// GET-based redirect.
	ResponseMode string

	// UsePKCE adds a PKCE (S256) challenge to the authorization request and
	// the verifier to the token exchange.
	UsePKCE bool

	// Prepare, when set, runs before the provider's endpoints are used (for
	// OIDC: discovery). An error answers 503, so a provider that is down does
	// not break the rest of the gateway.
	Prepare func(ctx context.Context) error
}

func NewOauth2Config(authProvider AuthProvider, providerCreds *config.AuthProviderCredentials, baseUrl string, endpoint oauth2.Endpoint) *oauth2.Config {
	redirectUrl, _ := url.JoinPath(baseUrl, "/_/auth/"+authProvider.Name()+"/callback")
	return &oauth2.Config{
		RedirectURL:  redirectUrl,
		ClientID:     providerCreds.ClientId,
		ClientSecret: providerCreds.ClientSecret,
		Scopes:       []string{"email", "profile"}, // Common scopes
		Endpoint:     endpoint,
	}
}

// NewAuthenticationProvider creates a new AuthenticationProvider.
// It now accepts repositories.SessionRepository.
func NewAuthenticationProvider(oauthConfig *oauth2.Config, provider AuthProvider, longName string, ur repositories.UserRepository, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, accountsService *accounts.Service) *AuthenticationProvider {
	// baseUrl is not directly needed here if redirectURL is in oauthConfig
	log.Printf("Registering %s Auth Provider. Redirecting to URL=%s", longName, oauthConfig.RedirectURL)

	return &AuthenticationProvider{
		Provider:      provider,
		LongName:      longName,
		OAuthConfig:   oauthConfig,
		UserRepo:      ur,
		SessionStore:  sessionStore,
		GatewayConfig: gatewayConfig,
		Accounts:      accountsService,
	}
}

// Login initiates the OAuth2 login flow
func (ap *AuthenticationProvider) Login(w http.ResponseWriter, r *http.Request) {
	ap.startAuth(w, r, r.URL.Query().Get("redirect"))
}

func (ap *AuthenticationProvider) startAuth(w http.ResponseWriter, r *http.Request, redirect string) {
	if !ap.prepare(w, r) {
		return
	}
	state, err := generateState()
	if err != nil {
		log.Printf("Error generating state: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	var opts []oauth2.AuthCodeOption
	if ap.ResponseMode != "" {
		opts = append(opts, oauth2.SetAuthURLParam("response_mode", ap.ResponseMode))
	}
	if ap.UsePKCE {
		verifier := oauth2.GenerateVerifier()
		opts = append(opts, oauth2.S256ChallengeOption(verifier))
		http.SetCookie(w, &http.Cookie{
			Name:     PKCEVerifierCookieName,
			Value:    verifier,
			Path:     "/",
			HttpOnly: true,
			Secure:   session.RequestIsSecure(r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   300,
		})
	}
	authCodeURL := ap.OAuthConfig.AuthCodeURL(state, opts...)

	// Get redirect URL from query parameters, sanitized to a same-origin
	// path before it's ever stored — see session.SanitizeRedirectPath's
	// doc comment for why: this cookie survives a real round trip through
	// the OAuth provider and back, so Callback below must be able to trust
	// whatever it finds in it.
	originalURL := session.SanitizeRedirectPath(redirect)

	// SameSite=Lax, not Strict: the provider's redirect back is a cross-site top-level
	// navigation, and Strict would drop these cookies and break every OAuth login.
	http.SetCookie(w, &http.Cookie{
		Name:     RedirectUrlCookieName,
		Value:    originalURL,
		Path:     "/",
		HttpOnly: true,
		Secure:   session.RequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Store the state
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   session.RequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
	})

	// Redirect to authorization URL
	http.Redirect(w, r, authCodeURL, http.StatusTemporaryRedirect)
}

// Callback handles the OAuth2 callback.
// Uses repositories.SessionRepository methods.
func (ap *AuthenticationProvider) Callback(w http.ResponseWriter, r *http.Request) {
	// r.FormValue reads from the URL query for every provider's GET
	// callback, and additionally from a POST body for Apple's
	// response_mode=form_post callback — one code path covers both, no
	// need to branch on r.Method.
	if !ap.prepare(w, r) {
		return
	}
	if err := r.ParseForm(); err != nil {
		log.Printf("Error parsing callback request: %v", err)
		http.Error(w, "Invalid callback request", http.StatusBadRequest)
		return
	}
	state := r.FormValue("state")
	code := r.FormValue("code")

	// Retrieve the state from cookie
	stateCookie, err := r.Cookie(StateCookieName)
	if err != nil || stateCookie.Value != state {
		log.Printf("Invalid state or missing state cookie")
		http.Error(w, "Invalid OAuth2 state", http.StatusUnauthorized)
		return
	}

	// Clear state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   session.RequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete immediately
	})

	// oauthConfig starts out as the provider's shared config and is only
	// ever replaced below, never mutated in place: ap.OAuthConfig is one
	// instance shared by every concurrent Callback call for this provider,
	// so writing a freshly-generated secret directly onto it would race
	// with other in-flight callbacks reading or overwriting the same
	// field. A per-request copy keeps each call's secret to itself.
	oauthConfig := ap.OAuthConfig
	if ap.ClientSecretFunc != nil {
		secret, err := ap.ClientSecretFunc()
		if err != nil {
			log.Printf("Error generating client secret for %s: %v", ap.Provider.Name(), err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		cfgCopy := *ap.OAuthConfig
		cfgCopy.ClientSecret = secret
		oauthConfig = &cfgCopy
	}

	// Exchange code for token
	var exchangeOpts []oauth2.AuthCodeOption
	if ap.UsePKCE {
		verifierCookie, err := r.Cookie(PKCEVerifierCookieName)
		if err != nil || verifierCookie.Value == "" {
			log.Printf("Missing PKCE verifier cookie for %s", ap.Provider.Name())
			http.Error(w, "Login expired, please try again", http.StatusBadRequest)
			return
		}
		clearCookie(w, r, PKCEVerifierCookieName)
		exchangeOpts = append(exchangeOpts, oauth2.VerifierOption(verifierCookie.Value))
	}
	token, err := oauthConfig.Exchange(r.Context(), code, exchangeOpts...)
	if err != nil {
		log.Printf("Error exchanging code for token: %v", err)
		http.Error(w, "Failed to exchange auth code", http.StatusInternalServerError)
		return
	}

	// Fetch user data using the token
	userInfo, err := ap.Fetcher.FetchUserData(r, token)
	if err != nil {
		log.Printf("Error loading user data from provider %s: %v", ap.Provider.Name(), err)
		http.Error(w, "Error loading user data from provider", http.StatusInternalServerError)
		return
	}

	profile := accounts.ExternalProfile{
		Provider:      ap.Provider.Name(),
		Subject:       userInfo.ID,
		Email:         userInfo.Email,
		EmailVerified: userInfo.VerifiedEmail,
		Username:      userInfo.Username,
		Name:          userInfo.Name,
		GivenName:     userInfo.GivenName,
		FamilyName:    userInfo.FamilyName,
		Picture:       userInfo.Picture,
		Locale:        userInfo.Locale,
	}
	client := accounts.ClientFromRequest(r)

	user, err := ap.Accounts.ResolveExternalLogin(profile, client)
	switch {
	case errors.Is(err, accounts.ErrLinkRequired):
		http.Redirect(w, r, config.ManagementPrefix+"/login?error=link_required", http.StatusFound)
		return
	case errors.Is(err, accounts.ErrEmailNotVerified):
		http.Redirect(w, r, config.ManagementPrefix+"/login?error=email_not_verified", http.StatusFound)
		return
	case err != nil:
		log.Printf("Error resolving %s login for %s: %v", ap.Provider.Name(), userInfo.Email, err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	_, err = StartSession(w, r, user, ap.Provider.Name(), ap.SessionStore, ap.GatewayConfig)
	if err != nil {
		log.Printf("Error creating session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	redirectURL := "/"
	redirectCookie, err := r.Cookie(RedirectUrlCookieName)
	if err == nil && redirectCookie.Value != "" {
		// Sanitized again here, not just when Login first set the cookie:
		// cookies aren't signed, so nothing stops a client from editing its
		// own before completing the OAuth round trip.
		redirectURL = session.SanitizeRedirectPath(redirectCookie.Value)
		// Clear the redirect cookie — same HttpOnly/Secure/SameSite as when
		// Login first set it, not just Name/Path/MaxAge: a Set-Cookie
		// clearing one out needs matching attributes to reliably overwrite
		// the original in every browser, not risk leaving a stale duplicate
		// behind under a different attribute combination.
		http.SetCookie(w, &http.Cookie{
			Name:     RedirectUrlCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   session.RequestIsSecure(r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// prepare runs ap.Prepare, answering 503 if it fails.
func (ap *AuthenticationProvider) prepare(w http.ResponseWriter, r *http.Request) bool {
	if ap.Prepare == nil {
		return true
	}
	err := ap.Prepare(r.Context())
	if err != nil {
		log.Printf("%s login is unavailable: %v", ap.LongName, err)
		http.Error(w, ap.LongName+" login is unavailable right now. Try again later or use another login method.", http.StatusServiceUnavailable)
		return false
	}
	return true
}

// StartSession creates a session for user and sets the session cookie.
func StartSession(w http.ResponseWriter, r *http.Request, user *db.User, providerName string, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig) (*db.Session, error) {
	sessionObj, err := sessionStore.NewSession(r, user, providerName, gatewayConfig.Management.Session.GetDuration())
	if err != nil {
		return nil, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     session.SessionCookieName,
		Value:    sessionObj.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   session.RequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(gatewayConfig.Management.Session.GetDuration().Seconds()),
	})
	return sessionObj, nil
}

// clearCookie deletes a cookie set with this package's usual attributes.
func clearCookie(w http.ResponseWriter, r *http.Request, name string) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   session.RequestIsSecure(r),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

// Logout handles the logout process.
// Uses repositories.SessionRepository.DeleteSession.
func (ap *AuthenticationProvider) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(session.SessionCookieName)
	if err == nil && cookie != nil {
		_ = ap.SessionStore.EndSession(cookie.Value) // End the session
		http.SetCookie(w, &http.Cookie{
			Name:     session.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   session.RequestIsSecure(r),
			SameSite: http.SameSiteLaxMode,
			MaxAge:   -1,
		})
	}
	// Add cache control headers to prevent browser caching
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	http.Redirect(w, r, "/", http.StatusFound) // Or a configured logout redirect URL
}

// GetLoginPath returns the path for login endpoint
func (ap *AuthenticationProvider) GetLoginPath() string {
	return "/_/auth/" + ap.Provider.Name() + "/login"
}

// GetCallbackPath returns the path for callback endpoint
func (ap *AuthenticationProvider) GetCallbackPath() string {
	return "/_/auth/" + ap.Provider.Name() + "/callback"
}

// RegisterEndpoints registers the login and callback handlers
func (ap *AuthenticationProvider) RegisterEndpoints(mux *http.ServeMux) {
	mux.HandleFunc(ap.GetLoginPath(), ap.Login)
	mux.HandleFunc(ap.GetCallbackPath(), ap.Callback)
	log.Printf("Registered OAuth2 endpoints for %s provider (%s, %s)", ap.LongName, ap.GetLoginPath(), ap.GetCallbackPath())
}

func generateState() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

func SplitRoles(roles string) []string {
	if roles == "" {
		return []string{}
	}
	return strings.Split(roles, ",")
}
