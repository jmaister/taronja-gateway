package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"golang.org/x/oauth2"
)

// appleIssuer is both the token issuer Apple's ID tokens declare and the
// audience the client-secret JWT this gateway signs must declare (Apple
// checks both, from opposite directions).
const appleIssuer = "https://appleid.apple.com"

// appleJWKSURL is where Apple publishes the public keys its ID tokens are
// signed with — see jwksAppleIDTokenVerifier. A var, not a const, purely so
// tests can point it at a fake JWKS server instead of depending on real
// network access to Apple.
var appleJWKSURL = appleIssuer + "/auth/keys"

type AppleProvider struct{}

func (p AppleProvider) Name() string {
	return "apple"
}

// appleFlexibleBool decodes an Apple ID token claim that's documented, and
// observed in the wild, as coming back as either a real JSON boolean or the
// literal string "true"/"false" depending on the client that requested the
// token — email_verified and is_private_email are both this shape.
type appleFlexibleBool bool

func (b *appleFlexibleBool) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	*b = appleFlexibleBool(s == "true")
	return nil
}

// appleIDTokenClaims is the subset of Sign in with Apple's ID token this
// gateway reads. See:
// https://developer.apple.com/documentation/sign_in_with_apple/tokenresponse/id_token
type appleIDTokenClaims struct {
	jwt.RegisteredClaims
	Email          string            `json:"email"`
	EmailVerified  appleFlexibleBool `json:"email_verified"`
	IsPrivateEmail appleFlexibleBool `json:"is_private_email"`
}

// appleUserPayload is the one-time "user" form field Apple includes
// directly in the callback request — only on the very first authorization
// a user ever grants this app. Every later login's callback omits it
// entirely (Apple's own documented behavior, not a bug to work around), so
// this is the one chance to ever capture the user's name.
type appleUserPayload struct {
	Name struct {
		FirstName string `json:"firstName"`
		LastName  string `json:"lastName"`
	} `json:"name"`
}

// appleIDTokenVerifier verifies and decodes an Apple-issued ID token.
// Abstracted behind an interface so tests can substitute a fake verifier
// (a self-signed key + a fake JWKS server) instead of depending on Apple's
// real endpoint — see jwksAppleIDTokenVerifier for the real implementation
// and apple_test.go for the fake one.
type appleIDTokenVerifier interface {
	Verify(idToken string) (*appleIDTokenClaims, error)
}

// jwksAppleIDTokenVerifier verifies an ID token's signature against Apple's
// published public keys (fetched and cached by keyfunc, refreshed hourly in
// the background — see RegisterAppleAuth), plus its issuer, audience, and
// expiry. This is what makes trusting the token's claims safe even though
// nothing else about this integration validates who's calling
// /_/auth/apple/callback beyond the OAuth2 state check every provider here
// already does.
type jwksAppleIDTokenVerifier struct {
	keyfunc  jwt.Keyfunc
	clientId string
}

func (v *jwksAppleIDTokenVerifier) Verify(idToken string) (*appleIDTokenClaims, error) {
	claims := &appleIDTokenClaims{}
	token, err := jwt.ParseWithClaims(idToken, claims, v.keyfunc,
		jwt.WithIssuer(appleIssuer),
		jwt.WithAudience(v.clientId),
		jwt.WithValidMethods([]string{"RS256"}),
	)
	if err != nil {
		return nil, fmt.Errorf("verifying Apple ID token: %w", err)
	}
	if !token.Valid {
		return nil, fmt.Errorf("Apple ID token failed validation")
	}
	return claims, nil
}

