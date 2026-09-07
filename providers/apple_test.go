package providers

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jmaister/taronja-gateway/config"
	"github.com/jmaister/taronja-gateway/db"
	"github.com/jmaister/taronja-gateway/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestAppleProvider_Name(t *testing.T) {
	provider := AppleProvider{}
	assert.Equal(t, "apple", provider.Name())
}

// generateTestECKeyPEM builds a fresh P-256 key (the curve ES256 requires)
// and PEM-encodes it in the PKCS8 shape Apple's own .p8 key downloads use —
// the same shape jwt.ParseECPrivateKeyFromPEM (and so buildAppleClientSecret)
// expects.
func generateTestECKeyPEM(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	return key, string(pemBytes)
}

func TestBuildAppleClientSecret(t *testing.T) {
	key, keyPEM := generateTestECKeyPEM(t)
	creds := config.AppleAuthProviderCredentials{
		ClientId:   "com.example.service",
		TeamId:     "TEAMID1234",
		KeyId:      "KEYID5678",
		PrivateKey: keyPEM,
	}

	secret, err := buildAppleClientSecret(creds)
	require.NoError(t, err)
	require.NotEmpty(t, secret)

	// Parse it back with the matching public key to confirm it's a real,
	// correctly-signed, correctly-shaped ES256 JWT — not just a non-empty
	// string.
	claims := &jwt.RegisteredClaims{}
	token, err := jwt.ParseWithClaims(secret, claims, func(t *jwt.Token) (interface{}, error) {
		return &key.PublicKey, nil
	}, jwt.WithValidMethods([]string{"ES256"}))
	require.NoError(t, err)
	require.True(t, token.Valid)

	assert.Equal(t, "KEYID5678", token.Header["kid"])
	assert.Equal(t, "TEAMID1234", claims.Issuer)
	assert.Equal(t, "com.example.service", claims.Subject)
	assert.Equal(t, jwt.ClaimStrings{appleIssuer}, claims.Audience)
	require.NotNil(t, claims.ExpiresAt)
	// Short-lived on purpose (regenerated fresh every login) — comfortably
	// under Apple's 6-month maximum, but this locks in "short", not any
	// exact duration, so a deliberate future tweak doesn't need to touch
	// this assertion's exact number.
	assert.WithinDuration(t, time.Now().Add(5*time.Minute), claims.ExpiresAt.Time, time.Minute)
}

func TestBuildAppleClientSecret_InvalidPrivateKey(t *testing.T) {
	creds := config.AppleAuthProviderCredentials{
		ClientId:   "com.example.service",
		TeamId:     "TEAMID1234",
		KeyId:      "KEYID5678",
		PrivateKey: "not a real PEM key",
	}
	_, err := buildAppleClientSecret(creds)
	assert.Error(t, err)
}

// newFakeAppleJWKSServer serves a real JWK Set (built from rsaKey's public
// half) over HTTP, standing in for https://appleid.apple.com/auth/keys —
// keyfunc doesn't know or care that it isn't really Apple.
func newFakeAppleJWKSServer(t *testing.T, rsaKey *rsa.PrivateKey, kid string) *httptest.Server {
	t.Helper()
	jwk, err := jwkset.NewJWKFromKey(&rsaKey.PublicKey, jwkset.JWKOptions{
		Metadata: jwkset.JWKMetadataOptions{
			KID: kid,
			ALG: jwkset.AlgRS256,
			USE: jwkset.UseSig,
		},
	})
	require.NoError(t, err)

	jwkSet := struct {
		Keys []jwkset.JWKMarshal `json:"keys"`
	}{
		Keys: []jwkset.JWKMarshal{jwk.Marshal()},
	}
	body, err := json.Marshal(jwkSet)
	require.NoError(t, err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(server.Close)
	return server
}

// signTestAppleIDToken builds and RS256-signs a token with appleIDTokenClaims'
// exact shape, so jwksAppleIDTokenVerifier.Verify parses it the same way it
// would a real one from Apple.
func signTestAppleIDToken(t *testing.T, key *rsa.PrivateKey, kid string, claims appleIDTokenClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	require.NoError(t, err)
	return signed
}

func validTestAppleClaims(clientId string) appleIDTokenClaims {
	now := time.Now()
	return appleIDTokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    appleIssuer,
			Audience:  jwt.ClaimStrings{clientId},
			Subject:   "001234.abcdef0123456789.5678",
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
		},
		Email:         "user@example.com",
		EmailVerified: true,
	}
}

