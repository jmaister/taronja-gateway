package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/github"
	"golang.org/x/oauth2/google"
)

// --- Authenticator Interface ---

type Authenticator interface {
	Authenticate(w http.ResponseWriter, r *http.Request) (bool, *http.Request) // Return modified request with context
}

// --- Authentication Manager ---

type AuthManager struct {
	authenticators map[string]Authenticator
	globalConfig   *Config
}

func NewAuthManager(cfg *Config) *AuthManager {
	manager := &AuthManager{
		authenticators: make(map[string]Authenticator),
		globalConfig:   cfg,
	}
	manager.registerAuthenticators()
	return manager
}

func (am *AuthManager) registerAuthenticators() {
	// Basic Auth
	am.authenticators["basic"] = &BasicAuthenticator{}

	// "Any" Authenticator (for internal endpoints like /me)
	am.authenticators["any"] = &AnyAuthenticator{}

	// OAuth2 - Google
	if creds := am.globalConfig.AuthenticationProviders.Google; creds.ClientId != "" {
		googleOAuthConfig := &oauth2.Config{
			ClientID:     creds.ClientId,
			ClientSecret: creds.ClientSecret,
			RedirectURL:  fmt.Sprintf("%s/auth/callback/google", strings.TrimSuffix(am.globalConfig.Server.URL, "/")),
			Scopes:       []string{"openid", "profile", "email"},
			Endpoint:     google.Endpoint,
		}
		am.authenticators["google"] = NewOAuth2Authenticator("google", googleOAuthConfig, am.globalConfig.Server.URL)
		log.Printf("auth.go: Registered OAuth2 Authenticator: google")
	} else {
		log.Printf("auth.go: OAuth2 provider 'google' not configured (missing clientId).")
	}

	// OAuth2 - GitHub
	if creds := am.globalConfig.AuthenticationProviders.Github; creds.ClientId != "" {
		githubOAuthConfig := &oauth2.Config{
			ClientID:     creds.ClientId,
			ClientSecret: creds.ClientSecret,
			RedirectURL:  fmt.Sprintf("%s/auth/callback/github", strings.TrimSuffix(am.globalConfig.Server.URL, "/")),
			Scopes:       []string{"read:user", "user:email"},
			Endpoint:     github.Endpoint,
		}
		am.authenticators["github"] = NewOAuth2Authenticator("github", githubOAuthConfig, am.globalConfig.Server.URL)
		log.Printf("auth.go: Registered OAuth2 Authenticator: github")
	} else {
		log.Printf("auth.go: OAuth2 provider 'github' not configured (missing clientId).")
	}
	// Add other providers here
}

// GetAuthenticator retrieves the appropriate authenticator.
func (am *AuthManager) GetAuthenticator(authConfig AuthenticationConfig) (Authenticator, error) {
	var providerKey string

	switch authConfig.Method {
	case "basic":
		providerKey = "basic"
	case "oauth2":
		if authConfig.Provider == "" {
			return nil, fmt.Errorf("oauth2 method requires a provider (e.g., google, github)")
		}
		providerKey = authConfig.Provider
	case "any": // Handle the special "any" method for internal endpoints
		providerKey = "any"
	case "":
		return nil, nil // No specific authenticator needed
	default:
		return nil, fmt.Errorf("unsupported authentication method: %s", authConfig.Method)
	}

	authenticator, exists := am.authenticators[providerKey]
	if !exists {
		// Provide more specific error message for OAuth providers
		if authConfig.Method == "oauth2" {
			return nil, fmt.Errorf("oauth2 provider '%s' not found or not configured in authenticationProviders", providerKey)
		}
		return nil, fmt.Errorf("authenticator for method '%s' not found or not configured", providerKey)
	}
	return authenticator, nil
}

// --- Basic Authenticator ---

type BasicAuthenticator struct{}