// buildAppleClientSecret signs a fresh JWT to use as the OAuth2 client
// secret Apple's token endpoint expects — Apple never issues a static
// client secret string the way every other provider here does; the
// developer's own EC private key signs one instead. Valid for just long
// enough to cover one token exchange: this is called fresh on every login
// (see AuthenticationProvider.ClientSecretFunc), so there's no benefit to
// the longer lifetime Apple allows (up to 6 months) and every reason to
// keep it short.
func buildAppleClientSecret(creds config.AppleAuthProviderCredentials) (string, error) {
	privateKey, err := jwt.ParseECPrivateKeyFromPEM([]byte(creds.PrivateKey))
	if err != nil {
		return "", fmt.Errorf("parsing Apple private key: %w", err)
	}

	now := time.Now()
	claims := jwt.RegisteredClaims{
		Issuer:    creds.TeamId,
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(now.Add(5 * time.Minute)),
		Audience:  jwt.ClaimStrings{appleIssuer},
		Subject:   creds.ClientId,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = creds.KeyId

	return token.SignedString(privateKey)
}

// AppleUserDataFetcher implements UserDataFetcher without ever making a
// REST call — Apple has no "get current user" endpoint at all. User info
// comes entirely from the verified ID token already present in the token
// exchange response, plus, only on a user's very first authorization, the
// one-time name payload described on appleUserPayload above.
type AppleUserDataFetcher struct {
	Verifier appleIDTokenVerifier
}

func (f *AppleUserDataFetcher) FetchUserData(r *http.Request, token *oauth2.Token) (*UserInfo, error) {
	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return nil, fmt.Errorf("token response from Apple did not include an id_token")
	}

	claims, err := f.Verifier.Verify(rawIDToken)
	if err != nil {
		return nil, err
	}

	userInfo := &UserInfo{
		ID:            claims.Subject, // Apple's stable, opaque per-app user identifier
		Email:         claims.Email,
		Username:      claims.Email,
		VerifiedEmail: bool(claims.EmailVerified),
		Provider:      "apple",
	}

	if raw := r.FormValue("user"); raw != "" {
		var payload appleUserPayload
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			// Non-fatal: the login itself is still valid without a name,
			// and this can only ever happen on the one first-authorization
			// request anyway.
			log.Printf("apple auth: failed to parse one-time user info payload: %v", err)
		} else {
			userInfo.GivenName = payload.Name.FirstName
			userInfo.FamilyName = payload.Name.LastName
			if userInfo.GivenName != "" || userInfo.FamilyName != "" {
				userInfo.Name = strings.TrimSpace(userInfo.GivenName + " " + userInfo.FamilyName)
			}
		}
	}

	return userInfo, nil
}

// RegisterAppleAuth configures and registers "Sign in with Apple"
// authentication.
//
// ctx bounds the lifetime of keyfunc's hourly JWKS-refresh goroutine — the
// caller (RegisterProviders, in turn gateway.registerLoginRoutes) cancels
// the ctx from the previous registration before this runs again on a config
// reload, so each reload's fetcher replaces rather than piles on top of the
// last one.
func RegisterAppleAuth(ctx context.Context, mux *http.ServeMux, sessionStore session.SessionStore, gatewayConfig *config.GatewayConfig, userRepo db.UserRepository) {
	creds := gatewayConfig.AuthenticationProviders.Apple
	if !creds.IsConfigured() {
		return // Skip if not fully configured
	}

	// Fails only on a malformed URL list, never on the JWKS endpoint being
	// briefly unreachable — keyfunc's default HTTP storage tolerates the
	// very first fetch failing and keeps retrying hourly in the
	// background, so a transient network hiccup at gateway startup can't
	// prevent Apple auth from registering.
	kf, err := keyfunc.NewDefaultCtx(ctx, []string{appleJWKSURL})
	if err != nil {
		log.Printf("Apple auth: failed to set up JWKS key fetcher, skipping registration: %v", err)
		return
	}

	oauthConfig := &oauth2.Config{
		ClientID:    creds.ClientId,
		RedirectURL: fmt.Sprintf("%s%s/auth/apple/callback", gatewayConfig.Server.URL, gatewayConfig.Management.Prefix),
		// name/email together force response_mode=form_post below — Apple
		// only ever returns additional user data via POST.
		Scopes: []string{"name", "email"},
		Endpoint: oauth2.Endpoint{
			AuthURL:  appleIssuer + "/auth/authorize",
			TokenURL: appleIssuer + "/auth/token",
		},
	}

	provider := AppleProvider{}
	fetcher := &AppleUserDataFetcher{
		Verifier: &jwksAppleIDTokenVerifier{
			keyfunc:  kf.Keyfunc,
			clientId: creds.ClientId,
		},
	}

	authProvider := NewAuthenticationProvider(oauthConfig, provider, "Apple", userRepo, sessionStore, gatewayConfig)
	authProvider.Fetcher = fetcher
	authProvider.ResponseMode = "form_post"
	authProvider.ClientSecretFunc = func() (string, error) {
		return buildAppleClientSecret(creds)
	}

	authProvider.RegisterEndpoints(mux)
}
