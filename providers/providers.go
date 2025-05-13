package providers

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
)

// More providers can be added here in the future: https://pkg.go.dev/golang.org/x/oauth2/endpoints

const RedirectUrlCookieName = "redirect_url"
const StateCookieName = "OAuthState"

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

type UserDataFetcher interface {
	FetchUserData(accessToken string) (*UserInfo, error)
}

// Define the AuthProvider interface
type AuthProvider interface {
	Name() string
}

// RegisterProviders registers all enabled authentication providers in the gateway
// Each provider is only registered if it's configured in the gateway config
func RegisterProviders(mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository) {
	log.Printf("Registering authentication providers...")

	// Register Basic Authentication if enabled
	if gatewayConfig.AuthenticationProviders.Basic.Enabled {
		log.Printf("Registering Basic Authentication provider")
		RegisterBasicAuth(mux, sessionStore, gatewayConfig.Management.Prefix, userRepo)
	}

	// Register GitHub Authentication if configured
	if gatewayConfig.AuthenticationProviders.Github.ClientId != "" &&
		gatewayConfig.AuthenticationProviders.Github.ClientSecret != "" {
		log.Printf("Registering GitHub Authentication provider")
		RegisterGithubAuth(mux, sessionStore, gatewayConfig, userRepo)
	} else {
		log.Printf("GitHub Authentication provider not configured, skipping registration")
	}

	// Register Google Authentication if configured
	if gatewayConfig.AuthenticationProviders.Google.ClientId != "" &&
		gatewayConfig.AuthenticationProviders.Google.ClientSecret != "" {
		log.Printf("Registering Google Authentication provider")
		RegisterGoogleAuth(mux, sessionStore, gatewayConfig, userRepo)
	} else {
		log.Printf("Google Authentication provider not configured, skipping registration")
	}

	// More providers can be added here in the future
}

// Simple implementation of AuthProvider
type SimpleAuthProvider struct {
	name string
}

func (p SimpleAuthProvider) Name() string {
	return p.name
}

func NewSimpleAuthProvider(name string) *SimpleAuthProvider {
	return &SimpleAuthProvider{name: name}
}

// AuthenticationProvider manages OAuth2 authentication flows
type AuthenticationProvider struct {
	Provider     AuthProvider
	LongName     string
	OAuthConfig  *oauth2.Config
	Fetcher      UserDataFetcher
	UserRepo     db.UserRepository
	SessionStore session.SessionStore
}

func NewOauth2Config(authProvider AuthProvider, provider *config.AuthProviderCredentials, baseUrl string, endpoint oauth2.Endpoint) *oauth2.Config {
	redirectUrl, _ := url.JoinPath(baseUrl, "/_/auth/"+authProvider.Name()+"/callback")

	// Scopes: https://developers.google.com/identity/protocols/oauth2/scopes
	return &oauth2.Config{
		RedirectURL:  redirectUrl,
		ClientID:     provider.ClientId,
		ClientSecret: provider.ClientSecret,
		Scopes:       []string{"email", "profile"},
		Endpoint:     endpoint,
	}
}

func NewAuthenticationProvider(oauthConfig *oauth2.Config, provider AuthProvider, longName string, baseUrl string, ur db.UserRepository, ss session.SessionStore) *AuthenticationProvider {
	redirectUrl, _ := url.JoinPath(baseUrl, "/_/auth/"+provider.Name()+"/callback")
	log.Printf("Registering %s Auth Provider. Redirecting to URL=%s", longName, redirectUrl)

	return &AuthenticationProvider{
		Provider:     provider,
		LongName:     longName,
		OAuthConfig:  oauthConfig,
		UserRepo:     ur,
		SessionStore: ss,
	}
}

// Login initiates the OAuth2 login flow
func (ap *AuthenticationProvider) Login(w http.ResponseWriter, r *http.Request) {
	state, err := generateState()
	if err != nil {
		log.Printf("Error generating state: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	authCodeURL := ap.OAuthConfig.AuthCodeURL(state)

	// Get redirect URL from query parameters, default to "/"
	originalURL := r.URL.Query().Get("redirect")
	if originalURL == "" {
		originalURL = "/"
	}

	// Set cookie for the redirect URL
	http.SetCookie(w, &http.Cookie{
		Name:     RedirectUrlCookieName,
		Value:    originalURL,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   300, // 5 minutes
	})

	// Store the state
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   300, // 5 minutes
	})

	// Redirect to authorization URL
	http.Redirect(w, r, authCodeURL, http.StatusTemporaryRedirect)
}