// TestJWKSAppleIDTokenVerifier_Verify is the real cryptographic-verification
// test the "include JWKS verification" scope decision was for: a genuine
// RS256-signed token, checked against a genuine (self-generated, served
// over real HTTP) key set — not just claim-shape decoding.
func TestJWKSAppleIDTokenVerifier_Verify(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	const kid = "test-kid"
	const clientId = "com.example.service"

	server := newFakeAppleJWKSServer(t, rsaKey, kid)
	kf, err := keyfunc.NewDefaultCtx(context.Background(), []string{server.URL})
	require.NoError(t, err)
	verifier := &jwksAppleIDTokenVerifier{keyfunc: kf.Keyfunc, clientId: clientId}

	t.Run("valid token verifies and decodes correctly", func(t *testing.T) {
		idToken := signTestAppleIDToken(t, rsaKey, kid, validTestAppleClaims(clientId))
		claims, err := verifier.Verify(idToken)
		require.NoError(t, err)
		assert.Equal(t, "001234.abcdef0123456789.5678", claims.Subject)
		assert.Equal(t, "user@example.com", claims.Email)
		assert.True(t, bool(claims.EmailVerified))
	})

	t.Run("rejects a token for a different audience", func(t *testing.T) {
		claims := validTestAppleClaims("some-other-client-id")
		idToken := signTestAppleIDToken(t, rsaKey, kid, claims)
		_, err := verifier.Verify(idToken)
		assert.Error(t, err)
	})

	t.Run("rejects an expired token", func(t *testing.T) {
		claims := validTestAppleClaims(clientId)
		claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
		idToken := signTestAppleIDToken(t, rsaKey, kid, claims)
		_, err := verifier.Verify(idToken)
		assert.Error(t, err)
	})

	t.Run("rejects a token with the wrong issuer", func(t *testing.T) {
		claims := validTestAppleClaims(clientId)
		claims.Issuer = "https://not-apple.example.com"
		idToken := signTestAppleIDToken(t, rsaKey, kid, claims)
		_, err := verifier.Verify(idToken)
		assert.Error(t, err)
	})

	t.Run("rejects a token signed by a different key than the one published", func(t *testing.T) {
		otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		// Signed with otherKey but claiming the *published* kid — the
		// signature must fail to verify against the real published key.
		idToken := signTestAppleIDToken(t, otherKey, kid, validTestAppleClaims(clientId))
		_, err = verifier.Verify(idToken)
		assert.Error(t, err)
	})
}

// fakeAppleIDTokenVerifier lets AppleUserDataFetcher tests isolate from real
// cryptography entirely, the same way MockGoogleUserDataFetcher etc. isolate
// the rest of the OAuth2 flow from a real provider.
type fakeAppleIDTokenVerifier struct {
	claims *appleIDTokenClaims
	err    error
}

func (f *fakeAppleIDTokenVerifier) Verify(idToken string) (*appleIDTokenClaims, error) {
	return f.claims, f.err
}