// Authenticate performs HTTP Basic Authentication and adds user to context.
func (ba *BasicAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (bool, *http.Request) {
	username, password, ok := r.BasicAuth()
	if !ok {
		log.Printf("auth.go: Basic Auth: Missing credentials\n")
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false, r
	}

	// --- Dummy Validation ---
	isValid := (username == "admin" && password == "password")
	// --- End Dummy Validation ---

	if !isValid {
		log.Printf("auth.go: Basic Auth: Invalid credentials for user: %s\n", username)
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false, r
	}

	log.Printf("auth.go: Basic Auth: User '%s' authenticated\n", username)

	// Add user info to request context
	user := &AuthenticatedUser{
		ID:     username,
		Source: "basic",
	}
	ctx := context.WithValue(r.Context(), userContextKey, user)
	return true, r.WithContext(ctx) // Return modified request
}

// --- Any Authenticator ---

// AnyAuthenticator checks if *any* user is present in the context.
// Used for internal endpoints like /me that require login but not a specific method.
type AnyAuthenticator struct{}

func (aa *AnyAuthenticator) Authenticate(w http.ResponseWriter, r *http.Request) (bool, *http.Request) {
	// Check if the context already contains user info (populated by a preceding authenticator)
	user, ok := r.Context().Value(userContextKey).(*AuthenticatedUser)

	if !ok || user == nil {
		// No authenticated user found in context.
		// This might happen if /me is accessed directly without prior auth,
		// or if the preceding auth middleware failed to add the user.
		log.Printf("auth.go: Any Auth: No authenticated user found in context for path %s.", r.URL.Path)
		http.Error(w, "Unauthorized - Login Required", http.StatusUnauthorized)
		// TODO: Optionally, could try initiating a default login flow (e.g., redirect to a login page or primary OAuth provider)?
		// For now, just return 401.
		return false, r
	}

	// User found in context, authentication successful for "any" method.
	log.Printf("auth.go: Any Auth: User '%s' (source: %s) already authenticated.", user.ID, user.Source)
	return true, r // Return original request (context already populated)
}

// --- OAuth2 Authenticator ---

// SessionStore interface (remains the same)
type SessionStore interface {
	Set(w http.ResponseWriter, r *http.Request, key string, value string)
	Get(r *http.Request, key string) (string, bool)
	Delete(w http.ResponseWriter, r *http.Request, key string)
}

// InMemorySessionStore (Example - remains the same)
type InMemorySessionStore struct{}

func (s *InMemorySessionStore) Set(w http.ResponseWriter, r *http.Request, key string, value string) {
	http.SetCookie(w, &http.Cookie{
		Name:     key,
		Value:    value,
		Path:     "/",
		HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode,
	})
}
func (s *InMemorySessionStore) Get(r *http.Request, key string) (string, bool) {
	cookie, err := r.Cookie(key)
	if err != nil {
		return "", false
	}
	return cookie.Value, true
}
func (s *InMemorySessionStore) Delete(w http.ResponseWriter, r *http.Request, key string) {
	http.SetCookie(w, &http.Cookie{
		Name: key, Value: "", Path: "/", HttpOnly: true, Secure: r.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

type OAuth2Authenticator struct {
	providerName string
	config       *oauth2.Config
	gatewayURL   string
	sessionStore SessionStore
}

func NewOAuth2Authenticator(providerName string, config *oauth2.Config, gatewayURL string) *OAuth2Authenticator {
	return &OAuth2Authenticator{
		providerName: providerName,
		config:       config,
		gatewayURL:   gatewayURL,
		sessionStore: &InMemorySessionStore{}, // FIXME: Use a real session store
	}
}

// Authenticate handles OAuth2 checks, redirects, and context population.
func (oa *OAuth2Authenticator) Authenticate(w http.ResponseWriter, r *http.Request) (bool, *http.Request) {
	// 1. Check for existing valid session token (created during callback)
	sessionToken, ok := oa.sessionStore.Get(r, fmt.Sprintf("%s_session_token", oa.providerName))
	if ok {
		// TODO: Implement proper session validation (e.g., lookup in server-side store, check expiry)
		// For now, assume token presence means valid session.
		// We also need to fetch/reconstruct the user info associated with this session token.
		// Placeholder: Assume token itself contains enough info or can be used to fetch it.
		user, err := oa.validateAndGetUserFromSessionToken(sessionToken) // Needs implementation
		if err == nil && user != nil {
			log.Printf("auth.go: OAuth2 [%s]: Valid session token found.", oa.providerName)
			ctx := context.WithValue(r.Context(), userContextKey, user)
			return true, r.WithContext(ctx)
		}
		log.Printf("auth.go: OAuth2 [%s]: Invalid session token found: %v.", oa.providerName, err)
		// Clear invalid session cookie
		oa.sessionStore.Delete(w, r, fmt.Sprintf("%s_session_token", oa.providerName))
		// Continue to check Bearer token or redirect...
	}

	// 2. Check for Bearer Token in Header (for API clients)
	authHeader := r.Header.Get("Authorization")
	if strings.HasPrefix(authHeader, "Bearer ") {
		token := strings.TrimPrefix(authHeader, "Bearer ")
		log.Printf("auth.go: OAuth2 [%s]: Received Bearer token. Validation required.", oa.providerName)

		// TODO: Implement actual Bearer token validation (e.g., introspection endpoint)
		// This validation should also return user info.
		user, err := oa.validateAndGetUserFromBearerToken(token) // Needs implementation
		if err == nil && user != nil {
			log.Printf("auth.go: OAuth2 [%s]: Bearer token validated successfully.", oa.providerName)
			ctx := context.WithValue(r.Context(), userContextKey, user)
			return true, r.WithContext(ctx)
		}

		log.Printf("auth.go: OAuth2 [%s]: Invalid Bearer token: %v.", oa.providerName, err)
		http.Error(w, "Unauthorized - Invalid Bearer Token", http.StatusUnauthorized)
		return false, r
	}

	// 3. If no session and no valid bearer token, assume browser flow: Initiate redirect.
	if strings.HasPrefix(r.URL.Path, "/auth/callback/") || strings.Contains(r.Header.Get("Accept"), "application/json") {
		log.Printf("auth.go: OAuth2 [%s]: No token/session found, but request seems like API or callback. Denying.", oa.providerName)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return false, r
	}

	log.Printf("auth.go: OAuth2 [%s]: No valid session or token found. Initiating redirect flow.", oa.providerName)
	stateBytes := make([]byte, 16)
	_, err := rand.Read(stateBytes)
	if err != nil {
		log.Printf("auth.go: OAuth2 [%s]: Failed to generate state: %v", oa.providerName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return false, r
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)
	oa.sessionStore.Set(w, r, fmt.Sprintf("%s_oauth_state", oa.providerName), state)
	oa.sessionStore.Set(w, r, fmt.Sprintf("%s_oauth_redirect", oa.providerName), r.URL.String())
	authURL := oa.config.AuthCodeURL(state)

	log.Printf("auth.go: OAuth2 [%s]: Redirecting user to: %s", oa.providerName, authURL)
	http.Redirect(w, r, authURL, http.StatusFound)
	return false, r // Return false because we sent a redirect
}

// --- TODO: Implement these validation functions ---

// validateAndGetUserFromSessionToken checks the session token and returns user info.
// Replace with actual session validation logic.
func (oa *OAuth2Authenticator) validateAndGetUserFromSessionToken(token string) (*AuthenticatedUser, error) {
	log.Printf("auth.go: OAuth2 [%s]: DUMMY CHECK - Validating session token: %s...", oa.providerName, token[:min(len(token), 10)])
	// TODO: Implement real session validation (check expiry, lookup in store, etc.)
	// If valid, reconstruct or fetch the AuthenticatedUser struct.
	if token == "valid-session-token-for-"+oa.providerName { // Replace with real check
		return &AuthenticatedUser{
			ID:     fmt.Sprintf("user-from-session-%s", oa.providerName), // Example ID
			Source: oa.providerName,
		}, nil
	}
	return nil, fmt.Errorf("invalid or expired session token")
}

// validateAndGetUserFromBearerToken validates an OAuth Bearer token and returns user info.
// Replace with actual token validation logic (e.g., introspection endpoint call).
func (oa *OAuth2Authenticator) validateAndGetUserFromBearerToken(token string) (*AuthenticatedUser, error) {
	log.Printf("auth.go: OAuth2 [%s]: DUMMY CHECK - Validating Bearer token: %s...", oa.providerName, token[:min(len(token), 10)])
	// TODO: Implement real token validation (call provider's endpoint, check signature/claims)
	// If valid, extract user ID and potentially other info.
	if token == fmt.Sprintf("valid-%s-token-from-api-client", oa.providerName) { // Replace with real check
		return &AuthenticatedUser{
			ID:     fmt.Sprintf("user-from-bearer-%s", oa.providerName), // Example ID
			Source: oa.providerName,
		}, nil
	}
	return nil, fmt.Errorf("invalid bearer token")
}

// --- OAuth Callback Handling ---

// HandleOAuthCallback processes the callback, exchanges code, validates token,
// creates a session, and populates context for the *redirect response*.
func (am *AuthManager) HandleOAuthCallback(provider string, w http.ResponseWriter, r *http.Request) {
	log.Printf("auth.go: OAuth Callback: Received callback for provider '%s'", provider)

	authInterface, exists := am.authenticators[provider]
	if !exists {
		log.Printf("auth.go: OAuth Callback Error: No authenticator configured for provider '%s'", provider)
		http.Error(w, "Configuration error: Unknown provider", http.StatusBadRequest)
		return
	}
	oauthAuthenticator, ok := authInterface.(*OAuth2Authenticator)
	if !ok {
		log.Printf("auth.go: OAuth Callback Error: Authenticator for '%s' is not an OAuth2 authenticator", provider)
		http.Error(w, "Configuration error: Invalid authenticator type", http.StatusInternalServerError)
		return
	}

	// 1. Verify State
	returnedState := r.URL.Query().Get("state")
	expectedState, ok := oauthAuthenticator.sessionStore.Get(r, fmt.Sprintf("%s_oauth_state", provider))
	// Clean up state immediately after reading
	oauthAuthenticator.sessionStore.Delete(w, r, fmt.Sprintf("%s_oauth_state", provider))
	if !ok || returnedState == "" || returnedState != expectedState {
		log.Printf("auth.go: OAuth Callback Error [%s]: Invalid state parameter. Expected '%s', got '%s'", provider, expectedState, returnedState)
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	// 2. Handle provider errors
	errorParam := r.URL.Query().Get("error")
	if errorParam != "" {
		errorDesc := r.URL.Query().Get("error_description")
		log.Printf("auth.go: OAuth Callback Error [%s]: Provider returned error: %s - %s", provider, errorParam, errorDesc)
		http.Error(w, fmt.Sprintf("OAuth provider error: %s", errorParam), http.StatusUnauthorized)
		return
	}

	// 3. Exchange Code for Token
	code := r.URL.Query().Get("code")
	if code == "" {
		log.Printf("auth.go: OAuth Callback Error [%s]: No authorization code received", provider)
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}
	token, err := oauthAuthenticator.config.Exchange(context.Background(), code)
	if err != nil {
		log.Printf("auth.go: OAuth Callback Error [%s]: Failed to exchange code for token: %v", provider, err)
		http.Error(w, "Failed to exchange token", http.StatusInternalServerError)
		return
	}
	if !token.Valid() {
		log.Printf("auth.go: OAuth Callback Error [%s]: Exchanged token is invalid", provider)
		http.Error(w, "Received invalid token", http.StatusInternalServerError)
		return
	}
	log.Printf("auth.go: OAuth Callback [%s]: Successfully exchanged code for token.", provider)

	// 4. Use Token to Get User Info (Crucial Step!)
	// TODO: Implement fetchUserInfo to call the provider's user info endpoint
	// This verifies the token and gets user details needed for the session/context.
	// client := oauthAuthenticator.config.Client(context.Background(), token)
	// userInfo, err := fetchUserInfo(client, provider) // Needs implementation
	// if err != nil {
	//     log.Printf("auth.go: OAuth Callback Error [%s]: Failed to fetch user info: %v", provider, err)
	//     http.Error(w, "Failed to verify token with provider", http.StatusInternalServerError)
	//     return
	// }
	// Dummy user info for now:
	userInfo := &AuthenticatedUser{
		ID:     fmt.Sprintf("user-%s-%s", provider, token.AccessToken[:min(len(token.AccessToken), 6)]), // Example ID
		Source: provider,
	}
	log.Printf("auth.go: OAuth Callback [%s]: Fetched User Info: ID=%s", provider, userInfo.ID)

	// 5. Create Session (Store token or session ID securely)
	// TODO: Replace dummy session token storage with secure method
	sessionToken := "valid-session-token-for-" + provider // DUMMY session token
	oauthAuthenticator.sessionStore.Set(w, r, fmt.Sprintf("%s_session_token", provider), sessionToken)
	log.Printf("auth.go: OAuth Callback [%s]: User authenticated. Session created (dummy).", provider)

	// 6. Redirect User
	redirectURL, ok := oauthAuthenticator.sessionStore.Get(r, fmt.Sprintf("%s_oauth_redirect", provider))
	if ok && redirectURL != "" {
		oauthAuthenticator.sessionStore.Delete(w, r, fmt.Sprintf("%s_oauth_redirect", provider))
		log.Printf("auth.go: OAuth Callback [%s]: Redirecting user back to: %s", provider, redirectURL)
		// Note: We don't typically add the user context to the *redirect* response itself,
		// but the session cookie set in step 5 will be sent by the browser on the *next* request
		// to the redirectURL, allowing Authenticate() to pick it up then.
		http.Redirect(w, r, redirectURL, http.StatusFound)
	} else {
		log.Printf("auth.go: OAuth Callback [%s]: Original redirect URL not found, redirecting to root.", provider)
		http.Redirect(w, r, "/", http.StatusFound)
	}
}

// Helper function
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// TODO: Implement fetchUserInfo(client *http.Client, provider string) (*AuthenticatedUser, error)
// This function would use the OAuth2 client to call the provider's user info endpoint
// (e.g., Google's userinfo endpoint, GitHub's /user endpoint) and parse the response
// into your AuthenticatedUser struct.