// Callback handles the OAuth2 callback
func (ap *AuthenticationProvider) Callback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")

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
		Secure:   r.TLS != nil,
		MaxAge:   -1, // Delete immediately
	})

	// Exchange code for token
	token, err := ap.OAuthConfig.Exchange(r.Context(), code)
	if err != nil {
		log.Printf("Error exchanging code for token: %v", err)
		http.Error(w, "Failed to exchange auth code", http.StatusInternalServerError)
		return
	}

	// Fetch user data using the token
	userInfo, err := ap.Fetcher.FetchUserData(token.AccessToken)
	if err != nil {
		log.Printf("Error loading user data: %v", err)
		http.Error(w, "Error loading user data", http.StatusInternalServerError)
		return
	}

	// Handle user creation or update
	user, err := ap.UserRepo.FindUserByIdOrUsername("", "", userInfo.Email)
	if err != nil {
		log.Printf("Error finding user: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if user == nil {
		// User not found, creating a new one
		user = &db.User{
			Email:          userInfo.Email,
			Username:       userInfo.Username,
			Name:           userInfo.Name,
			GivenName:      userInfo.GivenName,
			FamilyName:     userInfo.FamilyName,
			Picture:        userInfo.Picture,
			Locale:         userInfo.Locale,
			Provider:       ap.Provider.Name(),
			ProviderId:     userInfo.ID,
			EmailConfirmed: true,
		}

		err = ap.UserRepo.CreateUser(user)
		if err != nil {
			log.Printf("Error creating user: %v", err)
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}
	} else if user.Provider == ap.Provider.Name() {
		// Update existing user with the same provider
		user.Name = userInfo.Name
		user.GivenName = userInfo.GivenName
		user.FamilyName = userInfo.FamilyName
		user.Picture = userInfo.Picture
		user.Locale = userInfo.Locale
		user.ProviderId = userInfo.ID

		err := ap.UserRepo.UpdateUser(user)
		if err != nil {
			log.Printf("Error updating user: %v", err)
			// Non-critical error, continue with login
		}
	} else {
		// User exists but with a different provider
		log.Printf("User already exists with a different provider: %s", user.Provider)
		http.Error(w, "User already exists with provider "+user.Provider, http.StatusUnauthorized)
		return
	}

	// Validate user for login
	if err := validateUserLogin(user); err != nil {
		log.Printf("User validation failed: %v", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Create a session for the authenticated user
	tokenKey, err := ap.SessionStore.GenerateKey()
	if err != nil {
		log.Printf("Error generating session key: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create a session object
	sessionObj := session.SessionObject{
		UserID:          user.ID, // Add UserID to correctly link session to user
		Username:        user.Username,
		Email:           user.Email,
		IsAuthenticated: true,
		ValidUntil:      time.Now().Add(24 * time.Hour), // 24-hour session
		Provider:        ap.Provider.Name(),
	}

	// Store the session
	if err := ap.SessionStore.Set(tokenKey, sessionObj); err != nil {
		log.Printf("Error storing session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     session.SessionCookieName,
		Value:    tokenKey,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   86400, // 24 hours in seconds
	})

	// Get redirect URL from cookie
	redirectURL := "/"
	redirectCookie, err := r.Cookie(RedirectUrlCookieName)
	if err == nil && redirectCookie.Value != "" {
		redirectURL = redirectCookie.Value
	}

	// Clear redirect cookie
	http.SetCookie(w, &http.Cookie{
		Name:     RedirectUrlCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   -1, // Delete immediately
	})

	// Redirect to the original URL
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Logout handles the logout process
func (ap *AuthenticationProvider) Logout(w http.ResponseWriter, r *http.Request) {
	// Get the session token from the cookie
	cookie, err := r.Cookie(session.SessionCookieName)
	if err != nil {
		log.Printf("Error getting session cookie: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Delete the session from the store
	if err := ap.SessionStore.Delete(cookie.Value); err != nil {
		log.Printf("Error deleting session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Expire the session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     session.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   -1, // Delete immediately
	})

	// Add cache control headers to prevent browser caching
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, post-check=0, pre-check=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Redirect to the login page or home page
	http.Redirect(w, r, "/", http.StatusFound)
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
	log.Printf("Registered OAuth2 endpoints for %s provider", ap.LongName)
}

func generateState() (string, error) {
	b := make([]byte, 16)
	_, err := rand.Read(b)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}

// validateUserLogin validates if a user can log in
func validateUserLogin(user *db.User) error {
	if !user.EmailConfirmed {
		return errors.New("user email is not confirmed")
	}
	return nil
}

// SplitRoles splits a comma-separated role string into a slice
func SplitRoles(roles string) []string {
	if roles == "" {
		return []string{}
	}
	return strings.Split(roles, ",")
}