func TestAppleUserDataFetcher_FetchUserData(t *testing.T) {
	t.Run("successful fetch, no name payload (a returning user's login)", func(t *testing.T) {
		fetcher := &AppleUserDataFetcher{
			Verifier: &fakeAppleIDTokenVerifier{
				claims: &appleIDTokenClaims{
					RegisteredClaims: jwt.RegisteredClaims{Subject: "001234.abcdef.5678"},
					Email:            "user@example.com",
					EmailVerified:    true,
				},
			},
		}
		token := (&oauth2.Token{}).WithExtra(map[string]interface{}{"id_token": "fake-id-token"})
		req := httptest.NewRequest("POST", "/_/auth/apple/callback", nil)

		userInfo, err := fetcher.FetchUserData(req, token)
		require.NoError(t, err)
		assert.Equal(t, "001234.abcdef.5678", userInfo.ID)
		assert.Equal(t, "user@example.com", userInfo.Email)
		assert.Equal(t, "user@example.com", userInfo.Username)
		assert.True(t, userInfo.VerifiedEmail)
		assert.Equal(t, "apple", userInfo.Provider)
		assert.Empty(t, userInfo.Name, "no 'user' form field present, so no name to have parsed")
	})

	t.Run("first authorization includes the one-time name payload", func(t *testing.T) {
		fetcher := &AppleUserDataFetcher{
			Verifier: &fakeAppleIDTokenVerifier{
				claims: &appleIDTokenClaims{
					RegisteredClaims: jwt.RegisteredClaims{Subject: "001234.abcdef.5678"},
					Email:            "user@example.com",
					EmailVerified:    true,
				},
			},
		}
		token := (&oauth2.Token{}).WithExtra(map[string]interface{}{"id_token": "fake-id-token"})

		form := url.Values{}
		form.Set("state", "irrelevant-here")
		form.Set("user", `{"name":{"firstName":"Ada","lastName":"Lovelace"}}`)
		req := httptest.NewRequest("POST", "/_/auth/apple/callback", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		require.NoError(t, req.ParseForm())

		userInfo, err := fetcher.FetchUserData(req, token)
		require.NoError(t, err)
		assert.Equal(t, "Ada", userInfo.GivenName)
		assert.Equal(t, "Lovelace", userInfo.FamilyName)
		assert.Equal(t, "Ada Lovelace", userInfo.Name)
	})

	t.Run("malformed name payload is non-fatal", func(t *testing.T) {
		fetcher := &AppleUserDataFetcher{
			Verifier: &fakeAppleIDTokenVerifier{
				claims: &appleIDTokenClaims{
					RegisteredClaims: jwt.RegisteredClaims{Subject: "001234.abcdef.5678"},
					Email:            "user@example.com",
				},
			},
		}
		token := (&oauth2.Token{}).WithExtra(map[string]interface{}{"id_token": "fake-id-token"})

		form := url.Values{}
		form.Set("user", `not valid json`)
		req := httptest.NewRequest("POST", "/_/auth/apple/callback", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		require.NoError(t, req.ParseForm())

		userInfo, err := fetcher.FetchUserData(req, token)
		require.NoError(t, err, "a malformed one-time name payload must not fail the whole login")
		assert.Equal(t, "user@example.com", userInfo.Email)
	})

	t.Run("missing id_token is an error", func(t *testing.T) {
		fetcher := &AppleUserDataFetcher{Verifier: &fakeAppleIDTokenVerifier{}}
		token := &oauth2.Token{} // no Extra("id_token") set at all
		req := httptest.NewRequest("POST", "/_/auth/apple/callback", nil)

		_, err := fetcher.FetchUserData(req, token)
		assert.Error(t, err)
	})

	t.Run("verifier error propagates", func(t *testing.T) {
		fetcher := &AppleUserDataFetcher{
			Verifier: &fakeAppleIDTokenVerifier{err: assert.AnError},
		}
		token := (&oauth2.Token{}).WithExtra(map[string]interface{}{"id_token": "fake-id-token"})
		req := httptest.NewRequest("POST", "/_/auth/apple/callback", nil)

		_, err := fetcher.FetchUserData(req, token)
		assert.Error(t, err)
	})
}

func TestRegisterAppleAuth(t *testing.T) {
	t.Run("register with valid configuration", func(t *testing.T) {
		// RegisterAppleAuth builds a real keyfunc pointed at appleJWKSURL —
		// swapped here to a fake, always-empty JWKS server so this test
		// never depends on real network access to Apple. keyfunc tolerates
		// an empty/unreachable key set at construction time regardless
		// (see apple.go's comment on this), so this is purely to avoid an
		// unnecessary real network call, not to dodge a failure.
		fakeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"keys":[]}`))
		}))
		defer fakeServer.Close()
		originalURL := appleJWKSURL
		appleJWKSURL = fakeServer.URL
		defer func() { appleJWKSURL = originalURL }()

		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		userRepo := db.NewDBUserRepository(nil)

		_, keyPEM := generateTestECKeyPEM(t)
		gatewayConfig := &config.GatewayConfig{
			Server:     config.ServerConfig{URL: "http://localhost:8080"},
			Management: config.ManagementConfig{Prefix: "/_"},
			AuthenticationProviders: config.AuthenticationProviders{
				Apple: config.AppleAuthProviderCredentials{
					ClientId:   "com.example.service",
					TeamId:     "TEAMID1234",
					KeyId:      "KEYID5678",
					PrivateKey: keyPEM,
				},
			},
		}

		RegisterAppleAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/apple/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)
		location := loginRec.Header().Get("Location")
		assert.Contains(t, location, "appleid.apple.com/auth/authorize")
		assert.Contains(t, location, "client_id=com.example.service")
		assert.Contains(t, location, "response_mode=form_post")
	})

	t.Run("skip registration when any credential is missing", func(t *testing.T) {
		mux := http.NewServeMux()
		sessionRepo := db.NewSessionRepositoryDB(db.GetConnection())
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)
		db.SetupTestDB("TestAppleAuth")
		userRepo := db.NewDBUserRepository(db.GetConnection())

		gatewayConfig := &config.GatewayConfig{
			AuthenticationProviders: config.AuthenticationProviders{
				Apple: config.AppleAuthProviderCredentials{
					ClientId: "com.example.service",
					TeamId:   "TEAMID1234",
					// KeyId and PrivateKey left empty
				},
			},
		}

		RegisterAppleAuth(mux, sessionStore, gatewayConfig, userRepo)

		loginReq := httptest.NewRequest("GET", "/_/auth/apple/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusNotFound, loginRec.Code)
	})
}

// TestAppleOAuth2Flow_SharedMachinery exercises the two extensions
// providers.go's AuthenticationProvider gained for Apple —
// ClientSecretFunc (regenerating the client secret right before every
// exchange) and ResponseMode (form_post) plus the switch from
// r.URL.Query() to r.FormValue() in Callback — directly, with a mock
// Fetcher, the same way the other three providers' OAuth2Flow tests
// exercise the unmodified shared machinery.
func TestAppleOAuth2Flow_SharedMachinery(t *testing.T) {
	t.Run("Login sends response_mode=form_post when ResponseMode is set", func(t *testing.T) {
		db.SetupTestDB("TestAppleAuth")
		gormDB := db.GetConnection()
		userRepo := db.NewDBUserRepository(gormDB)
		sessionRepo := db.NewSessionRepositoryDB(gormDB)
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)

		gatewayConfig := &config.GatewayConfig{
			Management: config.ManagementConfig{Session: config.SessionConfig{SecondsDuration: 86400}},
		}
		oauthConfig := &oauth2.Config{
			ClientID:    "test-client-id",
			RedirectURL: "http://localhost:8080/_/auth/apple/callback",
			Scopes:      []string{"name", "email"},
			Endpoint: oauth2.Endpoint{
				AuthURL:  "https://appleid.apple.com/auth/authorize",
				TokenURL: "https://appleid.apple.com/auth/token",
			},
		}

		provider := AppleProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Apple", userRepo, sessionStore, gatewayConfig)
		authProvider.ResponseMode = "form_post"

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		loginReq := httptest.NewRequest("GET", "/_/auth/apple/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)

		assert.Equal(t, http.StatusTemporaryRedirect, loginRec.Code)
		assert.Contains(t, loginRec.Header().Get("Location"), "response_mode=form_post")
	})

	t.Run("Callback accepts a POST form body and regenerates the client secret before exchange", func(t *testing.T) {
		db.SetupTestDB("TestAppleAuth")
		gormDB := db.GetConnection()
		userRepo := db.NewDBUserRepository(gormDB)
		sessionRepo := db.NewSessionRepositoryDB(gormDB)
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)

		gatewayConfig := &config.GatewayConfig{
			Management: config.ManagementConfig{Session: config.SessionConfig{SecondsDuration: 86400}},
		}

		// A dummy TokenURL that always returns a minimal valid token
		// response — this is standing in for Apple's real token endpoint,
		// so the test can drive the shared Callback logic (state check,
		// ClientSecretFunc, r.FormValue-based param reading, Fetcher call,
		// session creation) end to end without a real Apple token server.
		tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// oauth2.Config.Exchange sends client credentials via HTTP
			// Basic auth by default for an endpoint it doesn't otherwise
			// recognize (AuthStyleAutoDetect) — not as a client_secret
			// form field.
			_, password, ok := r.BasicAuth()
			assert.True(t, ok, "expected client credentials via HTTP Basic auth")
			assert.Equal(t, "generated-fresh-secret", password, "the secret must be the freshly-generated one, not empty/stale")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"access_token":"fake-access-token","token_type":"bearer"}`))
		}))
		defer tokenServer.Close()

		oauthConfig := &oauth2.Config{
			ClientID:    "test-client-id",
			RedirectURL: "http://localhost:8080/_/auth/apple/callback",
			Endpoint:    oauth2.Endpoint{TokenURL: tokenServer.URL},
		}

		provider := AppleProvider{}
		mockFetcher := &MockUserDataFetcher{
			userInfo: &UserInfo{
				ID:            "apple123",
				Email:         "testuser@example.com",
				Username:      "testuser@example.com",
				VerifiedEmail: true,
				Provider:      "apple",
			},
		}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Apple", userRepo, sessionStore, gatewayConfig)
		authProvider.Fetcher = mockFetcher
		authProvider.ResponseMode = "form_post"
		secretCalls := 0
		authProvider.ClientSecretFunc = func() (string, error) {
			secretCalls++
			return "generated-fresh-secret", nil
		}

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		// Drive the real state-cookie handshake via Login first, exactly
		// like a browser would, rather than fabricating a matching
		// state+cookie pair by hand.
		loginReq := httptest.NewRequest("GET", "/_/auth/apple/login", nil)
		loginRec := httptest.NewRecorder()
		mux.ServeHTTP(loginRec, loginReq)
		var stateCookie, redirectCookie *http.Cookie
		for _, c := range loginRec.Result().Cookies() {
			switch c.Name {
			case StateCookieName:
				stateCookie = c
			case RedirectUrlCookieName:
				redirectCookie = c
			}
		}
		require.NotNil(t, stateCookie)

		form := url.Values{}
		form.Set("state", stateCookie.Value)
		form.Set("code", "test-auth-code")
		callbackReq := httptest.NewRequest("POST", "/_/auth/apple/callback", strings.NewReader(form.Encode()))
		callbackReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		callbackReq.AddCookie(stateCookie)
		if redirectCookie != nil {
			callbackReq.AddCookie(redirectCookie)
		}
		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)

		assert.Equal(t, http.StatusFound, callbackRec.Code, "body: %s", callbackRec.Body.String())
		assert.Equal(t, 1, secretCalls, "ClientSecretFunc must be called exactly once, right before the exchange")

		var sessionCookie *http.Cookie
		for _, c := range callbackRec.Result().Cookies() {
			if c.Name == session.SessionCookieName {
				sessionCookie = c
			}
		}
		require.NotNil(t, sessionCookie, "a successful callback must set a real session cookie")
	})

	t.Run("Callback with invalid state still rejects, via POST", func(t *testing.T) {
		db.SetupTestDB("TestAppleAuth")
		gormDB := db.GetConnection()
		userRepo := db.NewDBUserRepository(gormDB)
		sessionRepo := db.NewSessionRepositoryDB(gormDB)
		sessionStore := session.NewSessionStore(sessionRepo, 24*time.Hour)

		gatewayConfig := &config.GatewayConfig{
			Management: config.ManagementConfig{Session: config.SessionConfig{SecondsDuration: 86400}},
		}
		oauthConfig := &oauth2.Config{ClientID: "test-client-id", RedirectURL: "http://localhost:8080/_/auth/apple/callback"}
		provider := AppleProvider{}
		authProvider := NewAuthenticationProvider(oauthConfig, provider, "Apple", userRepo, sessionStore, gatewayConfig)

		mux := http.NewServeMux()
		authProvider.RegisterEndpoints(mux)

		form := url.Values{}
		form.Set("state", "invalid-state")
		form.Set("code", "test-code")
		callbackReq := httptest.NewRequest("POST", "/_/auth/apple/callback", strings.NewReader(form.Encode()))
		callbackReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		callbackReq.AddCookie(&http.Cookie{Name: StateCookieName, Value: "different-state"})

		callbackRec := httptest.NewRecorder()
		mux.ServeHTTP(callbackRec, callbackReq)

		assert.Equal(t, http.StatusUnauthorized, callbackRec.Code)
	})
}
