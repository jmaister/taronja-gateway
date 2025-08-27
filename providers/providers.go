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
	"gorm.io/gorm"
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

// RegisterProviders registers all enabled authentication providers.
// It now accepts db.SessionRepository.
func RegisterProviders(mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository, userLoginRepo db.UserLoginRepository) {
	log.Printf("Registering authentication providers...")

	if gatewayConfig.AuthenticationProviders.Basic.Enabled || gatewayConfig.Management.Admin.Enabled {
		log.Printf("Registering Basic Authentication provider")
		RegisterBasicAuth(mux, sessionStore, gatewayConfig.Management.Prefix, userRepo, gatewayConfig)
	}

	if gatewayConfig.AuthenticationProviders.Github.ClientId != "" &&
		gatewayConfig.AuthenticationProviders.Github.ClientSecret != "" {
		log.Printf("Registering GitHub Authentication provider")
		RegisterGithubAuth(mux, sessionStore, gatewayConfig, userRepo, userLoginRepo)
	} else {
		log.Printf("GitHub Authentication provider not configured, skipping registration")
	}

	if gatewayConfig.AuthenticationProviders.Google.ClientId != "" &&
		gatewayConfig.AuthenticationProviders.Google.ClientSecret != "" {
		log.Printf("Registering Google Authentication provider")
		RegisterGoogleAuth(mux, sessionStore, gatewayConfig, userRepo, userLoginRepo)
	} else {
		log.Printf("Google Authentication provider not configured, skipping registration")
	}
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
// It now uses db.SessionRepository.
type AuthenticationProvider struct {
	Provider        AuthProvider
	LongName        string
	OAuthConfig     *oauth2.Config
	Fetcher         UserDataFetcher
	UserRepo        db.UserRepository
	UserLoginRepo   db.UserLoginRepository
	SessionStore    session.SessionStore
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
// It now accepts db.SessionRepository and db.UserLoginRepository.
func NewAuthenticationProvider(oauthConfig *oauth2.Config, provider AuthProvider, longName string, ur db.UserRepository, ulr db.UserLoginRepository, sessionStore session.SessionStore) *AuthenticationProvider {
	// baseUrl is not directly needed here if redirectURL is in oauthConfig
	// log.Printf("Registering %s Auth Provider. Redirecting to URL=%s", longName, oauthConfig.RedirectURL)

	return &AuthenticationProvider{
		Provider:      provider,
		LongName:      longName,
		OAuthConfig:   oauthConfig,
		UserRepo:      ur,
		UserLoginRepo: ulr,
		SessionStore:  sessionStore,
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

// Callback handles the OAuth2 callback with multi-login support.
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
		log.Printf("Error loading user data from provider %s: %v", ap.Provider.Name(), err)
		http.Error(w, "Error loading user data from provider", http.StatusInternalServerError)
		return
	}

	// Check if this provider login already exists
	existingUser, err := ap.UserLoginRepo.FindUserByProviderLogin(ap.Provider.Name(), userInfo.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		log.Printf("Error finding user by provider login: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	var user *db.User

	if existingUser != nil {
		// User login already exists, log them in
		user = existingUser
		log.Printf("Existing user login found for %s, provider %s", userInfo.Email, ap.Provider.Name())
		
		// Update the provider login information
		userLogin, err := ap.UserLoginRepo.FindUserLoginByProvider(ap.Provider.Name(), userInfo.ID)
		if err == nil {
			userLogin.Email = userInfo.Email
			userLogin.Username = userInfo.Username
			userLogin.Picture = userInfo.Picture
			userLogin.GivenName = userInfo.GivenName
			userLogin.FamilyName = userInfo.FamilyName
			userLogin.Locale = userInfo.Locale
			ap.UserLoginRepo.UpdateUserLogin(userLogin)
		}
		
		// Update user's profile information if needed
		if user.Name == "" && userInfo.Name != "" {
			user.Name = userInfo.Name
		}
		if user.Picture == "" && userInfo.Picture != "" {
			user.Picture = userInfo.Picture
		}
		if user.Locale == "" && userInfo.Locale != "" {
			user.Locale = userInfo.Locale
		}
		ap.UserRepo.UpdateUser(user)
	} else {
		// This provider login doesn't exist yet
		// Check if a user exists with this email address
		user, err = ap.UserRepo.FindUserByIdOrUsername("", "", userInfo.Email)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Error finding user by email: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		if user == nil {
			// No user exists with this email, create a new user
			user = &db.User{
				Email:          userInfo.Email,
				Username:       userInfo.Username,
				Name:           userInfo.Name,
				Picture:        userInfo.Picture,
				Locale:         userInfo.Locale,
				EmailConfirmed: true, // OAuth providers typically verify email
			}
			
			// Ensure username is unique
			if user.Username == "" {
				user.Username = userInfo.Email
			}
			
			err = ap.UserRepo.CreateUser(user)
			if err != nil {
				log.Printf("Error creating user: %v", err)
				http.Error(w, "Failed to create user", http.StatusInternalServerError)
				return
			}
			log.Printf("Created new user: %s", user.Email)
		} else {
			log.Printf("Found existing user by email: %s, linking new provider %s", user.Email, ap.Provider.Name())
		}

		// Create the user login entry
		userLogin := &db.UserLogin{
			UserID:     user.ID,
			Provider:   ap.Provider.Name(),
			ProviderId: userInfo.ID,
			Email:      userInfo.Email,
			Username:   userInfo.Username,
			Picture:    userInfo.Picture,
			GivenName:  userInfo.GivenName,
			FamilyName: userInfo.FamilyName,
			Locale:     userInfo.Locale,
			IsActive:   true,
		}
		
		err = ap.UserLoginRepo.CreateUserLogin(userLogin)
		if err != nil {
			log.Printf("Error creating user login: %v", err)
			http.Error(w, "Failed to link provider", http.StatusInternalServerError)
			return
		}
		log.Printf("Linked provider %s to user %s", ap.Provider.Name(), user.Email)
	}

	if err := validateUserLogin(user); err != nil {
		log.Printf("User validation failed for %s: %v", user.Email, err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	sessionObj, err := ap.SessionStore.NewSession(r, user, ap.Provider.Name(), 24*time.Hour)
	if err != nil {
		log.Printf("Error creating session: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     session.SessionCookieName,
		Value:    sessionObj.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		MaxAge:   int((24 * time.Hour).Seconds()), // 24 hours
	})

	redirectURL := "/"
	redirectCookie, err := r.Cookie(RedirectUrlCookieName)
	if err == nil && redirectCookie.Value != "" {
		redirectURL = redirectCookie.Value
		// Clear the redirect cookie
		http.SetCookie(w, &http.Cookie{Name: RedirectUrlCookieName, Value: "", Path: "/", MaxAge: -1})
	}

	http.Redirect(w, r, redirectURL, http.StatusFound)
}

// Logout handles the logout process.
// Uses db.SessionRepository.DeleteSession.
func (ap *AuthenticationProvider) Logout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(session.SessionCookieName)
	if err == nil && cookie != nil {
		_ = ap.SessionStore.EndSession(cookie.Value) // End the session
		http.SetCookie(w, &http.Cookie{
			Name:     session.SessionCookieName,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			Secure:   r.TLS != nil,
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

func validateUserLogin(user *db.User) error {
	if !user.EmailConfirmed {
		return errors.New("user email is not confirmed")
	}
	// Add other validation rules if necessary
	return nil
}

func SplitRoles(roles string) []string {
	if roles == "" {
		return []string{}
	}
	return strings.Split(roles, ",")
}
